package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func performSimpleDownload(url string, outputPath string, client *http.Client, userAgent string, progressCh chan<- int64) error {
	log := GetLogger("simple-download")
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
	buffer := make([]byte, bufferSize) // from helpers.go
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

func downloadWithProgress(config downloadConfig, client *http.Client, fileSize int64, progressCh chan<- int64) error {
	log := GetLogger("download-worker")
	log.Debug().Str("size", formatBytes(uint64(fileSize))).Msg("File size determined")
	job := downloadJob{
		Config:    config,
		FileSize:  fileSize,
		StartTime: time.Now(),
	}
	tempDir := filepath.Join(filepath.Dir(config.OutputPath), ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Error().Err(err).Str("dir", tempDir).Msg("Error creating temp directory")
		return fmt.Errorf("error creating temp directory: %v", err)
	}

	// Setup chunks
	mutex := &sync.Mutex{}
	chunkSize := fileSize / int64(config.Connections)
	log.Debug().Int("connections", config.Connections).Str("chunkSize", formatBytes(uint64(chunkSize))).Msg("Download configuration")
	var currentPosition int64 = 0
	for i := range config.Connections {
		startByte := currentPosition
		endByte := startByte + chunkSize - 1
		if i == config.Connections-1 {
			endByte = fileSize - 1
		}
		if endByte >= fileSize {
			endByte = fileSize - 1
		}
		if endByte >= startByte {
			job.Chunks = append(job.Chunks, downloadChunk{
				ID:        i,
				StartByte: startByte,
				EndByte:   endByte,
			})
		}
		currentPosition = endByte + 1
	}
	log.Debug().Str("output", config.OutputPath).Int("chunks", len(job.Chunks)).Msg("Download divided into chunks")

	// Start connection goroutines
	var wg sync.WaitGroup
	for i := range job.Chunks {
		wg.Add(1)
		go chunkedDownload(&job, &job.Chunks[i], client, &wg, progressCh, mutex)
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(progressCh)
	allCompleted := true
	var incompleteChunks []int
	for i, chunk := range job.Chunks {
		if !chunk.Completed {
			allCompleted = false
			incompleteChunks = append(incompleteChunks, i)
		}
	}
	if !allCompleted {
		return fmt.Errorf("download incomplete: %d chunks failed: %v", len(incompleteChunks), incompleteChunks)
	}

	// Assemble the file
	err := assembleFile(job)
	if err != nil {
		return fmt.Errorf("error assembling file: %v", err)
	}
	return nil
}
