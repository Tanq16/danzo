package s3

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

type S3Downloader struct{}

func (d *S3Downloader) ValidateJob(job *utils.DanzoJob) error {
	bucket, key, err := parseS3URL(job.URL)
	if err != nil {
		return err
	}
	job.Metadata["bucket"] = bucket
	job.Metadata["key"] = key
	log.Info().Str("op", "s3/initial").Msgf("job validated for s3://%s/%s", bucket, key)
	return nil
}

func (d *S3Downloader) BuildJob(job *utils.DanzoJob) error {
	bucket := job.Metadata["bucket"].(string)
	key := job.Metadata["key"].(string)
	profile := job.Metadata["profile"].(string)
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
	log.Debug().Str("op", "s3/initial").Msgf("Determined object type: %s, size: %d", fileType, size)

	if job.OutputPath == "" {
		if fileType == "folder" {
			parts := strings.Split(strings.TrimSuffix(key, "/"), "/")
			job.OutputPath = parts[len(parts)-1]
			if job.OutputPath == "" {
				job.OutputPath = bucket
			}
		} else {
			parts := strings.Split(key, "/")
			job.OutputPath = parts[len(parts)-1]
		}
	}

	if fileType == "folder" {
		if exists, err := directoryExists(job.OutputPath); err == nil && exists {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		}
	} else {
		if exists, err := fileExists(job.OutputPath); err == nil && exists {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		}
	}
	log.Info().Str("op", "s3/initial").Msgf("job built for s3://%s/%s", bucket, key)
	return nil
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
