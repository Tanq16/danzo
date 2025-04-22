package gdrive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	danzohttp "github.com/tanq16/danzo/downloaders/http"
	"github.com/tanq16/danzo/utils"
)

var (
	driveFileRegex      = regexp.MustCompile(`https://drive\.google\.com/file/d/([^/]+)`)
	driveShortLinkRegex = regexp.MustCompile(`https://drive\.google\.com/open\?id=([^&\s]+)`)
)

const driveAPIURL = "https://www.googleapis.com/drive/v3/files"

func extractFileID(rawURL string) (string, error) {
	if matches := driveFileRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}
	if matches := driveShortLinkRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}
	// Heuristically try to extract from URL parameters
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	idParam := parsedURL.Query().Get("id")
	if idParam != "" {
		return idParam, nil
	}
	return "", fmt.Errorf("unable to extract file ID from URL: %s", rawURL)
}

func GetFileMetadata(rawURL string, client *http.Client, token string) (map[string]any, string, error) {
	fileID, err := extractFileID(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("error extracting file ID: %v", err)
	}
	isOAuth := !strings.HasPrefix(token, "AIza")
	var metadataURL string
	if isOAuth {
		metadataURL = fmt.Sprintf("%s/%s?fields=name,size,mimeType", driveAPIURL, fileID)
	} else {
		metadataURL = fmt.Sprintf("%s/%s?fields=name,size,mimeType&key=%s", driveAPIURL, fileID, token)
	}

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating metadata request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	if isOAuth {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error fetching file metadata: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to get file metadata, status: %d", resp.StatusCode)
	}
	var metadata map[string]any
	err = json.NewDecoder(resp.Body).Decode(&metadata)
	if err != nil {
		return nil, "", fmt.Errorf("error parsing metadata response: %v", err)
	}
	return metadata, fileID, nil
}

func PerformGDriveDownload(config utils.DownloadConfig, token string, fileID string, client *http.Client, progressCh chan<- int64) error {
	outputDir := filepath.Dir(config.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}
	isOAuth := !strings.HasPrefix(token, "AIza")
	var downloadURL string
	if isOAuth {
		downloadURL = fmt.Sprintf("%s/%s?alt=media|%s", driveAPIURL, fileID, token)
	} else {
		downloadURL = fmt.Sprintf("%s/%s?alt=media&key=%s", driveAPIURL, fileID, token)
	}
	err := danzohttp.PerformSimpleDownload(downloadURL, config.OutputPath, client, progressCh)
	if err != nil {
		return fmt.Errorf("error downloading Google Drive file: %v", err)
	}
	return nil
}
