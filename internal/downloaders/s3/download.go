package s3

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tanq16/danzo/internal/utils"
)

func (d *S3Downloader) Download(job *utils.DanzoJob) error {
	bucket := job.Metadata["bucket"].(string)
	key := job.Metadata["key"].(string)
	fileType := job.Metadata["fileType"].(string)
	profile := job.Metadata["profile"].(string)

	// Get S3 client
	s3Client, err := getS3Client(profile)
	if err != nil {
		return fmt.Errorf("error creating S3 client: %v", err)
	}

	if fileType == "folder" {
		return d.downloadFolder(job, bucket, key, s3Client)
	} else {
		return d.downloadFile(job, bucket, key, s3Client)
	}
}

func (d *S3Downloader) downloadFile(job *utils.DanzoJob, bucket, key string, s3Client *S3Client) error {
	size := job.Metadata["size"].(int64)

	progressCh := make(chan int64, 100)
	defer close(progressCh)

	// Progress tracking
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
	// List all objects in folder
	objects, err := listS3Objects(bucket, prefix, s3Client)
	if err != nil {
		return fmt.Errorf("error listing objects: %v", err)
	}

	if len(objects) == 0 {
		return fmt.Errorf("no objects found in s3://%s/%s", bucket, prefix)
	}

	// Calculate total size
	var totalSize int64
	for _, obj := range objects {
		totalSize += obj.Size
	}

	// Download objects in parallel
	var totalDownloaded int64
	var mu sync.Mutex
	var downloadErr error

	// Create download jobs
	jobCh := make(chan s3Object, len(objects))
	for _, obj := range objects {
		jobCh <- obj
	}
	close(jobCh)

	// Start workers
	numWorkers := job.Connections
	if numWorkers > len(objects) {
		numWorkers = len(objects)
	}

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
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

				// Download individual file
				progressCh := make(chan int64, 100)

				// Track progress
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
