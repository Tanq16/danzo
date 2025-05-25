package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/tanq16/danzo/internal/utils"
)

type S3Client struct {
	client *s3.Client
}

type s3Object struct {
	Key  string
	Size int64
}

func getS3Client(profile string) (*S3Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithSharedConfigProfile(profile),
		config.WithRetryMode("adaptive"),
	)
	if err != nil {
		return nil, fmt.Errorf("error loading AWS config: %v", err)
	}

	return &S3Client{
		client: s3.NewFromConfig(cfg),
	}, nil
}

func getS3ObjectInfo(bucket, key string, client *S3Client) (string, int64, error) {
	// Try HEAD request first
	headObj, err := client.client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	if err == nil {
		// It's a file
		size := int64(0)
		if headObj.ContentLength != nil {
			size = *headObj.ContentLength
		}
		return "file", size, nil
	}

	// Check if it's a folder by listing with prefix
	result, err := client.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(key),
		MaxKeys: aws.Int32(1),
	})

	if err != nil {
		return "", 0, fmt.Errorf("error accessing S3 object: %v", err)
	}

	if len(result.Contents) > 0 || len(result.CommonPrefixes) > 0 {
		return "folder", -1, nil
	}

	return "", 0, fmt.Errorf("S3 object not found")
}

func listS3Objects(bucket, prefix string, client *S3Client) ([]s3Object, error) {
	var objects []s3Object

	paginator := s3.NewListObjectsV2Paginator(client.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("error listing objects: %v", err)
		}

		for _, obj := range page.Contents {
			if obj.Key != nil && obj.Size != nil {
				// Skip directories (0-byte objects ending with /)
				if *obj.Size == 0 && strings.HasSuffix(*obj.Key, "/") {
					continue
				}
				objects = append(objects, s3Object{
					Key:  *obj.Key,
					Size: *obj.Size,
				})
			}
		}
	}

	return objects, nil
}

func performS3Download(bucket, key, outputPath string, client *S3Client, progressCh chan<- int64) error {
	// Get object
	result, err := client.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("error getting object: %v", err)
	}
	defer result.Body.Close()

	// Create output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	// Download with progress
	buffer := make([]byte, utils.DefaultBufferSize)
	for {
		n, err := result.Body.Read(buffer)
		if n > 0 {
			_, writeErr := file.Write(buffer[:n])
			if writeErr != nil {
				return fmt.Errorf("error writing file: %v", writeErr)
			}
			progressCh <- int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading object: %v", err)
		}
	}

	return nil
}

// Helper functions that might be missing from utils package
func directoryExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}

func createDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}
