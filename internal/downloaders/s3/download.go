package s3

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

func (d *S3Downloader) Download(job *utils.DanzoJob) error {
	bucket := job.Metadata["bucket"].(string)
	key := job.Metadata["key"].(string)
	fileType := job.Metadata["fileType"].(string)
	profile := job.Metadata["profile"].(string)
	s3Client, err := getS3Client(profile)
	if err != nil {
		return fmt.Errorf("error creating S3 client: %v", err)
	}
	if fileType == "folder" {
		log.Info().Str("op", "s3/download").Msgf("Starting folder download for s3://%s/%s", bucket, key)
		return d.downloadFolder(job, bucket, key, s3Client)
	} else {
		log.Info().Str("op", "s3/download").Msgf("Starting file download for s3://%s/%s", bucket, key)
		return d.downloadFile(job, bucket, key, s3Client)
	}
}

func (d *S3Downloader) downloadFile(job *utils.DanzoJob, bucket, key string, s3Client *S3Client) error {
	size := job.Metadata["size"].(int64)
	progressCh := make(chan int64, 100)
	defer close(progressCh)
	go func() {
		var totalDownloaded int64
		for bytes := range progressCh {
			totalDownloaded += bytes
			if job.ProgressFunc != nil {
				job.ProgressFunc(totalDownloaded, size)
			}
		}
	}()
	return performS3Download(bucket, key, job.OutputPath, s3Client, progressCh)
}

func (d *S3Downloader) downloadFolder(job *utils.DanzoJob, bucket, prefix string, s3Client *S3Client) error {
	objects, err := listS3Objects(bucket, prefix, s3Client)
	if err != nil {
		return fmt.Errorf("error listing objects: %v", err)
	}
	if len(objects) == 0 {
		return fmt.Errorf("no objects found in s3://%s/%s", bucket, prefix)
	}
	log.Debug().Str("op", "s3/download").Msgf("Found %d objects to download in folder", len(objects))
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
	numWorkers := min(job.Connections, len(objects))
	log.Debug().Str("op", "s3/download").Msgf("Using %d parallel workers for folder download", numWorkers)

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for obj := range jobCh {
				// Create relative path for output
				relPath := strings.TrimPrefix(obj.Key, prefix)
				relPath = strings.TrimPrefix(relPath, "/")
				outputPath := filepath.Join(job.OutputPath, relPath)
				// Create directory if needed
				if err := createDirectory(filepath.Dir(outputPath)); err != nil {
					mu.Lock()
					if downloadErr == nil {
						downloadErr = fmt.Errorf("error creating directory: %v", err)
					}
					mu.Unlock()
					return
				}
				// Track progress
				progressCh := make(chan int64, 100)
				go func(ch <-chan int64) {
					for bytes := range ch {
						downloaded := atomic.AddInt64(&totalDownloaded, bytes)
						if job.ProgressFunc != nil {
							job.ProgressFunc(downloaded, totalSize)
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
