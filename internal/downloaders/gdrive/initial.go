package gdrive

import (
	"fmt"
	"os"

	"github.com/tanq16/danzo/internal/utils"
)

type GDriveDownloader struct{}

func (d *GDriveDownloader) ValidateJob(job *utils.DanzoJob) error {
	// Extract file/folder ID from URL
	fileID, err := extractFileID(job.URL)
	if err != nil {
		return err
	}
	job.Metadata["fileID"] = fileID

	// Validate auth method
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

	return nil
}

func (d *GDriveDownloader) BuildJob(job *utils.DanzoJob) error {
	fileID := job.Metadata["fileID"].(string)

	// Get auth token
	var token string
	var err error

	if apiKey, ok := job.Metadata["apiKey"].(string); ok {
		token = apiKey
	} else if credFile, ok := job.Metadata["credentialsFile"].(string); ok {
		job.PauseFunc()
		token, err = getAccessTokenFromCredentials(credFile)
		job.ResumeFunc()
		if err != nil {
			return fmt.Errorf("error getting OAuth token: %v", err)
		}
	}
	job.Metadata["token"] = token

	// Get metadata
	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	metadata, _, err := getFileMetadata(job.URL, client, token)
	if err != nil {
		return fmt.Errorf("error getting metadata: %v", err)
	}

	// Check if it's a folder
	mimeType, _ := metadata["mimeType"].(string)
	if mimeType == "application/vnd.google-apps.folder" {
		job.Metadata["isFolder"] = true
		// List folder contents
		files, err := listFolderContents(fileID, token, client)
		if err != nil {
			return fmt.Errorf("error listing folder contents: %v", err)
		}
		job.Metadata["folderFiles"] = files

		// Calculate total size
		var totalSize int64
		for _, file := range files {
			if size, ok := file["size"].(string); ok {
				if sizeInt, err := parseSize(size); err == nil {
					totalSize += sizeInt
				}
			}
		}
		job.Metadata["totalSize"] = totalSize

		// Set output path as folder name
		if job.OutputPath == "" {
			job.OutputPath = metadata["name"].(string)
		}
	} else {
		job.Metadata["isFolder"] = false
		// Single file
		if job.OutputPath == "" {
			job.OutputPath = metadata["name"].(string)
		}

		// Get file size
		if sizeStr, ok := metadata["size"].(string); ok {
			size, _ := parseSize(sizeStr)
			job.Metadata["totalSize"] = size
		}
	}

	// Check if output exists
	if info, err := os.Stat(job.OutputPath); err == nil {
		if job.Metadata["isFolder"].(bool) && info.IsDir() {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		} else if !job.Metadata["isFolder"].(bool) && !info.IsDir() {
			job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		}
	}

	return nil
}
