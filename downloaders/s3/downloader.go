package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/tanq16/danzo/utils"
)

type S3Job struct {
	Output string
	Size   int64
	Bucket string
	Key    string
}

func parseS3URL(rawURL string) (string, string, error) {
	parts := strings.SplitN(rawURL[5:], "/", 2)
	if len(parts) < 2 {
		return parts[0], "", nil
	}
	return parts[0], parts[1], nil
}

type S3ProgressWriter struct {
	writer     io.WriterAt
	progressCh chan<- int64
}

func (pw *S3ProgressWriter) WriteAt(p []byte, off int64) (int, error) {
	n, err := pw.writer.WriteAt(p, off)
	if n > 0 {
		pw.progressCh <- int64(n)
	}
	return n, err
}

func GetS3Client() (*s3.Client, error) {
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithSharedConfigProfile(profile), config.WithRetryMode("adaptive"))
	if err != nil {
		return nil, fmt.Errorf("error loading AWS config: %v", err)
	}
	return s3.NewFromConfig(cfg), nil
}

func GetS3ObjectInfo(url string, s3Client *s3.Client) (string, string, string, int64, error) {
	bucket, key, err := parseS3URL(url)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("error parsing S3 URL: %v", err)
	}
	headObj, err := s3Client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		if headObj.ContentLength == nil {
			return "", "", "", 0, fmt.Errorf("object size is nil")
		}
		return bucket, key, *headObj.ContentType, int64(*headObj.ContentLength), nil
	} else {
		// Check if it's a folder
		result, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:    aws.String(bucket),
			Prefix:    aws.String(key),
			MaxKeys:   aws.Int32(1),
			Delimiter: aws.String("/"),
		})
		if err != nil {
			return "", "", "", 0, fmt.Errorf("error checking if key is a folder: %v", err)
		}
		if len(result.Contents) > 0 || len(result.CommonPrefixes) > 0 {
			return bucket, key, "folder", -1, nil
		} else {
			return "", "", "", 0, fmt.Errorf("could not determine key type")
		}
	}
}

func PerformS3ObjectDownload(bucket, key, outputPath string, size int64, s3Client *s3.Client, progressCh chan<- int64) error {
	log := utils.GetLogger("s3object-download")
	log.Debug().Str("bucket", bucket).Str("key", key).Str("output", outputPath).Msg("Starting S3 download")
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer file.Close()

	downloader := manager.NewDownloader(s3Client, func(d *manager.Downloader) {
		d.PartSize = 2 * utils.DefaultBufferSize
		d.Concurrency = 4
		d.BufferProvider = manager.NewPooledBufferedWriterReadFromProvider(utils.DefaultBufferSize)
	})
	progressWriter := &S3ProgressWriter{
		writer:     file,
		progressCh: progressCh,
	}
	_, err = downloader.Download(context.Background(), progressWriter, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("error downloading S3 object: %v", err)
	}
	log.Debug().Str("bucket", bucket).Str("key", key).Str("output", outputPath).Msg("S3 download completed")
	return nil
}

func GetAllObjectsFromFolder(bucket, prefix string, s3Client *s3.Client) ([]S3Job, error) {
	var objects []S3Job
	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("error listing S3 objects: %v", err)
		}
		for _, obj := range page.Contents {
			if obj.Size != nil {
				objects = append(objects, S3Job{
					Bucket: bucket,
					Key:    *obj.Key,
					Output: *obj.Key,
					Size:   int64(*obj.Size),
				})
			}
		}
	}
	return objects, nil
}

// func PerformS3FolderDownload(bucket, prefix, outputBasePath string, s3Client *s3.Client, numWorkers int) error {
// 	log := utils.GetLogger("s3-folder")
// 	log.Debug().Str("bucket", bucket).Str("prefix", prefix).Str("outputBase", outputBasePath).Msg("Starting S3 folder download")
// 	if err := os.MkdirAll(outputBasePath, 0755); err != nil {
// 		return fmt.Errorf("error creating output directory: %v", err)
// 	}

// 	var objects []types.Object
// 	// var objects []s3.Object
// 	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
// 		Bucket: aws.String(bucket),
// 		Prefix: aws.String(prefix),
// 	})

// 	for paginator.HasMorePages() {
// 		page, err := paginator.NextPage(context.Background())
// 		if err != nil {
// 			return fmt.Errorf("error listing S3 objects: %v", err)
// 		}
// 		objects = append(objects, page.Contents...)
// 	}

// 	if len(objects) == 0 {
// 		return fmt.Errorf("no objects found at s3://%s/%s", bucket, prefix)
// 	}

// 	log.Debug().Int("objectCount", len(objects)).Msg("Found objects to download")

// 	// Create a worker pool for downloading files
// 	var wg sync.WaitGroup
// 	jobs := make(chan types.Object, len(objects))
// 	errorCh := make(chan error, len(objects))

