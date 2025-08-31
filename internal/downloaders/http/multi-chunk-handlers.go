package danzohttp

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tanq16/danzo/internal/utils"
)

func chunkedDownload(job *utils.HTTPDownloadJob, chunk *utils.HTTPDownloadChunk, client *utils.DanzoHTTPClient, wg *sync.WaitGroup, progressCh chan<- int64, mutex *sync.Mutex) {
	defer wg.Done()
	tempDir := filepath.Join(filepath.Dir(job.Config.OutputPath), ".danzo-temp")
	tempFileName := filepath.Join(tempDir, fmt.Sprintf("%s.part%d", filepath.Base(job.Config.OutputPath), chunk.ID))
	expectedSize := chunk.EndByte - chunk.StartByte + 1
	resumeOffset := int64(0)
	if fileInfo, err := os.Stat(tempFileName); err == nil {
		resumeOffset = fileInfo.Size()
		if resumeOffset == expectedSize {
			mutex.Lock()
			job.TempFiles = append(job.TempFiles, tempFileName)
			mutex.Unlock()
			chunk.Downloaded = resumeOffset
			chunk.Completed = true
			progressCh <- resumeOffset
			return
		} else if resumeOffset > 0 && resumeOffset < expectedSize {
		} else if chunk.Downloaded > 0 {
			os.Remove(tempFileName)
			resumeOffset = 0
		}
	}
	maxRetries := 5
	for retry := range maxRetries {
		if retry > 0 {
			time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond) // Backoff
			if fileInfo, err := os.Stat(tempFileName); err == nil {
				currentSize := fileInfo.Size()
				if currentSize != chunk.Downloaded && chunk.Downloaded > 0 {
					os.Remove(tempFileName)
					progressCh <- -chunk.Downloaded // Subtract from progress
					chunk.Downloaded = 0
					resumeOffset = 0
				}
			}
		}
		if err := downloadSingleChunk(job, chunk, client, tempFileName, progressCh, resumeOffset); err != nil {
			continue
		}
		// On success
		mutex.Lock()
		job.TempFiles = append(job.TempFiles, tempFileName)
		mutex.Unlock()
		chunk.Completed = true
		return
	}
}

func downloadSingleChunk(job *utils.HTTPDownloadJob, chunk *utils.HTTPDownloadChunk, client *utils.DanzoHTTPClient, tempFileName string, progressCh chan<- int64, resumeOffset int64) error {
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
	req, err := http.NewRequest("GET", job.Config.URL, nil)
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

	if resumeOffset > 0 {
		progressCh <- resumeOffset
		chunk.Downloaded = resumeOffset
	}
	remainingBytes := chunk.EndByte - startByte + 1
	buffer := make([]byte, utils.DefaultBufferSize)
	newBytes := int64(0)
	for {
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
