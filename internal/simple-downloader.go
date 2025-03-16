package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func SimpleDownload(url string, outputPath string) error {
	log := GetLogger("simple-download")
	progressManager := NewProgressManager()
	progressManager.StartDisplay()
	defer func() {
		close(progressManager.doneCh)
		progressManager.ShowSummary()
	}()

	client := createHTTPClient(3*time.Minute, 90*time.Second, "")
	progressCh := make(chan int64)
	// doneCh := make(chan struct{})

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("error creating HEAD request: %v", err)
	}
	req.Header.Set("User-Agent", ToolUserAgent)
	resp, err := client.Do(req)
	var fileSize int64 = -1
	if err == nil {
		defer resp.Body.Close()
		contentLength := resp.Header.Get("Content-Length")
		if contentLength != "" {
			fileSize, _ = parseContentLength(contentLength)
		}
	}
	log.Debug().Str("url", url).Int64("fileSize", fileSize).Msg("Registering ProgressManager")
	progressManager.Register(outputPath, fileSize)

	// Internal goroutine to forward progress updates to the manager
	go func() {
		var totalDownloaded int64
		for bytesDownloaded := range progressCh {
			progressManager.Update(outputPath, bytesDownloaded)
			totalDownloaded += bytesDownloaded
		}
		progressManager.Complete(outputPath, totalDownloaded)
	}()

	err = performSimpleDownload(url, outputPath, client, ToolUserAgent, progressCh)
	close(progressCh)
	// close(doneCh)
	return err
}

func performSimpleDownload(url string, outputPath string, client *http.Client, userAgent string, progressCh chan<- int64) error {
	log := GetLogger("simple-download")
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer outFile.Close()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating GET request: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Connection", "keep-alive")
	log.Debug().Str("url", url).Msg("Starting simple download")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing GET request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	buffer := make([]byte, bufferSize) // from helpers.go

	var totalDownloaded int64
	for {
		bytesRead, err := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := outFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				return fmt.Errorf("error writing to output file: %v", writeErr)
			}
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
	log.Debug().Int64("downloadedSize", totalDownloaded).Msg("Simple download completed")
	return nil
}