// 	// Start worker goroutines
// 	for i := 0; i < numWorkers; i++ {
// 		wg.Add(1)
// 		go func(workerID int) {
// 			defer wg.Done()
// 			workerLog := log.With().Int("workerID", workerID).Logger()

// 			// Create a progress channel for each worker
// 			workerProgressCh := make(chan int64)
// 			defer close(workerProgressCh)

// 			// Forward progress updates to the main progress channel
// 			go func() {
// 				for update := range workerProgressCh {
// 					progressCh <- update
// 				}
// 			}()

// 			for obj := range jobs {
// 				// Determine relative path by removing prefix
// 				relativePath := *obj.Key
// 				if prefix != "" {
// 					relativePath = strings.TrimPrefix(relativePath, prefix)
// 					relativePath = strings.TrimPrefix(relativePath, "/")
// 				}

// 				outputPath := filepath.Join(outputBasePath, relativePath)
// 				outputDir := filepath.Dir(outputPath)

// 				// Create output directory
// 				if err := os.MkdirAll(outputDir, 0755); err != nil {
// 					errorCh <- fmt.Errorf("error creating directory %s: %v", outputDir, err)
// 					continue
// 				}

// 				workerLog.Debug().Str("key", *obj.Key).Str("output", outputPath).Msg("Downloading object")

// 				// Download the object
// 				if err := PerformS3ObjectDownload(bucket, *obj.Key, outputPath, workerProgressCh); err != nil {
// 					errorCh <- fmt.Errorf("error downloading %s: %v", *obj.Key, err)
// 					continue
// 				}

// 				workerLog.Debug().Str("key", *obj.Key).Str("output", outputPath).Msg("Download completed")
// 			}
// 		}(i)
// 	}

// 	// Send jobs to workers
// 	for _, obj := range objects {
// 		jobs <- obj
// 	}
// 	close(jobs)

// 	// Wait for all downloads to complete
// 	wg.Wait()
// 	close(errorCh)

// 	// Collect errors
// 	var errors []error
// 	for err := range errorCh {
// 		errors = append(errors, err)
// 	}
// 	if len(errors) > 0 {
// 		return fmt.Errorf("folder download completed with %d errors: %v", len(errors), errors)
// 	}

// 	log.Debug().Int("objectCount", len(objects)).Str("outputBase", outputBasePath).Msg("S3 folder download completed")
// 	return nil
// }

// DetermineS3DownloadType checks if the key is an object or prefix
// func DetermineS3DownloadType(client *s3.Client, bucket, key string) (isFolder bool, err error) {
// 	// First, check if exact key exists (file)
// 	_, err = client.HeadObject(context.Background(), &s3.HeadObjectInput{
// 		Bucket: aws.String(bucket),
// 		Key:    aws.String(key),
// 	})
// 	if err == nil {
// 		return false, nil // Key exists, it's a file
// 	}

// 	// Check if it's a folder (has objects with this prefix)
// 	result, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
// 		Bucket:    aws.String(bucket),
// 		Prefix:    aws.String(key),
// 		MaxKeys:   aws.Int32(1),
// 		Delimiter: aws.String("/"),
// 	})
// 	if err != nil {
// 		return false, fmt.Errorf("error checking if key is a folder: %v", err)
// 	}

// 	// If there are contents or common prefixes, it's a folder
// 	return len(result.Contents) > 0 || len(result.CommonPrefixes) > 0, nil
// }

// PerformS3DownloadDispatch determines if the S3 path is a file or folder and handles accordingly
// func PerformS3DownloadDispatch(url string, outputPath string, numWorkers int, progressCh chan<- int64) error {
// 	log := utils.GetLogger("s3-dispatcher")

// 	// Parse S3 URL
// 	bucket, key, err := parseS3URL(url)
// 	if err != nil {
// 		return err
// 	}

// 	// Load AWS config
// 	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithSharedConfigProfile(os.Getenv("AWS_PROFILE")))
// 	if err != nil {
// 		return fmt.Errorf("error loading AWS config: %v", err)
// 	}
// 	s3Client := s3.NewFromConfig(cfg)

// 	// Determine if path is a folder or file
// 	isFolder, err := DetermineS3DownloadType(s3Client, bucket, key)
// 	if err != nil {
// 		return err
// 	}

// 	if isFolder {
// 		log.Debug().Str("bucket", bucket).Str("key", key).Bool("isFolder", isFolder).Msg("Detected folder path")
// 		return PerformS3FolderDownload(bucket, key, outputPath, numWorkers, progressCh)
// 	} else {
// 		log.Debug().Str("bucket", bucket).Str("key", key).Bool("isFolder", isFolder).Msg("Detected single object")
// 		return PerformS3Download(bucket, key, outputPath, progressCh)
// 	}
// }
