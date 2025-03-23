package internal

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	danzohttp "github.com/tanq16/danzo/downloaders/http"
	"github.com/tanq16/danzo/utils"
)

func BatchDownload(entries []utils.DownloadEntry, numLinks int, connectionsPerLink int, timeout time.Duration, kaTimeout time.Duration, userAgent string, proxyURL string) error {
	log := utils.GetLogger("downloader")
	log.Info().Int("totalFiles", len(entries)).Int("workers", numLinks).Int("connections", connectionsPerLink).Msg("Initiating download")

	progressManager := NewProgressManager()
	progressManager.StartDisplay()
	defer func() {
		progressManager.Stop()
		progressManager.ShowSummary()
		for _, entry := range entries {
			utils.Clean(entry.OutputPath)
		}
	}()

	var wg sync.WaitGroup
	errorCh := make(chan error, len(entries))
	entriesCh := make(chan utils.DownloadEntry, len(entries))
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
				logger.Debug().Str("tempName", entry.OutputPath).Msg("Worker starting download")
				if userAgent == "randomize" {
					userAgent = utils.GetRandomUserAgent()
				}
				config := utils.DownloadConfig{
					URL:         entry.URL,
					OutputPath:  entry.OutputPath,
					Connections: connectionsPerLink,
					Timeout:     timeout,
					KATimeout:   kaTimeout,
					UserAgent:   userAgent,
					ProxyURL:    proxyURL,
				}
				progressCh := make(chan int64)
				useHighThreadMode := config.Connections > 5
				client := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, useHighThreadMode)
				fileSize, fileName, err := utils.GetFileInfo(config.URL, config.UserAgent, client)
				if config.OutputPath == "" && fileName != "" {
					config.OutputPath = fileName
					entry.OutputPath = fileName
				} else if config.OutputPath == "" {
					parsedURL, _ := url.Parse(config.URL)
					config.OutputPath = strings.Split(parsedURL.Path, "/")[len(strings.Split(parsedURL.Path, "/"))-1]
					entry.OutputPath = config.OutputPath
				}
				logger.Debug().Str("output", config.OutputPath).Msg("Output path determined")

				if err == utils.ErrRangeRequestsNotSupported {
					logger.Debug().Str("url", entry.URL).Msg("Range requests not supported, using simple download")
					progressManager.Register(entry.OutputPath, -1) // -1 means unknown size
				} else if err != nil {
					logger.Error().Err(err).Str("output", entry.OutputPath).Msg("Failed to get file size")
					errorCh <- fmt.Errorf("error getting file size for %s: %v", entry.URL, err)
					continue
				} else {
					progressManager.Register(entry.OutputPath, fileSize)
				}

				var progressWg sync.WaitGroup
				progressWg.Add(1)
				// Internal goroutine to forward progress updates to the manager
				go func(outputPath string, progCh <-chan int64) {
					defer progressWg.Done()
					var totalDownloaded int64
					for bytesDownloaded := range progCh {
						progressManager.Update(outputPath, bytesDownloaded)
						totalDownloaded += bytesDownloaded
					}
					progressManager.Complete(outputPath, totalDownloaded)
				}(entry.OutputPath, progressCh)

				if err == utils.ErrRangeRequestsNotSupported || config.Connections == 1 {
					logger.Debug().Str("output", entry.OutputPath).Msg("SIMPLE DOWNLOAD with 1 connection")
					simpleClient := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, false)
					err = danzohttp.PerformSimpleDownload(entry.URL, entry.OutputPath, simpleClient, config.UserAgent, progressCh)
					close(progressCh)
				} else if fileSize/int64(config.Connections) < 10*1024*1024 {
					logger.Debug().Str("output", entry.OutputPath).Msg("SIMPLE DOWNLOAD bcz low file size")
					simpleClient := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, false)
					err = danzohttp.PerformSimpleDownload(entry.URL, entry.OutputPath, simpleClient, config.UserAgent, progressCh)
					close(progressCh)
				} else {
					err = danzohttp.PerformMultiDownload(config, client, fileSize, progressCh)
				}
				progressWg.Wait()

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
