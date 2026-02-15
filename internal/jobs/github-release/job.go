package ghrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	danzohttp "github.com/tanq16/danzo/internal/jobs/http"
	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/internal/utils"
)

type GHReleaseJob struct {
	URL           string
	OutputPath    string
	Manual        bool
	HTTPConfig    utils.HTTPClientConfig
	PauseDisplay  func()
	ResumeDisplay func()
}

type ghReleaseJobState struct {
	URL        string            `json:"url"`
	OutputPath string            `json:"outputPath"`
	Manual     bool              `json:"manual"`
	ProxyURL   string            `json:"proxyURL,omitempty"`
	UserAgent  string            `json:"userAgent,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

func New(url, outputPath string, manual bool, httpConfig utils.HTTPClientConfig) *GHReleaseJob {
	return &GHReleaseJob{
		URL:        url,
		OutputPath: outputPath,
		Manual:     manual,
		HTTPConfig: httpConfig,
	}
}

func (j *GHReleaseJob) ID() string {
	if j.OutputPath != "" {
		return j.OutputPath
	}
	return j.URL
}

func (j *GHReleaseJob) Type() string { return "github-release" }

func (j *GHReleaseJob) Run(ctx context.Context, progress chan<- highway.Progress) error {
	owner, repo, err := parseGitHubURL(j.URL)
	if err != nil {
		return err
	}

	client := utils.NewDanzoHTTPClient(j.HTTPConfig)
	assets, tagName, err := getGitHubReleaseAssets(owner, repo, client)
	if err != nil {
		return fmt.Errorf("error fetching release info: %v", err)
	}

	downloadURL, size, err := selectGitHubLatestAsset(assets)
	if err != nil {
		return err
	}
	if downloadURL == "" && !j.Manual {
		return fmt.Errorf("could not automatically select asset for platform %s/%s, use --manual flag", runtime.GOOS, runtime.GOARCH)
	}
	if j.Manual {
		if j.PauseDisplay != nil {
			j.PauseDisplay()
		}
		downloadURL, size, err = promptGitHubAssetSelection(assets, tagName)
		if j.ResumeDisplay != nil {
			j.ResumeDisplay()
		}
		if err != nil {
			return err
		}
	}

	urlParts := strings.Split(downloadURL, "/")
	filename := urlParts[len(urlParts)-1]
	if j.OutputPath == "" {
		j.OutputPath = filename
	}

	progress <- highway.Progress{
		JobID: j.ID(), Type: highway.ProgressTypeProgress,
		Message: "Downloading", Current: 0, Total: size,
	}

	bytesCh := make(chan int64)
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
						Message: "Downloading", Current: totalDownloaded, Total: size,
						Extra: utils.FormatBytes(uint64(totalDownloaded)) + "/" + utils.FormatBytes(uint64(size)),
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
						Message: "Downloading", Current: totalDownloaded, Total: size,
						Extra: speed,
					}
				}
			}
		}
	}()

	dlErr := danzohttp.PerformSimpleDownload(downloadURL, j.OutputPath, client, bytesCh)
	<-bytesDone

	if dlErr != nil {
		return dlErr
	}

	progress <- highway.Progress{JobID: j.ID(), Done: true}
	return nil
}

func (j *GHReleaseJob) Marshal() ([]byte, error) {
	return json.Marshal(ghReleaseJobState{
		URL:        j.URL,
		OutputPath: j.OutputPath,
		Manual:     j.Manual,
		ProxyURL:   j.HTTPConfig.ProxyURL,
		UserAgent:  j.HTTPConfig.UserAgent,
		Headers:    j.HTTPConfig.Headers,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state ghReleaseJobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URL, state.OutputPath, state.Manual, utils.HTTPClientConfig{
		ProxyURL:  state.ProxyURL,
		UserAgent: state.UserAgent,
		Headers:   state.Headers,
	}), nil
}
