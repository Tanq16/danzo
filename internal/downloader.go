package internal

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	danzogdrive "github.com/tanq16/danzo/downloaders/gdrive"
	danzogitr "github.com/tanq16/danzo/downloaders/gitrelease"
	danzohttp "github.com/tanq16/danzo/downloaders/http"
	danzos3 "github.com/tanq16/danzo/downloaders/s3"
	danzoyoutube "github.com/tanq16/danzo/downloaders/youtube"
	"github.com/tanq16/danzo/utils"
)

func BatchDownload(entries []utils.DownloadEntry, numLinks, connectionsPerLink int, timeout, kaTimeout time.Duration, userAgent, proxyURL string, headers []string) error {
	log := utils.GetLogger("downloader")
	log.Debug().Int("totalFiles", len(entries)).Int("workers", numLinks).Int("connections", connectionsPerLink).Msg("Initiating download")

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
					Headers:     headers,
				}
				progressCh := make(chan int64)
				useHighThreadMode := config.Connections > 5

				urlType := utils.DetermineDownloadType(config.URL)
				switch urlType {

				// =================================================================================================================
				// DOWNLOAD TYPES USED BY DANZO ====================================================================================
				// EACH DOWNLOAD TYPE HAS A SPECIFIC HANDLER OR PROCESSOR ==========================================================
				// HANDLERS ARE TRIGGERED IN THE SWITCH CASE BASED ON TYPE =========================================================
				// =================================================================================================================

				// HTTP download
				// =================================================================================================================
				case "http":
					client := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, useHighThreadMode, config.Headers)
					fileSize, fileName, err := utils.GetFileInfo(config.URL, config.UserAgent, client)
					if config.OutputPath == "" && fileName != "" {
						config.OutputPath = fileName
						entry.OutputPath = fileName
					} else if config.OutputPath == "" {
						parsedURL, _ := url.Parse(config.URL)
						config.OutputPath = strings.Split(parsedURL.Path, "/")[len(strings.Split(parsedURL.Path, "/"))-1]
						entry.OutputPath = config.OutputPath
					} else if config.OutputPath != "" {
						existingFile, _ := os.Stat(config.OutputPath)
						if existingFile != nil {
							if fileSize > 0 && existingFile.Size() == fileSize {
								logger.Debug().Str("output", config.OutputPath).Msg("File already exists and is complete, skipping download")
								continue
							}
							config.OutputPath = utils.RenewOutputPath(config.OutputPath)
							entry.OutputPath = config.OutputPath
						}
					}
					logger.Debug().Str("output", config.OutputPath).Msg("Output path determined")

					if err == utils.ErrRangeRequestsNotSupported {
						logger.Debug().Str("url", entry.URL).Msg("Range requests not supported, using simple download")
						progressManager.Register(entry.OutputPath, -1) // -1 means unknown size
					} else if err != nil {
						logger.Debug().Err(err).Str("output", entry.OutputPath).Msg("Failed to get file size")
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
						simpleClient := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, false, config.Headers)
						err = danzohttp.PerformSimpleDownload(entry.URL, entry.OutputPath, simpleClient, config.UserAgent, progressCh)
						close(progressCh)
					} else if fileSize/int64(config.Connections) < 2*utils.DefaultBufferSize {
						logger.Debug().Str("output", entry.OutputPath).Msg("SIMPLE DOWNLOAD bcz low file size")
						simpleClient := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, false, config.Headers)
						err = danzohttp.PerformSimpleDownload(entry.URL, entry.OutputPath, simpleClient, config.UserAgent, progressCh)
						close(progressCh)
					} else {
						err = danzohttp.PerformMultiDownload(config, client, fileSize, progressCh)
					}
					if err != nil {
						logger.Debug().Err(err).Msg("Download failed")
						reportError := fmt.Errorf("error downloading %s: %v", entry.URL, err)
						errorCh <- reportError
						progressManager.ReportError(entry.OutputPath, reportError)
					} else {
						logger.Debug().Str("output", entry.OutputPath).Msg("Download completed successfully")
					}
					// close(progressCh) // Closing the progress channel here would cause a panic (Multi-Download already closes it)
					progressWg.Wait()

				// YouTube download (with yt-dlp as dependency)
				// =================================================================================================================
				case "youtube":
					logger.Debug().Str("url", entry.URL).Msg("YouTube URL detected")
					processedURL, format, dType, output, err := danzoyoutube.ProcessURL(entry.URL)
					if err != nil {
						logger.Debug().Err(err).Msg("Failed to process YouTube URL")
						errorCh <- fmt.Errorf("error processing YouTube URL %s: %v", entry.URL, err)
						continue
					}
					if config.OutputPath == "" {
						config.OutputPath = output
						entry.OutputPath = output
					} else {
						existingFile, _ := os.Stat(config.OutputPath)
						if existingFile != nil {
							if existingFile.Size() > 0 {
								logger.Debug().Str("output", config.OutputPath).Msg("File already exists and is complete, skipping download")
								continue
							}
							config.OutputPath = utils.RenewOutputPath(config.OutputPath)
							entry.OutputPath = config.OutputPath
						}
					}
					entry.URL = processedURL
					// For YouTube, we register with unknown size and only update at the end
					progressManager.Register(entry.OutputPath, -1)
					streamCh := make(chan []string, 7)

					// Internal goroutine to forward progress updates to the manager
					var progressWg sync.WaitGroup
					progressWg.Add(1)
					go func(outputPath string, progCh <-chan int64) {
						defer progressWg.Done()
						var totalDownloaded int64
						for bytesDownloaded := range progCh {
							totalDownloaded += bytesDownloaded
						}
						progressManager.Complete(outputPath, totalDownloaded)
					}(entry.OutputPath, progressCh)

					// Goroutine to forward streaming output to the manager
					var streamWg sync.WaitGroup
					streamWg.Add(1)
					go func(outputPath string, streamCh <-chan []string) {
						defer streamWg.Done()
						for streamOutput := range streamCh {
							progressManager.UpdateStreamOutput(outputPath, streamOutput)
						}
					}(entry.OutputPath, streamCh)

					err = danzoyoutube.DownloadYouTubeVideo(entry.URL, entry.OutputPath, format, dType, progressCh, streamCh)
					if err != nil {
						logger.Debug().Err(err).Msg("YouTube download failed")
						reportError := fmt.Errorf("error downloading %s: %v", entry.URL, err)
						errorCh <- reportError
						progressManager.ReportError(entry.OutputPath, reportError)
					} else {
						logger.Debug().Str("output", entry.OutputPath).Msg("YouTube download completed successfully")
					}
					close(progressCh)
					close(streamCh)
					progressWg.Wait()
					streamWg.Wait()

				// GitHub Release download
				// =================================================================================================================
				case "gitrelease":
					logger.Debug().Str("url", entry.URL).Msg("GitHub Release URL detected")
					simpleClient := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, false, nil)
					downloadURL, filename, size, err := danzogitr.ProcessRelease(entry.URL, simpleClient)
					if err != nil {
						logger.Debug().Err(err).Msg("Failed to process GitHub release")
						errorCh <- fmt.Errorf("error processing GitHub release URL %s: %v", entry.URL, err)
						continue
					}

					if config.OutputPath == "" {
						config.OutputPath = filename
						entry.OutputPath = filename
					}
					logger.Debug().Str("output", config.OutputPath).Msg("Output path determined")

					progressManager.Register(entry.OutputPath, size)
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

					err = danzohttp.PerformSimpleDownload(downloadURL, entry.OutputPath, simpleClient, config.UserAgent, progressCh)
					if err != nil {
						logger.Debug().Err(err).Msg("GitHub release download failed")
						reportError := fmt.Errorf("error downloading %s: %v", entry.URL, err)
						errorCh <- reportError
						progressManager.ReportError(entry.OutputPath, reportError)
					} else {
						logger.Debug().Str("output", entry.OutputPath).Msg("GitHub release download completed successfully")
					}
					close(progressCh)
					progressWg.Wait()

				// AWS S3 download
				// =================================================================================================================
				case "s3":
					logger.Debug().Str("url", entry.URL).Msg("S3 URL detected")
					s3client, err := danzos3.GetS3Client()
					if err != nil {
						logger.Debug().Err(err).Msg("Failed to get S3 client")
						errorCh <- fmt.Errorf("error getting S3 client: %v", err)
						continue
					}
					bucket, key, fileType, size, err := danzos3.GetS3ObjectInfo(entry.URL, s3client)
					if err != nil {
						logger.Debug().Err(err).Msg("Failed to get S3 object info")
						errorCh <- fmt.Errorf("error getting S3 object info: %v", err)
						continue
					}

					var S3Jobs []danzos3.S3Job
					if fileType != "folder" {
						S3Jobs = append(S3Jobs, danzos3.S3Job{
							Bucket: bucket,
							Key:    key,
							Output: strings.Split(key, "/")[len(strings.Split(key, "/"))-1],
							Size:   size,
						})
					} else {
						S3Jobs, err = danzos3.GetAllObjectsFromFolder(bucket, key, s3client)
						if err != nil {
							logger.Debug().Err(err).Msg("Failed to list S3 objects in folder")
							errorCh <- fmt.Errorf("error listing S3 objects in folder: %v", err)
							continue
						}
					}

					// Close the progress channel because it's not used in S3 downloads
					close(progressCh)
					// Do all S3 downloads with connectionsPerLink number of parallel downloads
					// NOTE: This is non-standard usage of connectionsPerLink (usually, it signifies connections per link)
					var s3wg sync.WaitGroup
					var progressWg sync.WaitGroup
					s3Workers := 0
					if len(S3Jobs) < connectionsPerLink {
						s3Workers = len(S3Jobs)
					} else {
						s3Workers = connectionsPerLink
					}
					s3JobsCh := make(chan danzos3.S3Job, len(S3Jobs))
					var s3JobWg sync.WaitGroup
					s3JobWg.Add(1)
					go func() {
						defer s3JobWg.Done()
						for _, s3Job := range S3Jobs {
							s3JobsCh <- s3Job
						}
						close(s3JobsCh)
					}()
					for i := range s3Workers {
						s3wg.Add(1)
						go func(workerID int, s3JobsCh <-chan danzos3.S3Job) {
							defer s3wg.Done()
							logger := log.With().Int("workerID", workerID).Logger()
							for s3Job := range s3JobsCh {
								logger.Debug().Str("object", s3Job.Key).Msg("Downloading")
								progressManager.Register(s3Job.Output, s3Job.Size)
								s3FolderProgressCh := make(chan int64)
								progressWg.Add(1)
								go func(outputPath string, progCh <-chan int64) {
									defer progressWg.Done()
									var totalDownloaded int64
									for bytesDownloaded := range progCh {
										if bytesDownloaded < 0 {
											progressManager.Register(outputPath, -bytesDownloaded)
											continue
										}
										progressManager.Update(outputPath, bytesDownloaded)
										totalDownloaded += bytesDownloaded
									}
									progressManager.Complete(outputPath, totalDownloaded)
								}(s3Job.Output, s3FolderProgressCh)
								err := danzos3.PerformS3ObjectDownload(s3Job.Bucket, s3Job.Key, s3Job.Output, s3Job.Size, s3client, s3FolderProgressCh)
								if err != nil {
									logger.Debug().Err(err).Msg("S3 Download failed")
									reportError := fmt.Errorf("error downloading Object %s: %v", s3Job.Key, err)
									errorCh <- reportError
									progressManager.ReportError(s3Job.Output, reportError)
								} else {
									logger.Debug().Str("output", entry.OutputPath).Msg("S3 Download completed successfully")
								}
								close(s3FolderProgressCh)
							}
						}(i+1, s3JobsCh)
					}
					s3JobWg.Wait()
					s3wg.Wait()
					progressWg.Wait()

				// FTP and FTPS download
				// =================================================================================================================
				case "ftp":
					// TODO

				// SFTP download
				// =================================================================================================================
				case "sftp":
					// TODO

				// Google Drive download
				// =================================================================================================================
				case "gdrive":
					logger.Debug().Str("url", entry.URL).Msg("Google Drive URL detected")
					simpleClient := utils.CreateHTTPClient(config.Timeout, config.KATimeout, config.ProxyURL, false, nil)
					apiKey, err := danzogdrive.GetAuthToken()
					if err != nil {
						logger.Debug().Err(err).Msg("Failed to get API key")
						errorCh <- fmt.Errorf("error getting API key: %v", err)
						continue
					}
					metadata, fileID, err := danzogdrive.GetFileMetadata(entry.URL, simpleClient, apiKey)
					if err != nil {
						logger.Debug().Err(err).Msg("Failed to get file metadata")
						errorCh <- fmt.Errorf("error getting file metadata: %v", err)
						continue
					}
					if config.OutputPath == "" {
						inferredFileName := utils.RenewOutputPath(metadata["name"].(string))
						config.OutputPath = inferredFileName
						entry.OutputPath = inferredFileName
					}
					fileSize := metadata["size"].(string)
					fileSizeInt, err := strconv.ParseInt(fileSize, 10, 64)
					if err != nil {
						progressManager.Register(entry.OutputPath, -1) // -1 means unknown size
					} else {
						progressManager.Register(entry.OutputPath, fileSizeInt)
					}

					var progressWg sync.WaitGroup
					progressWg.Add(1)
					go func(outputPath string, progCh <-chan int64) {
						defer progressWg.Done()
						var totalDownloaded int64
						for bytesDownloaded := range progCh {
							progressManager.Update(outputPath, bytesDownloaded)
							totalDownloaded += bytesDownloaded
						}
						progressManager.Complete(outputPath, totalDownloaded)
					}(entry.OutputPath, progressCh)

					err = danzogdrive.PerformGDriveDownload(config, apiKey, fileID, simpleClient, progressCh)
					if err != nil {
						logger.Debug().Err(err).Msg("Download failed")
						reportError := fmt.Errorf("error downloading %s: %v", entry.URL, err)
						errorCh <- reportError
						progressManager.ReportError(entry.OutputPath, reportError)
					} else {
						logger.Debug().Str("output", entry.OutputPath).Msg("Download completed successfully")
					}
					close(progressCh)
					progressWg.Wait()
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
