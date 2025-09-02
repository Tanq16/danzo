package gdrive

import (
	"fmt"
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

type GDriveDownloader struct{}

func (d *GDriveDownloader) ValidateJob(job *utils.DanzoJob) error {
	fileID, err := extractFileID(job.URL)
	if err != nil {
		return err
	}
	job.Metadata["fileID"] = fileID

	_, hasAPIKey := job.Metadata["apiKey"].(string)
	credentialsFile, hasCredsFile := job.Metadata["credentialsFile"].(string)
	if !hasAPIKey && !hasCredsFile {
		return fmt.Errorf("either --api-key or --credentials must be provided")
	}
	if hasAPIKey && hasCredsFile {
		return fmt.Errorf("only one of --api-key or --credentials can be provided")
	}
	if hasCredsFile {
		if _, err := os.Stat(credentialsFile); err != nil {
			return fmt.Errorf("credentials file not found: %v", err)
		}
	}
	log.Info().Str("op", "google-drive/initial").Msgf("job validated for %s", job.URL)
	return nil
}

func (d *GDriveDownloader) BuildJob(job *utils.DanzoJob) error {
	fileID := job.Metadata["fileID"].(string)
	var token string
	var err error
	if apiKey, ok := job.Metadata["apiKey"].(string); ok {
		log.Debug().Str("op", "google-drive/initial").Msgf("using API key")
		token = apiKey
	} else if credFile, ok := job.Metadata["credentialsFile"].(string); ok {
		job.PauseFunc()
		log.Debug().Str("op", "google-drive/initial").Msgf("using credentials file")
		token, err = getAccessTokenFromCredentials(credFile)
		job.ResumeFunc()
		if err != nil {
			return fmt.Errorf("error getting OAuth token: %v", err)
		}
	}
	log.Debug().Str("op", "google-drive/initial").Msgf("token retrieved")
	job.Metadata["token"] = token

	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	metadata, _, err := getFileMetadata(job.URL, client, token)
	if err != nil {
		return fmt.Errorf("error getting metadata: %v", err)
	}
	log.Debug().Str("op", "google-drive/initial").Msgf("retrieved item metadata")

	// Check if it's a folder
	mimeType, _ := metadata["mimeType"].(string)
	if mimeType == "application/vnd.google-apps.folder" {
		job.Metadata["isFolder"] = true
		log.Debug().Str("op", "google-drive/initial").Msgf("detected folder, listing contents")
		files, err := listFolderContents(fileID, token, client)
		if err != nil {
			return fmt.Errorf("error listing folder contents: %v", err)
		}
		job.Metadata["folderFiles"] = files
		var totalSize int64
		for _, file := range files {
			if size, ok := file["size"].(string); ok {
				if sizeInt, err := strconv.ParseInt(size, 10, 64); err == nil {
					totalSize += sizeInt
				}
			}
		}
		job.Metadata["totalSize"] = totalSize
		log.Debug().Str("op", "google-drive/initial").Msgf("recorded total size as %v", totalSize)
		if job.OutputPath == "" {
			job.OutputPath = metadata["name"].(string)
		}
	} else {
		log.Debug().Str("op", "google-drive/initial").Msgf("detected file")
		job.Metadata["isFolder"] = false
		if job.OutputPath == "" {
			job.OutputPath = metadata["name"].(string)
		}
		if sizeStr, ok := metadata["size"].(string); ok {
			size, _ := strconv.ParseInt(sizeStr, 10, 64)
			job.Metadata["totalSize"] = size
		}
	}
	if info, err := os.Stat(job.OutputPath); err == nil {
		if job.Metadata["isFolder"].(bool) && info.IsDir() {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		} else if !job.Metadata["isFolder"].(bool) && !info.IsDir() {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		}
	}
	log.Info().Str("op", "google-drive/initial").Msgf("job built for gdrive %s", fileID)
	return nil
}
