package internal

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	danzohttp "github.com/tanq16/danzo/downloaders/http"
	"github.com/tanq16/danzo/utils"
)

func BatchDownload(entries []utils.DownloadEntry, numLinks, connectionsPerLink int, httpClientConfig utils.HTTPClientConfig) error {
	outputMgr := utils.NewManager(10)
	outputMgr.StartDisplay()
	defer func() {
		outputMgr.StopDisplay()
		for _, entry := range entries {
			utils.Clean(entry.OutputPath)
		}
	}()

	var wg sync.WaitGroup
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
			for entry := range entriesCh {
				utils.PrintDebug(fmt.Sprintf("\nWorker %d processing entry: %s\n", workerID, entry.URL))
				entryFunctionId := outputMgr.Register(entry.OutputPath)
				config := utils.DownloadConfig{
					URL:              entry.URL,
					OutputPath:       entry.OutputPath,
					Connections:      connectionsPerLink,
					HTTPClientConfig: httpClientConfig,
				}
				progressCh := make(chan int64)
				useHighThreadMode := config.Connections > 5

				urlType := utils.DetermineDownloadType(config.URL)
				// Set custom headers and user agent only for HTTP/HTTPS downloads
				if urlType != "http" {
					httpClientConfig.Headers = nil
					httpClientConfig.UserAgent = utils.ToolUserAgent
				}
				outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Starting %s download for %s", urlType, entry.OutputPath))
				switch urlType {

				// =================================================================================================================
				// DOWNLOAD TYPES USED BY DANZO ====================================================================================
				// EACH DOWNLOAD TYPE HAS A SPECIFIC HANDLER OR PROCESSOR ==========================================================
				// HANDLERS ARE TRIGGERED IN THE SWITCH CASE BASED ON TYPE =========================================================
				// =================================================================================================================

				// HTTP download
				// =================================================================================================================
				case "http":
					client := utils.CreateHTTPClient(httpClientConfig, useHighThreadMode)
					fileSize, fileName, err := utils.GetFileInfo(config.URL, client)
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
								continue
							}
							config.OutputPath = utils.RenewOutputPath(config.OutputPath)
							entry.OutputPath = config.OutputPath
						}
					}

					if err == utils.ErrRangeRequestsNotSupported {
						outputMgr.SetStatus(entryFunctionId, "warning")
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading %s with single connection (range requests not supported)", entry.URL))
					} else if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error getting file size for %s: %v", entry.URL, err))
						continue
					} else {
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading %s (%s)", entry.OutputPath, utils.FormatBytes(uint64(fileSize))))
					}

					// Internal goroutine to forward progress updates to the manager
					var progressWg sync.WaitGroup
					progressWg.Add(1)
					go func(funcId string, totalFileSize int64, progCh <-chan int64) {
						defer progressWg.Done()
						var totalDownloaded int64
						for bytesDownloaded := range progCh {
							totalDownloaded += bytesDownloaded
							progressString := fmt.Sprintf("%s / %s", utils.FormatBytes(uint64(totalDownloaded)), utils.FormatBytes(uint64(totalFileSize)))
							outputMgr.AddProgressBarToStream(funcId, totalDownloaded, totalFileSize, progressString)
						}
					}(entryFunctionId, fileSize, progressCh)

					if err == utils.ErrRangeRequestsNotSupported || config.Connections == 1 {
						simpleClient := utils.CreateHTTPClient(httpClientConfig, false)
						err = danzohttp.PerformSimpleDownload(entry.URL, entry.OutputPath, simpleClient, progressCh)
						close(progressCh)
					} else if fileSize/int64(config.Connections) < 2*utils.DefaultBufferSize {
						simpleClient := utils.CreateHTTPClient(httpClientConfig, false)
						err = danzohttp.PerformSimpleDownload(entry.URL, entry.OutputPath, simpleClient, progressCh)
						close(progressCh)
					} else {
						err = danzohttp.PerformMultiDownload(config, client, fileSize, progressCh)
					}
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading %s: %v", entry.URL, err))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("Download completed for %s", entry.OutputPath))
					}
					// close(progressCh) // Closing the progress channel here would cause a panic (Multi-Download already closes it)
					progressWg.Wait()

				// YouTube download (with yt-dlp as dependency)
				// =================================================================================================================
				// case "youtube":
				// 	processedURL, format, dType, output, err := danzoyoutube.ProcessURL(entry.URL)
				// 	if err != nil {
				// 		errorCh <- fmt.Errorf("error processing YouTube URL %s: %v", entry.URL, err)
				// 		continue
				// 	}
				// 	if config.OutputPath == "" {
				// 		config.OutputPath = output
				// 		entry.OutputPath = output
				// 	} else {
				// 		existingFile, _ := os.Stat(config.OutputPath)
				// 		if existingFile != nil {
				// 			if existingFile.Size() > 0 {
				// 				continue
				// 			}
				// 			config.OutputPath = utils.RenewOutputPath(config.OutputPath)
				// 			entry.OutputPath = config.OutputPath
				// 		} else {
				// 			entry.OutputPath = config.OutputPath
				// 		}
				// 	}
				// 	entry.URL = processedURL
				// 	// For YouTube, we register with unknown size and only update at the end
				// 	progressManager.Register(entry.OutputPath, -1)
				// 	streamCh := make(chan []string, 7)

				// 	// Internal goroutine to forward progress updates to the manager
				// 	var progressWg sync.WaitGroup
				// 	progressWg.Add(1)
				// 	go func(outputPath string, progCh <-chan int64) {
				// 		defer progressWg.Done()
				// 		var totalDownloaded int64
				// 		for bytesDownloaded := range progCh {
				// 			totalDownloaded += bytesDownloaded
				// 		}
				// 		progressManager.Complete(outputPath, totalDownloaded)
				// 	}(entry.OutputPath, progressCh)

				// 	// Goroutine to forward streaming output to the manager
				// 	var streamWg sync.WaitGroup
				// 	streamWg.Add(1)
				// 	go func(outputPath string, streamCh <-chan []string) {
				// 		defer streamWg.Done()
				// 		for streamOutput := range streamCh {
				// 			progressManager.UpdateStreamOutput(outputPath, streamOutput)
				// 		}
				// 	}(entry.OutputPath, streamCh)

				// 	err = danzoyoutube.DownloadYouTubeVideo(entry.URL, entry.OutputPath, format, dType, progressCh, streamCh)
				// 	if err != nil {
				// 		reportError := fmt.Errorf("error downloading %s: %v", entry.URL, err)
				// 		errorCh <- reportError
				// 		progressManager.ReportError(entry.OutputPath, reportError)
				// 	}
				// 	close(progressCh)
				// 	close(streamCh)
				// 	progressWg.Wait()
				// 	streamWg.Wait()

				// GitHub Release download
				// =================================================================================================================
				// case "gitrelease":
				// 	simpleClient := utils.CreateHTTPClient(httpClientConfig, false)
				// 	userSelectOverride := false
				// 	if len(entries) > 1 {
				// 		userSelectOverride = true
				// 	}
				// 	downloadURL, filename, size, err := danzogitr.ProcessRelease(entry.URL, userSelectOverride, simpleClient)
				// 	if err != nil {
				// 		errorCh <- fmt.Errorf("error processing GitHub release URL %s: %v", entry.URL, err)
				// 		continue
				// 	}

				// 	if config.OutputPath == "" {
				// 		config.OutputPath = filename
				// 		entry.OutputPath = filename
				// 	}
				// 	progressManager.Register(entry.OutputPath, size)

				// 	// Internal goroutine to forward progress updates to the manager
				// 	var progressWg sync.WaitGroup
				// 	progressWg.Add(1)
				// 	go func(outputPath string, progCh <-chan int64) {
				// 		defer progressWg.Done()
				// 		var totalDownloaded int64
				// 		for bytesDownloaded := range progCh {
				// 			progressManager.Update(outputPath, bytesDownloaded)
				// 			totalDownloaded += bytesDownloaded
				// 		}
				// 		progressManager.Complete(outputPath, totalDownloaded)
				// 	}(entry.OutputPath, progressCh)

				// 	err = danzohttp.PerformSimpleDownload(downloadURL, entry.OutputPath, simpleClient, progressCh)
				// 	if err != nil {
				// 		reportError := fmt.Errorf("error downloading %s: %v", entry.URL, err)
				// 		errorCh <- reportError
				// 		progressManager.ReportError(entry.OutputPath, reportError)
				// 	}
				// 	close(progressCh)
				// 	progressWg.Wait()

				// Git clone download
				// =================================================================================================================
				// case "gitclone":
				// 	if config.OutputPath == "" {
				// 		urlParts := strings.Split(entry.URL, "/")
				// 		if len(urlParts) >= 2 {
				// 			tempName := strings.Split(urlParts[len(urlParts)-1], "||")
				// 			config.OutputPath = tempName[0]
				// 			entry.OutputPath = config.OutputPath
				// 		} else {
				// 			config.OutputPath = "git-repo"
				// 			entry.OutputPath = "git-repo"
				// 		}
				// 	}
				// 	existingFile, _ := os.Stat(config.OutputPath)
				// 	if existingFile != nil {
				// 		config.OutputPath = utils.RenewOutputPath(config.OutputPath)
				// 		entry.OutputPath = config.OutputPath
				// 	}
				// 	parsedURL, depth, err := danzogitc.InitGitClone(entry.URL, config.OutputPath)
				// 	if err != nil {
				// 		errorCh <- fmt.Errorf("error checking Git clone %s: %v", entry.URL, err)
				// 		continue
				// 	}

				// 	progressManager.Register(entry.OutputPath, -1)
				// 	streamCh := make(chan []string, 5)

				// 	// Internal goroutine to forward progress updates to the manager
				// 	var progressWg sync.WaitGroup
				// 	progressWg.Add(1)
				// 	go func(outputPath string, progCh <-chan int64) {
				// 		defer progressWg.Done()
				// 		var totalSize int64
				// 		for size := range progCh {
				// 			totalSize += size
				// 		}
				// 		progressManager.Complete(outputPath, totalSize)
				// 	}(entry.OutputPath, progressCh)

				// 	// Goroutine to forward streaming output to the manager
				// 	var streamWg sync.WaitGroup
				// 	streamWg.Add(1)
				// 	go func(outputPath string, streamCh <-chan []string) {
				// 		defer streamWg.Done()
				// 		for streamOutput := range streamCh {
				// 			progressManager.UpdateStreamOutput(outputPath, streamOutput)
				// 		}
				// 	}(entry.OutputPath, streamCh)

				// 	err = danzogitc.CloneRepository(parsedURL, config.OutputPath, progressCh, streamCh, depth)
				// 	if err != nil {
				// 		reportError := fmt.Errorf("error cloning %s: %v", entry.URL, err)
				// 		errorCh <- reportError
				// 		progressManager.ReportError(entry.OutputPath, reportError)
				// 	}

				// 	close(progressCh)
				// 	close(streamCh)
				// 	progressWg.Wait()
				// 	streamWg.Wait()

				// AWS S3 download
				// =================================================================================================================
				// case "s3":
				// 	s3client, err := danzos3.GetS3Client()
				// 	if err != nil {
				// 		errorCh <- fmt.Errorf("error getting S3 client: %v", err)
				// 		continue
				// 	}
				// 	bucket, key, fileType, size, err := danzos3.GetS3ObjectInfo(entry.URL, s3client)
				// 	if err != nil {
				// 		errorCh <- fmt.Errorf("error getting S3 object info: %v", err)
				// 		continue
				// 	}

				// 	var S3Jobs []danzos3.S3Job
				// 	if fileType != "folder" {
				// 		S3Jobs = append(S3Jobs, danzos3.S3Job{
				// 			Bucket: bucket,
				// 			Key:    key,
				// 			Output: strings.Split(key, "/")[len(strings.Split(key, "/"))-1],
				// 			Size:   size,
				// 		})
				// 	} else {
				// 		S3Jobs, err = danzos3.GetAllObjectsFromFolder(bucket, key, s3client)
				// 		if err != nil {
				// 			errorCh <- fmt.Errorf("error listing S3 objects in folder: %v", err)
				// 			continue
				// 		}
				// 	}

				// 	// Close the progress channel because it's not used in S3 downloads
				// 	close(progressCh)
				// 	// Do all S3 downloads with connectionsPerLink number of parallel downloads
				// 	// NOTE: This is non-standard usage of connectionsPerLink (usually, it signifies connections per link)
				// 	var s3wg sync.WaitGroup
				// 	var progressWg sync.WaitGroup
				// 	s3Workers := 0
				// 	if len(S3Jobs) < connectionsPerLink {
				// 		s3Workers = len(S3Jobs)
				// 	} else {
				// 		s3Workers = connectionsPerLink
				// 	}
				// 	s3JobsCh := make(chan danzos3.S3Job, len(S3Jobs))
				// 	var s3JobWg sync.WaitGroup
				// 	s3JobWg.Add(1)
				// 	go func() {
				// 		defer s3JobWg.Done()
				// 		for _, s3Job := range S3Jobs {
				// 			s3JobsCh <- s3Job
				// 		}
				// 		close(s3JobsCh)
				// 	}()
				// 	for i := range s3Workers {
				// 		s3wg.Add(1)
				// 		go func(workerID int, s3JobsCh <-chan danzos3.S3Job) {
				// 			defer s3wg.Done()
				// 			for s3Job := range s3JobsCh {
				// 				progressManager.Register(s3Job.Output, s3Job.Size)
				// 				s3FolderProgressCh := make(chan int64)
				// 				progressWg.Add(1)
				// 				go func(outputPath string, progCh <-chan int64) {
				// 					defer progressWg.Done()
				// 					var totalDownloaded int64
				// 					for bytesDownloaded := range progCh {
				// 						if bytesDownloaded < 0 {
				// 							progressManager.Register(outputPath, -bytesDownloaded)
				// 							continue
				// 						}
				// 						progressManager.Update(outputPath, bytesDownloaded)
				// 						totalDownloaded += bytesDownloaded
				// 					}
				// 					progressManager.Complete(outputPath, totalDownloaded)
				// 				}(s3Job.Output, s3FolderProgressCh)
				// 				err := danzos3.PerformS3ObjectDownload(s3Job.Bucket, s3Job.Key, s3Job.Output, s3Job.Size, s3client, s3FolderProgressCh)
				// 				if err != nil {
				// 					reportError := fmt.Errorf("error downloading Object %s: %v", s3Job.Key, err)
				// 					errorCh <- reportError
				// 					progressManager.ReportError(s3Job.Output, reportError)
				// 				}
				// 				close(s3FolderProgressCh)
				// 			}
				// 		}(i+1, s3JobsCh)
				// 	}
				// 	s3JobWg.Wait()
				// 	s3wg.Wait()
				// 	progressWg.Wait()

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
					// case "gdrive":
					// 	simpleClient := utils.CreateHTTPClient(httpClientConfig, false)
					// 	apiKey, err := danzogdrive.GetAuthToken()
					// 	if err != nil {
					// 		errorCh <- fmt.Errorf("error getting API key: %v", err)
					// 		continue
					// 	}
					// 	metadata, fileID, err := danzogdrive.GetFileMetadata(entry.URL, simpleClient, apiKey)
					// 	if err != nil {
					// 		errorCh <- fmt.Errorf("error getting file metadata: %v", err)
					// 		continue
					// 	}
					// 	if config.OutputPath == "" {
					// 		inferredFileName := utils.RenewOutputPath(metadata["name"].(string))
					// 		config.OutputPath = inferredFileName
					// 		entry.OutputPath = inferredFileName
					// 	}
					// 	fileSize := metadata["size"].(string)
					// 	fileSizeInt, err := strconv.ParseInt(fileSize, 10, 64)
					// 	if err != nil {
					// 		progressManager.Register(entry.OutputPath, -1) // -1 means unknown size
					// 	} else {
					// 		progressManager.Register(entry.OutputPath, fileSizeInt)
					// 	}

					// 	var progressWg sync.WaitGroup
					// 	progressWg.Add(1)
					// 	go func(outputPath string, progCh <-chan int64) {
					// 		defer progressWg.Done()
					// 		var totalDownloaded int64
					// 		for bytesDownloaded := range progCh {
					// 			progressManager.Update(outputPath, bytesDownloaded)
					// 			totalDownloaded += bytesDownloaded
					// 		}
					// 		progressManager.Complete(outputPath, totalDownloaded)
					// 	}(entry.OutputPath, progressCh)

					// 	err = danzogdrive.PerformGDriveDownload(config, apiKey, fileID, simpleClient, progressCh)
					// 	if err != nil {
					// 		reportError := fmt.Errorf("error downloading %s: %v", entry.URL, err)
					// 		errorCh <- reportError
					// 		progressManager.ReportError(entry.OutputPath, reportError)
					// 	}
					// 	close(progressCh)
					// 	progressWg.Wait()
				}
			}
		}(i + 1)
	}

	// Wait for all downloads to complete
	wg.Wait()
	return nil
}
