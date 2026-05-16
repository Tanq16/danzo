package danzohttp

import (
	"context"
	"encoding/json"
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

	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/utils"
)

type HTTPDownloadConfig struct {
	URL              string
	OutputPath       string
	Connections      int
	HTTPClientConfig utils.HTTPClientConfig
}

type HTTPDownloadChunk struct {
	ID         int
	StartByte  int64
	EndByte    int64
	Downloaded int64
	Completed  bool
	Retries    int
	LastError  error
	StartTime  time.Time
	FinishTime time.Time
}

type HTTPDownloadJob struct {
	Config    HTTPDownloadConfig
	FileSize  int64
	Chunks    []HTTPDownloadChunk
	StartTime time.Time
	TempFiles []string
}

type HTTPJob struct {
	URL         string
	OutputPath  string
	Connections int
	HTTPConfig  utils.HTTPClientConfig
}

type httpJobState struct {
	URL         string            `json:"url"`
	OutputPath  string            `json:"outputPath"`
	Connections int               `json:"connections"`
	ProxyURL    string            `json:"proxyURL,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func New(url, outputPath string, connections int, httpConfig utils.HTTPClientConfig) *HTTPJob {
	return &HTTPJob{
		URL:         url,
		OutputPath:  outputPath,
		Connections: connections,
		HTTPConfig:  httpConfig,
	}
}

func (j *HTTPJob) ID() string {
	if j.OutputPath != "" {
		return j.OutputPath
	}
	parsedURL, err := url.Parse(j.URL)
	if err != nil {
		return j.URL
	}
	parts := strings.Split(parsedURL.Path, "/")
	name := parts[len(parts)-1]
	if name == "" {
		return j.URL
	}
	return name
}

func (j *HTTPJob) Type() string { return "http" }

func (j *HTTPJob) Run(ctx context.Context, progress chan<- highway.Progress) error {
	parsedURL, err := url.Parse(j.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}

	client := utils.NewDanzoHTTPClient(j.HTTPConfig)
	req, err := http.NewRequestWithContext(ctx, "HEAD", j.URL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error checking URL: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound {
		if location := resp.Header.Get("Location"); location != "" {
			j.URL = location
		}
	} else if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("URL not found (404)")
	} else if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned error: %d", resp.StatusCode)
	}

	j.HTTPConfig.HighThreadMode = j.Connections > 5
	client = utils.NewDanzoHTTPClient(j.HTTPConfig)
	fileSize, fileName, err := getFileInfo(ctx, j.URL, client)
	rangeSupported := err != utils.ErrRangeRequestsNotSupported
	if err != nil && err != utils.ErrRangeRequestsNotSupported {
		return fmt.Errorf("error getting file info: %v", err)
	}

	if j.OutputPath == "" && fileName != "" {
		j.OutputPath = fileName
	} else if j.OutputPath == "" {
		pu, _ := url.Parse(j.URL)
		pathParts := strings.Split(pu.Path, "/")
		j.OutputPath = pathParts[len(pathParts)-1]
		if j.OutputPath == "" {
			j.OutputPath = "download"
		}
	}

	if existingFile, statErr := os.Stat(j.OutputPath); statErr == nil {
		if fileSize > 0 && existingFile.Size() == fileSize {
			progress <- highway.Progress{JobID: j.ID(), Done: true, Message: "Already exists"}
			return nil
		}
		j.OutputPath = utils.RenewOutputPath(j.OutputPath)
	}

	progress <- highway.Progress{
		JobID: j.ID(), Type: highway.ProgressTypeProgress,
		Message: "Downloading", Current: 0, Total: fileSize,
	}

	bytesCh := make(chan int64, 100)
	bytesDone := make(chan struct{})
	startTime := time.Now()

	go func() {
		defer close(bytesDone)
		var totalDownloaded int64
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case bytes, ok := <-bytesCh:
				if !ok {
					progress <- highway.Progress{
						JobID: j.ID(), Type: highway.ProgressTypeProgress,
						Message: "Downloading", Current: totalDownloaded, Total: fileSize,
						Extra: utils.FormatBytes(uint64(totalDownloaded)) + "/" + utils.FormatBytes(uint64(fileSize)),
					}
					return
				}
				totalDownloaded += bytes
			case <-ticker.C:
				if totalDownloaded > 0 {
					elapsed := time.Since(startTime).Seconds()
					speed := utils.FormatSpeed(totalDownloaded, elapsed)
					progress <- highway.Progress{
						JobID: j.ID(), Type: highway.ProgressTypeProgress,
						Message: "Downloading", Current: totalDownloaded, Total: fileSize,
						Extra: speed,
					}
				}
			}
		}
	}()

	var dlErr error
	if !rangeSupported || j.Connections == 1 {
		dlErr = PerformSimpleDownload(ctx, j.URL, j.OutputPath, client, bytesCh)
	} else if fileSize/int64(j.Connections) < 2*utils.DefaultBufferSize {
		dlErr = PerformSimpleDownload(ctx, j.URL, j.OutputPath, client, bytesCh)
	} else {
		config := HTTPDownloadConfig{
			URL:              j.URL,
			OutputPath:       j.OutputPath,
			Connections:      j.Connections,
			HTTPClientConfig: j.HTTPConfig,
		}
		dlErr = PerformMultiDownload(ctx, config, client, fileSize, bytesCh)
	}

	<-bytesDone

	if dlErr != nil {
		return dlErr
	}

	progress <- highway.Progress{JobID: j.ID(), Done: true}
	return nil
}

func (j *HTTPJob) Marshal() ([]byte, error) {
	return json.Marshal(httpJobState{
		URL:         j.URL,
		OutputPath:  j.OutputPath,
		Connections: j.Connections,
		ProxyURL:    j.HTTPConfig.ProxyURL,
		UserAgent:   j.HTTPConfig.UserAgent,
		Headers:     j.HTTPConfig.Headers,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state httpJobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URL, state.OutputPath, state.Connections, utils.HTTPClientConfig{
		ProxyURL:  state.ProxyURL,
		UserAgent: state.UserAgent,
		Headers:   state.Headers,
	}), nil
}

func getFileInfo(ctx context.Context, link string, client *utils.DanzoHTTPClient) (int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", link, nil)
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
