package internal

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

func Download(config DownloadConfig) error {
	client := createHTTPClient(config.Timeout)
	fileSize, err := getFileSize(config.URL, config.UserAgent, client)
	if err != nil {
		return fmt.Errorf("error getting file size: %v", err)
	}
	log.Printf("File size: %s", formatBytes(uint64(fileSize)))
	job := DownloadJob{
		Config:    config,
		FileSize:  fileSize,
		StartTime: time.Now(),
	}
	mutex := &sync.Mutex{}
	chunkSize := fileSize / int64(config.Connections)
	minChunkSize := int64(1024 * 1024) // 1MB minimum
	if chunkSize < minChunkSize && fileSize > minChunkSize {
		newConnections := max(int(fileSize/minChunkSize), 1)
		log.Printf("Adjusting connections from %d to %d to maintain minimum chunk size", config.Connections, newConnections)
		config.Connections = newConnections
		chunkSize = fileSize / int64(config.Connections)
	}
	log.Printf("Using %d connections with chunk size of %s", config.Connections, formatBytes(uint64(chunkSize)))
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
	log.Printf("Download divided into %d chunks", len(job.Chunks))
	progressCh := make(chan int64, config.Connections*2) // Buffer to prevent blocking
	doneCh := make(chan struct{})
	go displayProgress(fileSize, progressCh, doneCh)
	log.Println("Starting download...")

	var wg sync.WaitGroup
	for i := range job.Chunks {
		wg.Add(1)
		go downloadChunk(&job, &job.Chunks[i], client, &wg, progressCh, mutex)
	}
	wg.Wait()
	// Signal progress display to finish
	close(doneCh)

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
	log.Println("Assembling file chunks...")
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
	log.Printf("File size verified: %s", formatBytes(uint64(fileInfo.Size())))
	return nil
}
