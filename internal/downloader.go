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
	danzogitc "github.com/tanq16/danzo/downloaders/gitclone"
	danzogitr "github.com/tanq16/danzo/downloaders/gitrelease"
	danzohttp "github.com/tanq16/danzo/downloaders/http"
	danzom3u8 "github.com/tanq16/danzo/downloaders/m3u8"
	danzos3 "github.com/tanq16/danzo/downloaders/s3"
	danzoyoutube "github.com/tanq16/danzo/downloaders/youtube"
	"github.com/tanq16/danzo/internal/utils"
)

func BatchDownload(entries []utils.DownloadEntry, numLinks, connectionsPerLink int, httpClientConfig utils.HTTPClientConfig, unlimitOut bool) error {
	outputMgr := utils.NewManager(10)
	if unlimitOut {
		outputMgr.SetUnlimitedOutput(unlimitOut)
		outputMgr.SetUpdateInterval(5 * time.Second)
	}
	fmt.Println()
	outputMgr.StartDisplay()
	var funcTrackerMutex sync.Mutex
	outputMgrFuncTracker := map[string]string{} // Only for http downloads
	defer func() {
		outputMgr.StopDisplay()
		for _, entry := range entries {
			if _, ok := outputMgrFuncTracker[entry.OutputPath]; ok {
				if outputMgr.GetStatus(outputMgrFuncTracker[entry.OutputPath]) == "error" {
					continue // Skip cleaning if there was an error
				}
			}
			utils.CleanFunction(entry.OutputPath)
		}
	}()

	// Disperse entries for job distribution
	var wg sync.WaitGroup
	entriesCh := make(chan utils.DownloadEntry, len(entries))
	for _, entry := range entries {
		entriesCh <- entry
	}
	close(entriesCh)

	// Start worker goroutines to handle jobs
	for i := range numLinks {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for entry := range entriesCh {
				entryFunctionId := outputMgr.Register(entry.OutputPath)
				funcTrackerMutex.Lock()
				outputMgrFuncTracker[entry.OutputPath] = entryFunctionId
				funcTrackerMutex.Unlock()
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
				initialMessage := fmt.Sprintf("Starting %s download for %s", urlType, entry.OutputPath)
				if entry.OutputPath == "" {
					shortenedURL := config.URL
					if len(shortenedURL) > 40 {
						shortenedURL = "..." + entry.URL[len(entry.URL)-40:]
					}
					initialMessage = fmt.Sprintf("Starting %s download for %s", urlType, shortenedURL)
				}
				outputMgr.SetMessage(entryFunctionId, initialMessage)
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
					funcTrackerMutex.Lock()
					outputMgrFuncTracker[entry.OutputPath] = entryFunctionId
					funcTrackerMutex.Unlock()

					if err == utils.ErrRangeRequestsNotSupported {
						outputMgr.SetStatus(entryFunctionId, "warning")
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("[range unsupported] Downloading %s with 1-thread (%s)", entry.OutputPath, utils.FormatBytes(uint64(fileSize))))
					} else if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error getting file size for %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error getting file size for %s", entry.OutputPath))
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
							// progressString := , utils.FormatBytes(uint64(totalFileSize)))
							outputMgr.AddProgressBarToStream(funcId, totalDownloaded, totalFileSize, utils.FormatBytes(uint64(totalDownloaded)))
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
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error downloading %s", entry.OutputPath))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("Download completed for %s", entry.OutputPath))
					}
					// close(progressCh) // Closing here causes a panic because Multi-Download already closes it
					progressWg.Wait()

				// YouTube download (with yt-dlp as dependency)
				// =================================================================================================================
				case "youtube":
					close(progressCh) // Note needed for youtube downloads
					processedURL, format, dType, output, err := danzoyoutube.ProcessURL(entry.URL)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error processing YouTube URL %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error processing YouTube URL %s", entry.OutputPath))
						continue
					}
					if config.OutputPath == "" {
						config.OutputPath = output
						entry.OutputPath = output
					} else {
						existingFile, _ := os.Stat(config.OutputPath)
						if existingFile != nil {
							if existingFile.Size() > 0 {
								continue
							}
							config.OutputPath = utils.RenewOutputPath(config.OutputPath)
							entry.OutputPath = config.OutputPath
						} else {
							entry.OutputPath = config.OutputPath
						}
					}
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading %s", entry.OutputPath))
					entry.URL = processedURL
					streamCh := make(chan []string, 7)

					// Goroutine to forward streaming output to the manager
					var streamWg sync.WaitGroup
					streamWg.Add(1)
					go func(outputPath string, streamCh <-chan []string) {
						defer streamWg.Done()
						for streamOutput := range streamCh {
							outputMgr.UpdateStreamOutput(entryFunctionId, streamOutput)
						}
					}(entry.OutputPath, streamCh)

					err = danzoyoutube.DownloadYouTubeVideo(entry.URL, entry.OutputPath, format, dType, streamCh)
					close(streamCh)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error downloading %s", entry.OutputPath))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("Download completed for %s", entry.OutputPath))
					}
					streamWg.Wait()

				// GitHub Release download
				// =================================================================================================================
				case "gitrelease":
					simpleClient := utils.CreateHTTPClient(httpClientConfig, false)
					userSelectOverride := false
					if len(entries) > 1 {
						userSelectOverride = true
					}

					// input control (to pause manager)
					needInputCh := make(chan bool)
					inputDoneCh := make(chan bool)
					var inputWg sync.WaitGroup
					inputWg.Add(1)
					go func() {
						defer inputWg.Done()
						for {
							select {
							case _, ok := <-needInputCh:
								if !ok {
									return
								}
								outputMgr.Pause()
							case _, ok := <-inputDoneCh:
								if !ok {
									return
								}
								outputMgr.Resume()
							}
						}
					}()

					downloadURL, filename, size, err := danzogitr.ProcessRelease(entry.URL, userSelectOverride, simpleClient, needInputCh, inputDoneCh)
					close(needInputCh)
					close(inputDoneCh)
					inputWg.Wait()
					outputMgr.Resume()

					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error processing GitHub release URL %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error processing GitHub release URL %s", entry.OutputPath))
						continue
					}
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading %s (%s)", filename, utils.FormatBytes(uint64(size))))

					if config.OutputPath == "" {
						config.OutputPath = filename
						entry.OutputPath = filename
					}

					// Internal goroutine to forward progress updates to the manager
					var progressWg sync.WaitGroup
					progressWg.Add(1)
					go func(outputPath string, totalFileSize int64, progCh <-chan int64) {
						defer progressWg.Done()
						var totalDownloaded int64
						for bytesDownloaded := range progCh {
							totalDownloaded += bytesDownloaded
							// progressString := fmt.Sprintf("%s / %s", , utils.FormatBytes(uint64(totalFileSize)))
							outputMgr.AddProgressBarToStream(entryFunctionId, totalDownloaded, totalFileSize, utils.FormatBytes(uint64(totalDownloaded)))
						}
					}(entry.OutputPath, size, progressCh)

					err = danzohttp.PerformSimpleDownload(downloadURL, entry.OutputPath, simpleClient, progressCh)
					close(progressCh)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error downloading %s", entry.OutputPath))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("Download completed for %s", entry.OutputPath))
					}
					progressWg.Wait()

				// Git clone download
				// =================================================================================================================
				case "gitclone":
					if config.OutputPath == "" {
						urlParts := strings.Split(entry.URL, "/")
						if len(urlParts) >= 2 {
							tempName := strings.Split(urlParts[len(urlParts)-1], "||")
							config.OutputPath = tempName[0]
							entry.OutputPath = config.OutputPath
						} else {
							config.OutputPath = "git-repo"
							entry.OutputPath = "git-repo"
						}
					}
					existingFile, _ := os.Stat(config.OutputPath)
					if existingFile != nil {
						config.OutputPath = utils.RenewOutputPath(config.OutputPath)
						entry.OutputPath = config.OutputPath
					}
					parsedURL, depth, err := danzogitc.InitGitClone(entry.URL, config.OutputPath)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error checking Git clone %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error checking Git clone %s", entry.OutputPath))
						continue
					}
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Cloning %s to %s", parsedURL, entry.OutputPath))
					streamCh := make(chan string)

					// Internal goroutine to forward progress updates to the manager
					var progressWg sync.WaitGroup
					progressWg.Add(1)
					go func(outputPath string, progCh <-chan int64) {
						defer progressWg.Done()
						var totalSize int64
						for size := range progCh {
							totalSize += size
						}
					}(entry.OutputPath, progressCh)

					// Goroutine to forward streaming output to the manager
					var streamWg sync.WaitGroup
					streamWg.Add(1)
					go func(outputPath string, streamCh <-chan string) {
						defer streamWg.Done()
						for streamOutput := range streamCh {
							outputMgr.AddStreamLine(entryFunctionId, streamOutput)
						}
					}(entry.OutputPath, streamCh)

					err = danzogitc.CloneRepository(parsedURL, config.OutputPath, progressCh, streamCh, depth)
					close(progressCh)
					close(streamCh)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error cloning %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error cloning %s", entry.OutputPath))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("Cloned repository - %s", entry.OutputPath))
					}

					progressWg.Wait()
					streamWg.Wait()

				// AWS S3 download
				// =================================================================================================================
				case "s3":
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading S3 key %s", entry.URL))
					s3client, err := danzos3.GetS3Client()
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error getting S3 client: %v", err))
						outputMgr.SetMessage(entryFunctionId, "Error getting S3 client")
						continue
					}
					bucket, key, fileType, size, err := danzos3.GetS3ObjectInfo(entry.URL, s3client)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error getting S3 object info: %v", err))
						outputMgr.SetMessage(entryFunctionId, "Error getting S3 object info")
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
							outputMgr.ReportError(entryFunctionId, fmt.Errorf("error listing S3 objects in folder: %v", err))
							outputMgr.SetMessage(entryFunctionId, "Error listing S3 objects in folder")
							continue
						}
					}

					// Do all S3 jobs with connectionsPerLink number of parallel downloads (only for folders)
					var s3wg sync.WaitGroup
					var progressWg sync.WaitGroup
					s3Workers := 0
					s3Workers = min(len(S3Jobs), connectionsPerLink)

					// Internal goroutine to distribute S3 jobs
					s3JobsCh := make(chan danzos3.S3Job, len(S3Jobs))
					var totoalJobSize int64
					for _, s3Job := range S3Jobs {
						s3JobsCh <- s3Job
						totoalJobSize += s3Job.Size
					}
					close(s3JobsCh)
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading S3 key %s (%s)", entry.URL, utils.FormatBytes(uint64(totoalJobSize))))

					// Internal goroutine to forward progress updates to the manager
					progressWg.Add(1)
					go func(outputPath string, totalFileSize int64, progCh <-chan int64) {
						defer progressWg.Done()
						var totalDownloaded int64
						for bytesDownloaded := range progCh {
							totalDownloaded += bytesDownloaded
							// progressString := fmt.Sprintf("%s / %s", utils.FormatBytes(uint64(totalDownloaded)), utils.FormatBytes(uint64(totalFileSize)))
							outputMgr.AddProgressBarToStream(entryFunctionId, totalDownloaded, totalFileSize, utils.FormatBytes(uint64(totalDownloaded)))
						}
					}(entry.OutputPath, totoalJobSize, progressCh)

					// Internal goroutine to gather error messages
					errorCh := make(chan error)
					var errorWg sync.WaitGroup
					errorWg.Add(1)
					var s3Error []error
					go func() {
						defer errorWg.Done()
						for err := range errorCh {
							if err != nil {
								s3Error = append(s3Error, err)
							}
						}
					}()

					// Start S3 workers
					for i := range s3Workers {
						s3wg.Add(1)
						go func(workerID int, s3JobsCh <-chan danzos3.S3Job, errorCh chan<- error) {
							defer s3wg.Done()
							for s3Job := range s3JobsCh {
								err := danzos3.PerformS3ObjectDownload(s3Job.Bucket, s3Job.Key, s3Job.Output, s3Job.Size, s3client, progressCh)
								if err != nil {
									errorCh <- fmt.Errorf("error downloading Object %s: %v", s3Job.Key, err)
								}
							}
						}(i+1, s3JobsCh, errorCh)
					}
					s3wg.Wait()
					close(progressCh)
					close(errorCh)
					var s3ErrorJoined error
					for _, ee := range s3Error {
						if s3ErrorJoined == nil {
							s3ErrorJoined = ee
						} else {
							s3ErrorJoined = fmt.Errorf("%w; %v", s3ErrorJoined, ee)
						}
					}
					if s3ErrorJoined != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading S3 object %s: %v", entry.URL, s3ErrorJoined))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error downloading S3 object %s", entry.URL))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("S3 download completed for %s", entry.URL))
					}
					errorWg.Wait()
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
					simpleClient := utils.CreateHTTPClient(httpClientConfig, false)
					outputMgr.Pause()
					apiKey, err := danzogdrive.GetAuthToken()
					outputMgr.Resume()
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error getting API key: %v", err))
						outputMgr.SetMessage(entryFunctionId, "Error getting API key")
						continue
					}
					metadata, fileID, err := danzogdrive.GetFileMetadata(entry.URL, simpleClient, apiKey)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error getting file metadata: %v", err))
						outputMgr.SetMessage(entryFunctionId, "Error getting file metadata")
						continue
					}
					if config.OutputPath == "" {
						config.OutputPath = metadata["name"].(string)
						if existingFile, _ := os.Stat(config.OutputPath); existingFile != nil {
							config.OutputPath = utils.RenewOutputPath(config.OutputPath)
						}
						entry.OutputPath = config.OutputPath
					}
					fileSize := metadata["size"].(string)
					fileSizeInt, _ := strconv.ParseInt(fileSize, 10, 64)
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Downloading GDrive file %s (%s)", entry.OutputPath, utils.FormatBytes(uint64(fileSizeInt))))

					var progressWg sync.WaitGroup
					progressWg.Add(1)
					go func(outputPath string, filesize int64, progCh <-chan int64) {
						defer progressWg.Done()
						var totalDownloaded int64
						for bytesDownloaded := range progCh {
							totalDownloaded += bytesDownloaded
							// progressString := fmt.Sprintf("%s / %s", utils.FormatBytes(uint64(totalDownloaded)), utils.FormatBytes(uint64(filesize)))
							outputMgr.AddProgressBarToStream(entryFunctionId, totalDownloaded, filesize, utils.FormatBytes(uint64(totalDownloaded)))
						}
					}(entry.OutputPath, fileSizeInt, progressCh)

					err = danzogdrive.PerformGDriveDownload(config, apiKey, fileID, simpleClient, progressCh)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading GDrive file %s: %v", entry.OutputPath, err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error downloading GDrive file %s", entry.OutputPath))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("Completed GDrive download - %s", entry.OutputPath))
					}
					close(progressCh)
					progressWg.Wait()

				// M3U8 Stream download
				// =================================================================================================================
				case "m3u8":
					close(progressCh) // Not needed for M3U8 downloads
					outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Processing M3U8 stream: %s", entry.OutputPath))
					if config.OutputPath == "" {
						config.OutputPath = fmt.Sprintf("stream_%s.mp4", time.Now().Format("2006-01-02_15-04"))
						entry.OutputPath = config.OutputPath
					} else {
						existingFile, _ := os.Stat(config.OutputPath)
						if existingFile != nil {
							config.OutputPath = utils.RenewOutputPath(config.OutputPath)
							entry.OutputPath = config.OutputPath
						}
					}
					client := utils.CreateHTTPClient(httpClientConfig, false)
					streamCh := make(chan string)

					// Goroutine to stream output to the manager
					var streamWg sync.WaitGroup
					streamWg.Add(1)
					go func(outputPath string, streamCh <-chan string) {
						defer streamWg.Done()
						for streamOutput := range streamCh {
							outputMgr.AddStreamLine(entryFunctionId, streamOutput)
						}
					}(entry.OutputPath, streamCh)

					err := danzom3u8.PerformM3U8Download(config, client, streamCh)
					close(streamCh)
					if err != nil {
						outputMgr.ReportError(entryFunctionId, fmt.Errorf("error downloading M3U8 stream: %v", err))
						outputMgr.SetMessage(entryFunctionId, fmt.Sprintf("Error downloading M3U8 stream: %s", entry.OutputPath))
					} else {
						outputMgr.Complete(entryFunctionId, fmt.Sprintf("M3U8 stream downloaded: %s", entry.OutputPath))
					}
					streamWg.Wait()
				}
			}
		}(i + 1)
	}

	// Wait for all downloads to complete
	wg.Wait()
	return nil
}
