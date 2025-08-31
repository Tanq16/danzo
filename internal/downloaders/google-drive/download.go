package gdrive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	danzohttp "github.com/tanq16/danzo/internal/downloaders/http"
	"github.com/tanq16/danzo/internal/utils"
)

func (d *GDriveDownloader) Download(job *utils.DanzoJob) error {
	token := job.Metadata["token"].(string)
	isFolder := job.Metadata["isFolder"].(bool)
	totalSize := job.Metadata["totalSize"].(int64)
	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	if isFolder {
		return d.downloadFolder(job, token, client, totalSize)
	} else {
		return d.downloadFile(job, token, client, totalSize)
	}
}

func (d *GDriveDownloader) downloadFile(job *utils.DanzoJob, token string, client *utils.DanzoHTTPClient, totalSize int64) error {
	fileID := job.Metadata["fileID"].(string)
	progressCh := make(chan int64)
	go func() {
		var downloaded int64
		for bytes := range progressCh {
			downloaded += bytes
			if job.ProgressFunc != nil {
				job.ProgressFunc(downloaded, totalSize)
			}
		}
	}()

	config := utils.HTTPDownloadConfig{
		URL:              job.URL,
		OutputPath:       job.OutputPath,
		HTTPClientConfig: job.HTTPClientConfig,
	}
	return performGDriveDownload(config, token, fileID, client, progressCh)
}

func (d *GDriveDownloader) downloadFolder(job *utils.DanzoJob, token string, client *utils.DanzoHTTPClient, totalSize int64) error {
	files := job.Metadata["folderFiles"].([]map[string]any)
	if err := os.MkdirAll(job.OutputPath, 0755); err != nil {
		return fmt.Errorf("error creating folder: %v", err)
	}

	var totalDownloaded int64
	for _, file := range files {
		fileID := file["id"].(string)
		fileName := file["name"].(string)
		mimeType := file["mimeType"].(string)
		// Skip Google Docs files
		if strings.HasPrefix(mimeType, "application/vnd.google-apps.") {
			continue
		}
		outputPath := filepath.Join(job.OutputPath, fileName)
		progressCh := make(chan int64)
		go func(ch <-chan int64) {
			for bytes := range ch {
				totalDownloaded += bytes
				if job.ProgressFunc != nil {
					job.ProgressFunc(totalDownloaded, totalSize)
				}
			}
		}(progressCh)

		config := utils.HTTPDownloadConfig{
			URL:              fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID),
			OutputPath:       outputPath,
			HTTPClientConfig: job.HTTPClientConfig,
		}
		err := performGDriveDownload(config, token, fileID, client, progressCh)
		if err != nil {
			return fmt.Errorf("error downloading %s: %v", fileName, err)
		}
	}
	return nil
}

func performGDriveDownload(config utils.HTTPDownloadConfig, token string, fileID string, client *utils.DanzoHTTPClient, progressCh chan<- int64) error {
	outputDir := filepath.Dir(config.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}
	isOAuth := !strings.HasPrefix(token, "AIza")
	var downloadURL string
	if isOAuth {
		downloadURL = fmt.Sprintf("%s/%s?alt=media", driveAPIURL, fileID)
		client.SetHeader("Authorization", "Bearer "+token)
	} else {
		downloadURL = fmt.Sprintf("%s/%s?alt=media&key=%s", driveAPIURL, fileID, token)
	}
	err := danzohttp.PerformSimpleDownload(downloadURL, config.OutputPath, client, progressCh)
	if err != nil {
		return fmt.Errorf("error downloading Google Drive file: %v", err)
	}
	return nil
}
