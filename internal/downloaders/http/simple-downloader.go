package danzohttp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tanq16/danzo/internal/utils"
)

func PerformSimpleDownload(url, outputPath string, client *utils.DanzoHTTPClient, progressCh chan<- int64) error {
	outputDir := filepath.Dir(outputPath)
	tempOutputPath := fmt.Sprintf("%s.part", outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}

	var resumeOffset int64 = 0
	var fileMode int = os.O_CREATE | os.O_WRONLY
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

	// Extract OAuth token for Google Drive download ONLY
	urlToken := []string{}
	if strings.HasPrefix(url, "https://www.googleapis.com/drive/v3/files") {
		urlToken = append(urlToken, strings.Split(url, "|")...)
		url = urlToken[0]
	}
	req, err := http.NewRequest("GET", url, nil)
	// Set OAuth bearer token for Google Drive download ONLY
	if len(urlToken) > 1 {
		req.Header.Set("Authorization", "Bearer "+urlToken[1])
	}
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
			defer outFile.Close()
			resumeOffset = 0
		} else {
			progressCh <- resumeOffset
		}
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	buffer := make([]byte, utils.DefaultBufferSize)
	var newBytes int64 = 0
	var totalDownloaded int64 = resumeOffset
	for {
		bytesRead, err := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := outFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				return fmt.Errorf("error writing to output file: %v", writeErr)
			}
			newBytes += int64(bytesRead)
			totalDownloaded += int64(bytesRead)
			progressCh <- int64(bytesRead)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading response body: %v", err)
		}
	}
	// Ensure file is synced and closed (while auto-handled in Unix, Windows needs this)
	outFile.Sync()
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("error closing output file: %v", err)
	}
	if err := os.Rename(tempOutputPath, outputPath); err != nil {
		return fmt.Errorf("error renaming (finalizing) output file: %v", err)
	}
	return nil
}
