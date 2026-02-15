package m3u8

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/internal/utils"
)

type LiveStreamJob struct {
	URL         string
	OutputPath  string
	Connections int
	Extractor   string
	HTTPConfig  utils.HTTPClientConfig
}

type liveStreamJobState struct {
	URL         string            `json:"url"`
	OutputPath  string            `json:"outputPath"`
	Connections int               `json:"connections"`
	Extractor   string            `json:"extractor,omitempty"`
	ProxyURL    string            `json:"proxyURL,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func New(urlStr, outputPath string, connections int, extractor string, httpConfig utils.HTTPClientConfig) *LiveStreamJob {
	return &LiveStreamJob{
		URL:         urlStr,
		OutputPath:  outputPath,
		Connections: connections,
		Extractor:   extractor,
		HTTPConfig:  httpConfig,
	}
}

func (j *LiveStreamJob) ID() string {
	if j.OutputPath != "" {
		return j.OutputPath
	}
	return j.URL
}

func (j *LiveStreamJob) Type() string { return "live-stream" }

func (j *LiveStreamJob) Run(ctx context.Context, progress chan<- highway.Progress) error {
	if j.Extractor == "" {
		parsedURL, err := url.Parse(j.URL)
		if err != nil {
			return fmt.Errorf("invalid URL: %v", err)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
		}
	}

	if err := runExtractor(&j.URL, j.Extractor, j.HTTPConfig); err != nil {
		return fmt.Errorf("extractor failed: %v", err)
	}

	if j.OutputPath == "" {
		j.OutputPath = fmt.Sprintf("stream_%s.mp4", time.Now().Format("2006-01-02_15-04"))
	}
	if existingFile, err := os.Stat(j.OutputPath); err == nil && existingFile != nil {
		j.OutputPath = utils.RenewOutputPath(j.OutputPath)
	}
	tempDir := filepath.Join(filepath.Dir(j.OutputPath), ".danzo-temp", "m3u8_"+time.Now().Format("20060102150405"))

	client := utils.NewDanzoHTTPClient(j.HTTPConfig)
	manifestContent, err := getM3U8Contents(j.URL, client)
	if err != nil {
		return fmt.Errorf("error fetching manifest: %v", err)
	}
	m3u8Info, err := parseM3U8Content(manifestContent, j.URL, client)
	if err != nil {
		return fmt.Errorf("error processing manifest: %v", err)
	}

	if len(m3u8Info.VideoSegmentURLs) == 0 {
		return fmt.Errorf("no video segments found in manifest")
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	var downloadErr error
	defer func() {
		if downloadErr == nil {
			os.RemoveAll(tempDir)
		}
	}()

	if m3u8Info.HasSeparateAudio {
		downloadErr = j.downloadAndMergeSeparateStreams(ctx, progress, m3u8Info, tempDir, client)
	} else {
		downloadErr = j.downloadAndMergeSingleStream(ctx, progress, m3u8Info, tempDir, client)
	}

	if downloadErr != nil {
		return downloadErr
	}

	progress <- highway.Progress{JobID: j.ID(), Done: true}
	return nil
}

func (j *LiveStreamJob) downloadAndMergeSingleStream(_ context.Context, progress chan<- highway.Progress, m3u8Info *M3U8Info, tempDir string, client *utils.DanzoHTTPClient) error {
	segmentURLs := m3u8Info.VideoSegmentURLs
	totalSize, _, err := calculateTotalSize(segmentURLs, j.Connections, client)
	if err != nil {
		totalSize = int64(len(segmentURLs)) * 1024 * 1024
	}

	isFMP4 := detectFMP4Format(j.URL, segmentURLs)

	var totalDownloaded int64
	wrappedProgressFunc := func(incrementalSize, _ int64) {
		newTotal := atomic.AddInt64(&totalDownloaded, incrementalSize)
		progress <- highway.Progress{
			JobID: j.ID(), Type: highway.ProgressTypeProgress,
			Message: "Downloading", Current: newTotal, Total: totalSize,
			Extra: utils.FormatBytes(uint64(newTotal)) + "/" + utils.FormatBytes(uint64(totalSize)),
		}
	}

	segmentFiles, err := downloadSegmentsParallel(segmentURLs, tempDir, j.Connections, client, wrappedProgressFunc, totalSize, isFMP4)
	if err != nil {
		return fmt.Errorf("error downloading segments: %v", err)
	}
	if err := mergeSegments(segmentFiles, j.OutputPath, isFMP4, m3u8Info.VideoInitSegment, tempDir, client); err != nil {
		return fmt.Errorf("error merging segments: %v", err)
	}
	return nil
}

func (j *LiveStreamJob) downloadAndMergeSeparateStreams(_ context.Context, progress chan<- highway.Progress, m3u8Info *M3U8Info, tempDir string, client *utils.DanzoHTTPClient) error {
	videoDir := filepath.Join(tempDir, "video")
	audioDir := filepath.Join(tempDir, "audio")
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		return fmt.Errorf("error creating video directory: %v", err)
	}
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		return fmt.Errorf("error creating audio directory: %v", err)
	}

	videoSegmentURLs := m3u8Info.VideoSegmentURLs
	audioSegmentURLs := m3u8Info.AudioSegmentURLs

	totalVideoSize, _, err := calculateTotalSize(videoSegmentURLs, j.Connections, client)
	if err != nil {
		totalVideoSize = int64(len(videoSegmentURLs)) * 1024 * 1024
	}
	totalAudioSize, _, err := calculateTotalSize(audioSegmentURLs, j.Connections, client)
	if err != nil {
		totalAudioSize = int64(len(audioSegmentURLs)) * 512 * 1024
	}
	totalSize := totalVideoSize + totalAudioSize

	isVideoFMP4 := detectFMP4Format(j.URL, videoSegmentURLs)
	isAudioFMP4 := detectFMP4Format(j.URL, audioSegmentURLs)

	var totalDownloaded int64
	wrappedProgressFunc := func(incrementalDownloaded, _ int64) {
		newTotal := atomic.AddInt64(&totalDownloaded, incrementalDownloaded)
		progress <- highway.Progress{
			JobID: j.ID(), Type: highway.ProgressTypeProgress,
			Message: "Downloading", Current: newTotal, Total: totalSize,
			Extra: utils.FormatBytes(uint64(newTotal)) + "/" + utils.FormatBytes(uint64(totalSize)),
		}
	}

	videoFiles, videoErr := downloadSegmentsParallel(videoSegmentURLs, videoDir, j.Connections, client, wrappedProgressFunc, totalVideoSize, isVideoFMP4)
	audioFiles, audioErr := downloadSegmentsParallel(audioSegmentURLs, audioDir, j.Connections, client, wrappedProgressFunc, totalAudioSize, isAudioFMP4)

	if videoErr != nil && audioErr != nil {
		return fmt.Errorf("both video and audio downloads failed - video: %v, audio: %v", videoErr, audioErr)
	}

	tempVideoPath := filepath.Join(tempDir, "video_temp.mp4")
	tempAudioPath := filepath.Join(tempDir, "audio_temp.m4a")

	var finalErr error

	if videoErr == nil {
		if err := mergeSegments(videoFiles, tempVideoPath, isVideoFMP4, m3u8Info.VideoInitSegment, videoDir, client); err != nil {
			return fmt.Errorf("error merging video segments: %v", err)
		}
	}

	if audioErr == nil {
		if err := mergeSegments(audioFiles, tempAudioPath, isAudioFMP4, m3u8Info.AudioInitSegment, audioDir, client); err != nil {
			if videoErr == nil {
				if err := os.Rename(tempVideoPath, j.OutputPath); err != nil {
					return fmt.Errorf("error saving video-only output: %v", err)
				}
				return fmt.Errorf("audio merge failed, saved video-only: %v", err)
			}
			return fmt.Errorf("error merging audio segments: %v", err)
		}
	}

	if videoErr == nil && audioErr == nil {
		if err := mergeVideoAndAudio(tempVideoPath, tempAudioPath, j.OutputPath); err != nil {
			return fmt.Errorf("error merging video and audio: %v", err)
		}
	} else if videoErr == nil && audioErr != nil {
		if err := os.Rename(tempVideoPath, j.OutputPath); err != nil {
			return fmt.Errorf("error saving video-only output: %v", err)
		}
		finalErr = fmt.Errorf("audio download failed: %v", audioErr)
	} else if audioErr == nil && videoErr != nil {
		if err := os.Rename(tempAudioPath, j.OutputPath); err != nil {
			return fmt.Errorf("error saving audio-only output: %v", err)
		}
		finalErr = fmt.Errorf("video download failed: %v", videoErr)
	}

	return finalErr
}

func (j *LiveStreamJob) Marshal() ([]byte, error) {
	return json.Marshal(liveStreamJobState{
		URL:         j.URL,
		OutputPath:  j.OutputPath,
		Connections: j.Connections,
		Extractor:   j.Extractor,
		ProxyURL:    j.HTTPConfig.ProxyURL,
		UserAgent:   j.HTTPConfig.UserAgent,
		Headers:     j.HTTPConfig.Headers,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state liveStreamJobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URL, state.OutputPath, state.Connections, state.Extractor, utils.HTTPClientConfig{
		ProxyURL:  state.ProxyURL,
		UserAgent: state.UserAgent,
		Headers:   state.Headers,
	}), nil
}
