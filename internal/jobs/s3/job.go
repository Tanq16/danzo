package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/internal/utils"
)

type S3Job struct {
	URL         string
	OutputPath  string
	Connections int
	Profile     string
}

type s3JobState struct {
	URL         string `json:"url"`
	OutputPath  string `json:"outputPath"`
	Connections int    `json:"connections"`
	Profile     string `json:"profile"`
}

func New(url, outputPath string, connections int, profile string) *S3Job {
	return &S3Job{
		URL:         url,
		OutputPath:  outputPath,
		Connections: connections,
		Profile:     profile,
	}
}

func (j *S3Job) ID() string {
	if j.OutputPath != "" {
		return j.OutputPath
	}
	return j.URL
}

func (j *S3Job) Type() string { return "s3" }

func (j *S3Job) Run(ctx context.Context, progress chan<- highway.Progress) error {
	bucket, key, err := parseS3URL(j.URL)
	if err != nil {
		return err
	}

	s3Client, err := getS3Client(j.Profile)
	if err != nil {
		return fmt.Errorf("error creating S3 client: %v", err)
	}

	fileType, size, err := getS3ObjectInfo(bucket, key, s3Client)
	if err != nil {
		return fmt.Errorf("error getting S3 object info: %v", err)
	}

	if j.OutputPath == "" {
		if fileType == "folder" {
			parts := strings.Split(strings.TrimSuffix(key, "/"), "/")
			j.OutputPath = parts[len(parts)-1]
			if j.OutputPath == "" {
				j.OutputPath = bucket
			}
		} else {
			parts := strings.Split(key, "/")
			j.OutputPath = parts[len(parts)-1]
		}
	}

	if fileType == "folder" {
		if exists, err := directoryExists(j.OutputPath); err == nil && exists {
			j.OutputPath = utils.RenewOutputPath(j.OutputPath)
		}
	} else {
		if exists, err := fileExists(j.OutputPath); err == nil && exists {
			j.OutputPath = utils.RenewOutputPath(j.OutputPath)
		}
	}

	progress <- highway.Progress{
		JobID: j.ID(), Type: highway.ProgressTypeProgress,
		Message: "Downloading",
	}

	if fileType == "folder" {
		err = j.downloadFolder(ctx, progress, bucket, key, s3Client)
	} else {
		err = j.downloadFile(ctx, progress, bucket, key, size, s3Client)
	}

	if err != nil {
		return err
	}

	progress <- highway.Progress{JobID: j.ID(), Done: true}
	return nil
}

func (j *S3Job) downloadFile(_ context.Context, progress chan<- highway.Progress, bucket, key string, size int64, s3Client *S3Client) error {
	progressCh := make(chan int64, 100)
	defer close(progressCh)
	go func() {
		var totalDownloaded int64
		for bytes := range progressCh {
			totalDownloaded += bytes
			progress <- highway.Progress{
				JobID: j.ID(), Type: highway.ProgressTypeProgress,
				Message: "Downloading", Current: totalDownloaded, Total: size,
				Extra: utils.FormatBytes(uint64(totalDownloaded)) + "/" + utils.FormatBytes(uint64(size)),
			}
		}
	}()
	return performS3Download(bucket, key, j.OutputPath, s3Client, progressCh)
}

func (j *S3Job) downloadFolder(_ context.Context, progress chan<- highway.Progress, bucket, prefix string, s3Client *S3Client) error {
	objects, err := listS3Objects(bucket, prefix, s3Client)
	if err != nil {
		return fmt.Errorf("error listing objects: %v", err)
	}
	if len(objects) == 0 {
		return fmt.Errorf("no objects found in s3://%s/%s", bucket, prefix)
	}
	var totalSize int64
	for _, obj := range objects {
		totalSize += obj.Size
	}

	var totalDownloaded int64
	var mu sync.Mutex
	var downloadErr error
	jobCh := make(chan s3Object, len(objects))
	for _, obj := range objects {
		jobCh <- obj
	}
	close(jobCh)
	numWorkers := min(j.Connections, len(objects))

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for obj := range jobCh {
				relPath := strings.TrimPrefix(obj.Key, prefix)
				relPath = strings.TrimPrefix(relPath, "/")
				outputPath := filepath.Join(j.OutputPath, relPath)
				if err := createDirectory(filepath.Dir(outputPath)); err != nil {
					mu.Lock()
					if downloadErr == nil {
						downloadErr = fmt.Errorf("error creating directory: %v", err)
					}
					mu.Unlock()
					return
				}
				progressCh := make(chan int64, 100)
				go func(ch <-chan int64) {
					for bytes := range ch {
						downloaded := atomic.AddInt64(&totalDownloaded, bytes)
						progress <- highway.Progress{
							JobID: j.ID(), Type: highway.ProgressTypeProgress,
							Message: "Downloading", Current: downloaded, Total: totalSize,
							Extra: utils.FormatBytes(uint64(downloaded)) + "/" + utils.FormatBytes(uint64(totalSize)),
						}
					}
				}(progressCh)

				err := performS3Download(bucket, obj.Key, outputPath, s3Client, progressCh)
				close(progressCh)
				if err != nil {
					mu.Lock()
					if downloadErr == nil {
						downloadErr = fmt.Errorf("error downloading %s: %v", obj.Key, err)
					}
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	return downloadErr
}

func (j *S3Job) Marshal() ([]byte, error) {
	return json.Marshal(s3JobState{
		URL:         j.URL,
		OutputPath:  j.OutputPath,
		Connections: j.Connections,
		Profile:     j.Profile,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state s3JobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URL, state.OutputPath, state.Connections, state.Profile), nil
}

func parseS3URL(url string) (string, string, error) {
	url = strings.TrimPrefix(url, "s3://")
	parts := strings.SplitN(url, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		return "", "", fmt.Errorf("invalid S3 URL format")
	}
	bucket := parts[0]
	key := ""
	if len(parts) > 1 {
		key = parts[1]
	}
	return bucket, key, nil
}
