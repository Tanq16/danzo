package danzohttp

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
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
	log.Debug().Str("op", "http/initial").Msgf("Sending HEAD request to %s", job.URL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error checking URL: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound {
		if location := resp.Header.Get("Location"); location != "" {
			log.Debug().Str("op", "http/initial").Msgf("URL redirected to %s", location)
			job.URL = location
		}
	} else if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("URL not found (404)")
	} else if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned error: %d", resp.StatusCode)
	}
	log.Info().Str("op", "http/initial").Msgf("job validated for %s", job.URL)
	return nil
}

func (d *HTTPDownloader) BuildJob(job *utils.DanzoJob) error {
	job.HTTPClientConfig.HighThreadMode = job.Connections > 5
	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	fileSize, fileName, err := getFileInfo(job.URL, client)
	if err != nil && err != utils.ErrRangeRequestsNotSupported {
		return fmt.Errorf("error getting file info: %v", err)
	}
	log.Debug().Str("op", "http/initial").Msgf("File info retrieved: size=%d, name=%s, rangeSupported=%v", fileSize, fileName, err != utils.ErrRangeRequestsNotSupported)

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

	if existingFile, err := os.Stat(job.OutputPath); err == nil {
		if fileSize > 0 && existingFile.Size() == fileSize {
			return fmt.Errorf("file already exists with same size")
		}
		job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		log.Debug().Str("op", "http/initial").Msgf("Output path renewed to %s", job.OutputPath)
	}
	job.Metadata["fileSize"] = fileSize
	job.Metadata["rangeSupported"] = err != utils.ErrRangeRequestsNotSupported
	log.Info().Str("op", "http/initial").Msgf("job built for %s", job.URL)
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
					if job.ProgressFunc != nil {
						job.ProgressFunc(totalDownloaded, fileSize)
					}
					return
				}
				totalDownloaded += bytes

			case <-ticker.C:
				if totalDownloaded > lastBytes {
					if job.ProgressFunc != nil {
						job.ProgressFunc(totalDownloaded, fileSize)
					}
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

	var err error
	if !rangeSupported || job.Connections == 1 {
		log.Debug().Str("op", "http/initial").Msg("Using simple downloader (range not supported or 1 connection)")
		err = PerformSimpleDownload(job.URL, job.OutputPath, client, progressCh)
	} else if fileSize/int64(job.Connections) < 2*utils.DefaultBufferSize {
		log.Debug().Str("op", "http/initial").Msg("Using simple downloader (chunk size too small)")
		err = PerformSimpleDownload(job.URL, job.OutputPath, client, progressCh)
	} else {
		log.Debug().Str("op", "http/initial").Msg("Using multi-chunk downloader")
		config := utils.HTTPDownloadConfig{
			URL:              job.URL,
			OutputPath:       job.OutputPath,
			Connections:      job.Connections,
			HTTPClientConfig: job.HTTPClientConfig,
		}
		err = PerformMultiDownload(config, client, fileSize, progressCh)
	}

	// Close progress channel and wait for final update
	// close(progressCh)
	<-progressDone

	job.Metadata["totalDownloaded"] = fileSize
	job.Metadata["totalTime"] = time.Since(startTime).Seconds()
	return err
}

func getFileInfo(link string, client *utils.DanzoHTTPClient) (int64, string, error) {
	req, err := http.NewRequest("HEAD", link, nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	filename := ""
	filenameRegex := regexp.MustCompile(`[^a-zA-Z0-9_\-\. ]+`)
	if contentDisposition := resp.Header.Get("Content-Disposition"); contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if fn, ok := params["filename"]; ok && fn != "" {
				filename = filenameRegex.ReplaceAllString(fn, "_")
			} else if fn, ok := params["filename*"]; ok && fn != "" {
				if strings.HasPrefix(fn, "UTF-8''") {
					unescaped, _ := url.PathUnescape(strings.TrimPrefix(fn, "UTF-8''"))
					filename = filenameRegex.ReplaceAllString(unescaped, "_")
				}
			}
		}
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		return 0, filename, utils.ErrRangeRequestsNotSupported
	}
	contentLength := resp.Header.Get("Content-Length")
	if contentLength == "" {
		return 0, filename, errors.New("server didn't provide Content-Length header")
	}
	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return 0, filename, err
	}
	if size <= 0 {
		return 0, filename, errors.New("invalid file size reported by server")
	}
	return size, filename, nil
}
