package danzohttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tanq16/danzo/utils"
)

func chunkedDownload(ctx context.Context, job *HTTPDownloadJob, chunk *HTTPDownloadChunk, client *utils.DanzoHTTPClient, progressCh chan<- int64, mutex *sync.Mutex) error {
	tempDir := filepath.Join(filepath.Dir(job.Config.OutputPath), ".danzo-temp")
	tempFileName := filepath.Join(tempDir, fmt.Sprintf("%s.part%d", filepath.Base(job.Config.OutputPath), chunk.ID))
	expectedSize := chunk.EndByte - chunk.StartByte + 1

	// reconcile keeps chunk.Downloaded equal to the on-disk size and emits the
	// signed delta on progressCh, so the receiver's accumulated total never
	// drifts above the true byte count across retries.
	reconcile := func() int64 {
		currentSize := int64(0)
		if fi, err := os.Stat(tempFileName); err == nil {
			currentSize = fi.Size()
		}
		if currentSize > expectedSize {
			os.Remove(tempFileName)
			currentSize = 0
		}
		if delta := currentSize - chunk.Downloaded; delta != 0 {
			progressCh <- delta
			chunk.Downloaded = currentSize
		}
		return currentSize
	}

	resumeOffset := reconcile()
	if resumeOffset == expectedSize {
		mutex.Lock()
		job.TempFiles = append(job.TempFiles, tempFileName)
		mutex.Unlock()
		chunk.Completed = true
		return nil
	}

	maxRetries := 5
	var lastErr error
	for retry := range maxRetries {
		if retry > 0 {
			time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
			resumeOffset = reconcile()
		}
		if err := downloadSingleChunk(ctx, job, chunk, client, tempFileName, progressCh, resumeOffset); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			resumeOffset = reconcile()
			continue
		}
		mutex.Lock()
		job.TempFiles = append(job.TempFiles, tempFileName)
		mutex.Unlock()
		chunk.Completed = true
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("chunk %d failed after %d retries: %w", chunk.ID, maxRetries, lastErr)
	}
	return fmt.Errorf("chunk %d failed after %d retries", chunk.ID, maxRetries)
}

func downloadSingleChunk(ctx context.Context, job *HTTPDownloadJob, chunk *HTTPDownloadChunk, client *utils.DanzoHTTPClient, tempFileName string, progressCh chan<- int64, resumeOffset int64) error {
	flag := os.O_WRONLY | os.O_CREATE
	if resumeOffset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	tempFile, err := os.OpenFile(tempFileName, flag, 0644)
	if err != nil {
		return fmt.Errorf("error opening temp file: %v", err)
	}
	defer tempFile.Close()

	startByte := chunk.StartByte + resumeOffset
	rangeHeader := fmt.Sprintf("bytes=%d-%d", startByte, chunk.EndByte)
	req, err := http.NewRequestWithContext(ctx, "GET", job.Config.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", rangeHeader)
	req.Header.Set("Connection", "keep-alive")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	contentRange := resp.Header.Get("Content-Range")
	if contentRange == "" {
		return errors.New("missing Content-Range header")
	}

	remainingBytes := chunk.EndByte - startByte + 1
	buffer := make([]byte, utils.DefaultBufferSize)
	newBytes := int64(0)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		bytesRead, err := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := tempFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				return writeErr
			}
			newBytes += int64(bytesRead)
			chunk.Downloaded += int64(bytesRead)
			progressCh <- int64(bytesRead)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if newBytes != remainingBytes {
		return fmt.Errorf("size mismatch: expected %d remaining bytes, got %d bytes this session", remainingBytes, newBytes)
	}
	totalExpectedSize := chunk.EndByte - chunk.StartByte + 1
	if chunk.Downloaded != totalExpectedSize {
		return fmt.Errorf("total size mismatch: expected %d total bytes, got %d bytes", totalExpectedSize, chunk.Downloaded)
	}
	return nil
}
