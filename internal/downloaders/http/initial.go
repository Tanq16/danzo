package danzohttp

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tanq16/danzo/internal/utils"
)

type HTTPDownloader struct{}

func (d *HTTPDownloader) ValidateJob(job *utils.DanzoJob) error {
	parsedURL, err := url.Parse(job.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}

	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)

	req, err := http.NewRequest("HEAD", job.URL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error checking URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound {
		if location := resp.Header.Get("Location"); location != "" {
			job.URL = location
		}
	} else if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("URL not found (404)")
	} else if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned error: %d", resp.StatusCode)
	}

	return nil
}

func (d *HTTPDownloader) BuildJob(job *utils.DanzoJob) error {
	job.HTTPClientConfig.HighThreadMode = job.Connections > 5

	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)

	fileSize, fileName, err := utils.GetFileInfo(job.URL, client)
	if err != nil && err != utils.ErrRangeRequestsNotSupported {
		return fmt.Errorf("error getting file info: %v", err)
	}

	if job.OutputPath == "" && fileName != "" {
		job.OutputPath = fileName
	} else if job.OutputPath == "" {
		parsedURL, _ := url.Parse(job.URL)
		pathParts := strings.Split(parsedURL.Path, "/")
		job.OutputPath = pathParts[len(pathParts)-1]
		if job.OutputPath == "" {
			job.OutputPath = "download"
		}
	}

	// Check existing file
	if existingFile, err := os.Stat(job.OutputPath); err == nil {
		if fileSize > 0 && existingFile.Size() == fileSize {
			return fmt.Errorf("file already exists with same size")
		}
		job.OutputPath = utils.RenewOutputPath(job.OutputPath)
	}

	job.Metadata["fileSize"] = fileSize
	job.Metadata["rangeSupported"] = err != utils.ErrRangeRequestsNotSupported

	return nil
}

func (d *HTTPDownloader) Download(job *utils.DanzoJob) error {
	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)

	fileSize, _ := job.Metadata["fileSize"].(int64)
	rangeSupported, _ := job.Metadata["rangeSupported"].(bool)

	progressCh := make(chan int64, 100)

	progressDone := make(chan struct{})
	startTime := time.Now()

	go func() {
		defer close(progressDone)
		var totalDownloaded int64
		var lastUpdate time.Time
		var lastBytes int64

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case bytes, ok := <-progressCh:
				if !ok {
					// Final update when channel closes
					if job.ProgressFunc != nil {
						job.ProgressFunc(totalDownloaded, fileSize)
					}
					return
				}
				totalDownloaded += bytes

			case <-ticker.C:
				// Periodic update for smooth progress display
				if totalDownloaded > lastBytes {
					if job.ProgressFunc != nil {
						job.ProgressFunc(totalDownloaded, fileSize)
					}

					// Calculate and store speed
					elapsed := time.Since(lastUpdate).Seconds()
					if elapsed > 0 {
						speed := float64(totalDownloaded-lastBytes) / elapsed
						job.Metadata["downloadSpeed"] = speed
						job.Metadata["elapsedTime"] = time.Since(startTime).Seconds()
					}

					lastUpdate = time.Now()
					lastBytes = totalDownloaded
				}
			}
		}
	}()

	// Perform download
	var err error

	// Decide download strategy
	if !rangeSupported || job.Connections == 1 {
		err = PerformSimpleDownload(job.URL, job.OutputPath, client, progressCh)
	} else if fileSize/int64(job.Connections) < 2*utils.DefaultBufferSize {
		// Chunk size would be too small, use simple download
		err = PerformSimpleDownload(job.URL, job.OutputPath, client, progressCh)
	} else {
		// Use multi-connection download
		config := utils.DownloadConfig{
			URL:              job.URL,
			OutputPath:       job.OutputPath,
			Connections:      job.Connections,
			HTTPClientConfig: job.HTTPClientConfig,
		}
		err = PerformMultiDownload(config, client, fileSize, progressCh)
	}

	// Close progress channel and wait for final update
	close(progressCh)
	<-progressDone

	// Store final statistics
	job.Metadata["totalDownloaded"] = fileSize
	job.Metadata["totalTime"] = time.Since(startTime).Seconds()

	return err
}
