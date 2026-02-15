package danzohttp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tanq16/danzo/internal/utils"
)

func PerformSimpleDownload(url, outputPath string, client *utils.DanzoHTTPClient, progressCh chan<- int64) error {
	defer close(progressCh)
	tempDir := filepath.Join(filepath.Dir(outputPath), ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	tempOutputPath := fmt.Sprintf("%s.part", filepath.Join(tempDir, filepath.Base(outputPath)))
	maxRetries := 5
	var lastErr error

	for retry := range maxRetries {
		if retry > 0 {
			time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
		}
		err := downloadAttempt(url, tempOutputPath, client, progressCh)
		if err != nil {
			lastErr = err
			continue
		}
		if err := os.Rename(tempOutputPath, outputPath); err != nil {
			return fmt.Errorf("error renaming (finalizing) output file: %v", err)
		}
		return nil
	}
	return fmt.Errorf("download failed after %d retries: %w", maxRetries, lastErr)
}

func downloadAttempt(url, tempOutputPath string, client *utils.DanzoHTTPClient, progressCh chan<- int64) error {
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

	req, err := http.NewRequest("GET", url, nil)
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

	if resumeOffset > 0 {
		if resp.StatusCode != http.StatusPartialContent {
			outFile.Close()
			outFile, err = os.OpenFile(tempOutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("error creating output file: %v", err)
			}
			resumeOffset = 0
		} else {
			progressCh <- resumeOffset
		}
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	buffer := make([]byte, utils.DefaultBufferSize)
	for {
		bytesRead, readErr := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := outFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				return fmt.Errorf("error writing to output file: %v", writeErr)
			}
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
