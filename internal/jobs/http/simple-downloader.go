package danzohttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tanq16/danzo/utils"
)

func PerformSimpleDownload(ctx context.Context, url, outputPath string, client *utils.DanzoHTTPClient, progressCh chan<- int64) error {
	defer close(progressCh)
	tempDir := filepath.Join(filepath.Dir(outputPath), ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	tempOutputPath := fmt.Sprintf("%s.part", filepath.Join(tempDir, filepath.Base(outputPath)))

	// reported is the running sum of byte deltas this function has emitted on
	// progressCh. reconcile re-syncs it with the on-disk temp file size so the
	// receiver's accumulated total never drifts above the true byte count
	// across retries.
	var reported int64
	reconcile := func() {
		currentSize := int64(0)
		if fi, err := os.Stat(tempOutputPath); err == nil {
			currentSize = fi.Size()
		}
		if delta := currentSize - reported; delta != 0 {
			progressCh <- delta
			reported = currentSize
		}
	}
	reconcile()

	maxRetries := 5
	var lastErr error
	for retry := range maxRetries {
		if retry > 0 {
			time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
			reconcile()
		}
		err := downloadAttempt(ctx, url, tempOutputPath, client, progressCh, &reported)
		if err != nil {
			lastErr = err
			reconcile()
			continue
		}
		if err := os.Rename(tempOutputPath, outputPath); err != nil {
			return fmt.Errorf("error renaming (finalizing) output file: %v", err)
		}
		return nil
	}
	return fmt.Errorf("download failed after %d retries: %w", maxRetries, lastErr)
}

func downloadAttempt(ctx context.Context, url, tempOutputPath string, client *utils.DanzoHTTPClient, progressCh chan<- int64, reported *int64) error {
	var resumeOffset int64 = 0
	fileMode := os.O_CREATE | os.O_WRONLY
	if fileInfo, err := os.Stat(tempOutputPath); err == nil {
		resumeOffset = fileInfo.Size()
		fileMode |= os.O_APPEND
	} else {
		fileMode |= os.O_TRUNC
	}

	outFile, err := os.OpenFile(tempOutputPath, fileMode, 0644)
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer outFile.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating GET request: %v", err)
	}

	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}
	req.Header.Set("Connection", "keep-alive")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing GET request: %v", err)
	}
	defer resp.Body.Close()

	switch {
	case resumeOffset > 0 && resp.StatusCode == http.StatusPartialContent:
		// happy path: server is honoring the Range request
	case resumeOffset > 0 && resp.StatusCode == http.StatusOK:
		// server ignored Range and is sending the whole body; restart fresh
		outFile.Close()
		outFile, err = os.OpenFile(tempOutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("error creating output file: %v", err)
		}
		if *reported > 0 {
			progressCh <- -*reported
			*reported = 0
		}
		resumeOffset = 0
	case resumeOffset == 0 && resp.StatusCode == http.StatusOK:
		// fresh download
	default:
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	buffer := make([]byte, utils.DefaultBufferSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		bytesRead, readErr := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := outFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				return fmt.Errorf("error writing to output file: %v", writeErr)
			}
			*reported += int64(bytesRead)
			progressCh <- int64(bytesRead)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("error reading response body: %v", readErr)
		}
	}
	outFile.Sync()
	return nil
}
