package internal

import (
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"
)

func Download(config DownloadConfig) error {
	log := GetLogger("downloader")

	parsedURL, err := url.Parse(config.URL)
	if err != nil {
		return fmt.Errorf("error parsing URL: %v", err)
	}
	primaryScheme := parsedURL.Scheme
	config.ProxyURL = fmt.Sprintf("%s://%s", primaryScheme, config.ProxyURL)
	client := createHTTPClient(config.Timeout, config.ProxyURL)
	fileSize, err := getFileSize(config.URL, config.UserAgent, client)
	if err != nil {
		return fmt.Errorf("error getting file size: %v", err)
	}
	log.Info().Str("size", formatBytes(uint64(fileSize))).Msg("File size determined")
	job := DownloadJob{
		Config:    config,
		FileSize:  fileSize,
		StartTime: time.Now(),
	}
	mutex := &sync.Mutex{}
	chunkSize := fileSize / int64(config.Connections)
	minChunkSize := int64(2 * 1024 * 1024) // 1MB minimum
	if chunkSize < minChunkSize && fileSize > minChunkSize {
		newConnections := max(int(fileSize/minChunkSize), 1)
		log.Info().Int("oldConnections", config.Connections).Int("newConnections", newConnections).Msg("Adjusting to maintain minimum chunk size")
		config.Connections = newConnections
		chunkSize = fileSize / int64(config.Connections)
	}
	log.Info().Int("connections", config.Connections).Str("chunkSize", formatBytes(uint64(chunkSize))).Msg("Download configuration")

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
			job.Chunks = append(job.Chunks, DownloadChunk{
				ID:        i,
				StartByte: startByte,
				EndByte:   endByte,
			})
		}
		currentPosition = endByte + 1
	}
	log.Info().Int("chunks", len(job.Chunks)).Msg("Download divided into chunks")

	progressCh := make(chan int64, config.Connections*2) // Buffer to prevent blocking
	doneCh := make(chan struct{})
	go displayProgress(fileSize, progressCh, doneCh)

	var wg sync.WaitGroup
	for i := range job.Chunks {
		wg.Add(1)
		go downloadChunk(&job, &job.Chunks[i], client, &wg, progressCh, mutex)
	}
	wg.Wait()
	close(doneCh) // Signal progress display to finish

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

	err = assembleFile(job)
	if err != nil {
		return fmt.Errorf("error assembling file: %v", err)
	}
	fileInfo, err := os.Stat(job.Config.OutputPath)
	if err != nil {
		return fmt.Errorf("error getting file info: %v", err)
	}
	if fileInfo.Size() != fileSize {
		return fmt.Errorf("file size mismatch: expected %d bytes, got %d bytes", fileSize, fileInfo.Size())
	}
	log.Info().Str("size", formatBytes(uint64(fileInfo.Size()))).Str("path", job.Config.OutputPath).Msg("File size verified")
	return nil
}
