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
	"github.com/tanq16/danzo/internal/utils"
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
	s3Options := func(o *s3.Options) {
		// Disable checksum validation warning
		o.DisableLogOutputChecksumValidationSkipped = true
	}
	return s3.NewFromConfig(cfg, s3Options), nil
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
					Output: filepath.Join(bucket, *obj.Key),
					Size:   int64(*obj.Size),
				})
			}
		}
	}
	return objects, nil
}
