package gdrive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tanq16/danzo/internal/highway"
	danzohttp "github.com/tanq16/danzo/internal/jobs/http"
	"github.com/tanq16/danzo/utils"
)

type GDriveJob struct {
	id              string
	URL             string
	OutputPath      string
	APIKey          string
	CredentialsFile string
	HTTPConfig      utils.HTTPClientConfig
	PauseDisplay    func()
	ResumeDisplay   func()
}

type gdriveJobState struct {
	URL             string            `json:"url"`
	OutputPath      string            `json:"outputPath"`
	APIKey          string            `json:"apiKey,omitempty"`
	CredentialsFile string            `json:"credentialsFile,omitempty"`
	ProxyURL        string            `json:"proxyURL,omitempty"`
	UserAgent       string            `json:"userAgent,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
}

func New(url, outputPath, apiKey, credentialsFile string, httpConfig utils.HTTPClientConfig) *GDriveJob {
	id := outputPath
	if id == "" {
		id = url
	}
	return &GDriveJob{
		id:              id,
		URL:             url,
		OutputPath:      outputPath,
		APIKey:          apiKey,
		CredentialsFile: credentialsFile,
		HTTPConfig:      httpConfig,
	}
}

func (j *GDriveJob) ID() string {
	return j.id
}

func (j *GDriveJob) Type() string { return "google-drive" }

func (j *GDriveJob) Run(ctx context.Context, progress chan<- highway.Progress) error {
	fileID, err := extractFileID(j.URL)
	if err != nil {
		return err
	}

	if j.APIKey == "" && j.CredentialsFile == "" {
		return fmt.Errorf("either --api-key or --credentials must be provided")
	}
	if j.APIKey != "" && j.CredentialsFile != "" {
		return fmt.Errorf("only one of --api-key or --credentials can be provided")
	}
	if j.CredentialsFile != "" {
		if _, err := os.Stat(j.CredentialsFile); err != nil {
			return fmt.Errorf("credentials file not found: %v", err)
		}
	}

	var token string
	if j.APIKey != "" {
		token = j.APIKey
	} else {
		if j.PauseDisplay != nil {
			j.PauseDisplay()
		}
		token, err = getAccessTokenFromCredentials(ctx, j.CredentialsFile)
		if j.ResumeDisplay != nil {
			j.ResumeDisplay()
		}
		if err != nil {
			return fmt.Errorf("error getting OAuth token: %v", err)
		}
	}

	client := utils.NewDanzoHTTPClient(j.HTTPConfig)
	metadata, _, err := getFileMetadata(ctx, j.URL, client, token)
	if err != nil {
		return fmt.Errorf("error getting metadata: %v", err)
	}

	mimeType, _ := metadata["mimeType"].(string)
	isFolder := mimeType == "application/vnd.google-apps.folder"

	var totalSize int64
	var folderFiles []map[string]any

	if isFolder {
		files, err := listFolderContents(ctx, fileID, token, client)
		if err != nil {
			return fmt.Errorf("error listing folder contents: %v", err)
		}
		folderFiles = files
		for _, file := range files {
			if size, ok := file["size"].(string); ok {
				if sizeInt, err := strconv.ParseInt(size, 10, 64); err == nil {
					totalSize += sizeInt
				}
			}
		}
		if j.OutputPath == "" {
			j.OutputPath = metadata["name"].(string)
		}
	} else {
		if j.OutputPath == "" {
			j.OutputPath = metadata["name"].(string)
		}
		if sizeStr, ok := metadata["size"].(string); ok {
			totalSize, _ = strconv.ParseInt(sizeStr, 10, 64)
		}
	}

	if info, err := os.Stat(j.OutputPath); err == nil {
		if isFolder && info.IsDir() {
			j.OutputPath = utils.RenewOutputPath(j.OutputPath)
		} else if !isFolder && !info.IsDir() {
			j.OutputPath = utils.RenewOutputPath(j.OutputPath)
		}
	}

	progress <- highway.Progress{
		JobID: j.ID(), Type: highway.ProgressTypeProgress,
		Message: "Downloading", Current: 0, Total: totalSize,
	}

	if isFolder {
		err = j.downloadFolder(ctx, progress, token, client, totalSize, folderFiles)
	} else {
		err = j.downloadFile(ctx, progress, token, fileID, client, totalSize)
	}

	if err != nil {
		return err
	}

	progress <- highway.Progress{JobID: j.ID(), Done: true}
	return nil
}

func (j *GDriveJob) downloadFile(ctx context.Context, progress chan<- highway.Progress, token, fileID string, client *utils.DanzoHTTPClient, totalSize int64) error {
	if err := os.MkdirAll(filepath.Dir(j.OutputPath), 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}
	progressCh := make(chan int64)
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		var downloaded int64
		for bytes := range progressCh {
			downloaded += bytes
			progress <- highway.Progress{
				JobID: j.ID(), Type: highway.ProgressTypeProgress,
				Message: "Downloading", Current: downloaded, Total: totalSize,
				Extra: utils.FormatBytes(uint64(downloaded)) + "/" + utils.FormatBytes(uint64(totalSize)),
			}
		}
	}()
	err := performGDriveDownload(ctx, j.OutputPath, token, fileID, client, progressCh)
	<-progressDone
	return err
}

func (j *GDriveJob) downloadFolder(ctx context.Context, progress chan<- highway.Progress, token string, client *utils.DanzoHTTPClient, totalSize int64, files []map[string]any) error {
	if err := os.MkdirAll(j.OutputPath, 0755); err != nil {
		return fmt.Errorf("error creating folder: %v", err)
	}
	var totalDownloaded int64
	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		fID := file["id"].(string)
		fileName := file["name"].(string)
		fMimeType := file["mimeType"].(string)
		if strings.HasPrefix(fMimeType, "application/vnd.google-apps.") {
			continue
		}
		outputPath := filepath.Join(j.OutputPath, fileName)
		progressCh := make(chan int64)
		progressDone := make(chan struct{})
		go func(ch <-chan int64) {
			defer close(progressDone)
			for bytes := range ch {
				totalDownloaded += bytes
				progress <- highway.Progress{
					JobID: j.ID(), Type: highway.ProgressTypeProgress,
					Message: "Downloading", Current: totalDownloaded, Total: totalSize,
					Extra: utils.FormatBytes(uint64(totalDownloaded)) + "/" + utils.FormatBytes(uint64(totalSize)),
				}
			}
		}(progressCh)
		err := performGDriveDownload(ctx, outputPath, token, fID, client, progressCh)
		<-progressDone
		if err != nil {
			return fmt.Errorf("error downloading %s: %v", fileName, err)
		}
	}
	return nil
}

func (j *GDriveJob) Marshal() ([]byte, error) {
	return json.Marshal(gdriveJobState{
		URL:             j.URL,
		OutputPath:      j.OutputPath,
		APIKey:          j.APIKey,
		CredentialsFile: j.CredentialsFile,
		ProxyURL:        j.HTTPConfig.ProxyURL,
		UserAgent:       j.HTTPConfig.UserAgent,
		Headers:         j.HTTPConfig.Headers,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state gdriveJobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URL, state.OutputPath, state.APIKey, state.CredentialsFile, utils.HTTPClientConfig{
		ProxyURL:  state.ProxyURL,
		UserAgent: state.UserAgent,
		Headers:   state.Headers,
	}), nil
}

func performGDriveDownload(ctx context.Context, outputPath string, token, fileID string, client *utils.DanzoHTTPClient, progressCh chan<- int64) error {
	isOAuth := !strings.HasPrefix(token, "AIza")
	var downloadURL string
	if isOAuth {
		downloadURL = fmt.Sprintf("%s/%s?alt=media", driveAPIURL, fileID)
		client.SetHeader("Authorization", "Bearer "+token)
	} else {
		downloadURL = fmt.Sprintf("%s/%s?alt=media&key=%s", driveAPIURL, fileID, token)
	}
	return danzohttp.PerformSimpleDownload(ctx, downloadURL, outputPath, client, progressCh)
}
