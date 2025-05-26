package s3

import (
	"fmt"
	"strings"

	"github.com/tanq16/danzo/internal/utils"
)

type S3Downloader struct{}

func (d *S3Downloader) ValidateJob(job *utils.DanzoJob) error {
	// Parse S3 URL - supports both s3://bucket/key and bucket/key formats
	bucket, key, err := parseS3URL(job.URL)
	if err != nil {
		return err
	}

	// Store parsed values
	job.Metadata["bucket"] = bucket
	job.Metadata["key"] = key

	return nil
}

func (d *S3Downloader) BuildJob(job *utils.DanzoJob) error {
	bucket := job.Metadata["bucket"].(string)
	key := job.Metadata["key"].(string)
	profile := job.Metadata["profile"].(string)

	// Get S3 client with profile
	s3Client, err := getS3Client(profile)
	if err != nil {
		return fmt.Errorf("error creating S3 client: %v", err)
	}

	// Check if it's a file or folder
	fileType, size, err := getS3ObjectInfo(bucket, key, s3Client)
	if err != nil {
		return fmt.Errorf("error getting S3 object info: %v", err)
	}

	job.Metadata["fileType"] = fileType
	job.Metadata["size"] = size

	// Set output path if not specified
	if job.OutputPath == "" {
		if fileType == "folder" {
			// For folders, use the key as directory name
			parts := strings.Split(strings.TrimSuffix(key, "/"), "/")
			job.OutputPath = parts[len(parts)-1]
			if job.OutputPath == "" {
				job.OutputPath = bucket
			}
		} else {
			// For files, use the filename
			parts := strings.Split(key, "/")
			job.OutputPath = parts[len(parts)-1]
		}
	}

	// Check if output already exists
	if fileType == "folder" {
		// For folders, check directory
		if exists, err := directoryExists(job.OutputPath); err == nil && exists {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		}
	} else {
		// For files, check file
		if exists, err := fileExists(job.OutputPath); err == nil && exists {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		}
	}

	return nil
}

func parseS3URL(url string) (string, string, error) {
	// Remove s3:// prefix if present
	url = strings.TrimPrefix(url, "s3://")

	// Split bucket and key
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
