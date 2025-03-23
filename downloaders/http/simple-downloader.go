package danzohttp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tanq16/danzo/utils"
)

func PerformSimpleDownload(url string, outputPath string, client *http.Client, userAgent string, progressCh chan<- int64) error {
	log := utils.GetLogger("simple-download")
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
		log.Debug().Str("file", filepath.Base(tempOutputPath)).Int64("size", resumeOffset).Msg("Resuming incomplete download")
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
		log.Debug().Int64("resumeOffset", resumeOffset).Msg("Setting Range header for resume")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Connection", "keep-alive")
	log.Debug().Str("url", url).Msg("Starting simple download")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing GET request: %v", err)
	}
	defer resp.Body.Close()

	if resumeOffset > 0 {
		if resp.StatusCode != http.StatusPartialContent {
			log.Warn().Int("statusCode", resp.StatusCode).Msg("Server doesn't support resume, starting from beginning")
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
	buffer := make([]byte, utils.DefaultBufferSize) // from helpers.go
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
	log.Debug().Int64("resumeOffset", resumeOffset).Int64("downloadedThisSession", newBytes).Int64("totalDownloaded", totalDownloaded).Msg("Simple download completed")
	if err := os.Rename(tempOutputPath, outputPath); err != nil {
		return fmt.Errorf("error renaming (finalizing) output file: %v", err)
	}
	return nil
}
