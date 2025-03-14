package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func BatchDownload(entries []DownloadEntry, numLinks int, connectionsPerLink int, timeout time.Duration, userAgent string, proxyURL string) error {
	log := GetLogger("downloader")
	log.Info().Int("totalFiles", len(entries)).Int("numLinks", numLinks).Int("connections", connectionsPerLink).Msg("Starting batch download")

	progressManager := NewProgressManager()
	progressManager.StartDisplay()
	defer func() {
		close(progressManager.doneCh)
		progressManager.ShowSummary()
		for _, entry := range entries {
			os.RemoveAll(filepath.Join(filepath.Dir(entry.OutputPath), ".danzo-temp"))
		}
	}()

	var wg sync.WaitGroup
	errorCh := make(chan error, len(entries))
	entriesCh := make(chan DownloadEntry, len(entries))
	for _, entry := range entries {
		entriesCh <- entry
	}
	close(entriesCh)

	// Start worker goroutines
	for i := range numLinks {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			logger := log.With().Int("workerID", workerID).Logger()
			for entry := range entriesCh {
				logger.Debug().Str("output", entry.OutputPath).Msg("Worker starting download")
				config := DownloadConfig{
					URL:         entry.URL,
					OutputPath:  entry.OutputPath,
					Connections: connectionsPerLink,
					Timeout:     timeout,
					UserAgent:   userAgent,
					ProxyURL:    proxyURL,
				}
				progressCh := make(chan int64)
				doneCh := make(chan struct{})
				client := createHTTPClient(config.Timeout, config.ProxyURL)
				fileSize, err := getFileSize(config.URL, config.UserAgent, client)
				if err != nil {
					logger.Error().Err(err).Str("output", entry.OutputPath).Msg("Failed to get file size")
					errorCh <- fmt.Errorf("error getting file size for %s: %v", entry.URL, err)
					continue
				}
				progressManager.Register(entry.OutputPath, fileSize)
				var internalWg sync.WaitGroup
				internalWg.Add(1)

				// Internal goroutine to forward progress updates to the manager
				go func(outputPath string, progCh <-chan int64, doneCh chan struct{}) {
					defer internalWg.Done()
					var totalDownloaded int64
					for {
						select {
						case bytesDownloaded, ok := <-progCh:
							if !ok {
								progressManager.Complete(outputPath, totalDownloaded)
								return
							}
							progressManager.Update(outputPath, bytesDownloaded)
							totalDownloaded += bytesDownloaded
						case <-doneCh:
							progressManager.Complete(outputPath, totalDownloaded)
							return
						}
					}
				}(entry.OutputPath, progressCh, doneCh)

				err = downloadWithProgress(config, fileSize, progressCh)
				close(doneCh)
				internalWg.Wait()

				if err != nil {
					logger.Error().Err(err).Msg("Download failed")
					errorCh <- fmt.Errorf("error downloading %s: %v", entry.URL, err)
				} else {
					logger.Debug().Str("output", entry.OutputPath).Msg("Download completed successfully")
				}
			}
		}(i + 1)
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(errorCh)
	var errors []error
	for err := range errorCh {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		return fmt.Errorf("batch download completed with %d errors: %v", len(errors), errors)
	}
	return nil
}

func downloadWithProgress(config DownloadConfig, fileSize int64, progressCh chan<- int64) error {
	log := GetLogger("download-worker")
	client := createHTTPClient(config.Timeout, config.ProxyURL)
	log.Debug().Str("size", formatBytes(uint64(fileSize))).Msg("File size determined")
	job := DownloadJob{
		Config:    config,
		FileSize:  fileSize,
		StartTime: time.Now(),
	}

	// Setup chunks
	mutex := &sync.Mutex{}
	chunkSize := fileSize / int64(config.Connections)
	minChunkSize := int64(2 * 1024 * 1024) // 2MB minimum
	if chunkSize < minChunkSize && fileSize > minChunkSize {
		newConnections := max(int(fileSize/minChunkSize), 1)
		log.Debug().Int("oldConnections", config.Connections).Int("newConnections", newConnections).Msg("Adjust connections for min. chunk size")
		config.Connections = newConnections
		chunkSize = fileSize / int64(config.Connections)
	}
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
			job.Chunks = append(job.Chunks, DownloadChunk{
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
		go downloadChunk(&job, &job.Chunks[i], client, &wg, progressCh, mutex)
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
