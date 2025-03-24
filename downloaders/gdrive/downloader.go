package gdrive

import (
	"encoding/json"
	"errors"
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
	driveFolderRegex    = regexp.MustCompile(`https://drive\.google\.com/drive/folders/([^/?\s]+)`)
	driveShortLinkRegex = regexp.MustCompile(`https://drive\.google\.com/open\?id=([^&\s]+)`)
)

const driveAPIURL = "https://www.googleapis.com/drive/v3/files"

func extractFileID(rawURL string) (string, error) {
	if matches := driveFileRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}
	if matches := driveFolderRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
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

func GetAPIKey() (string, error) {
	apiKey := os.Getenv("GDRIVE_API_KEY")
	if apiKey == "" {
		return "", errors.New("GDRIVE_API_KEY environment variable not set")
	}
	return apiKey, nil
}

func GetFileMetadata(rawURL string, client *http.Client, apiKey string) (map[string]any, string, error) {
	log := utils.GetLogger("gdrive-metadata")
	fileID, err := extractFileID(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("error extracting file ID: %v", err)
	}
	metadataURL := fmt.Sprintf("%s/%s?fields=name,size,mimeType&key=%s", driveAPIURL, fileID, apiKey)
	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating metadata request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
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
	log.Debug().Str("fileID", fileID).Str("name", metadata["name"].(string)).Msg("Retrieved file metadata")
	return metadata, fileID, nil
}

func PerformGDriveDownload(config utils.DownloadConfig, apiKey, fileID string, client *http.Client, progressCh chan<- int64) error {
	outputDir := filepath.Dir(config.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}
	log := utils.GetLogger("gdrive-download")
	downloadURL := fmt.Sprintf(driveAPIURL+"/%s?alt=media", fileID)
	if !strings.Contains(downloadURL, "key=") {
		if strings.Contains(downloadURL, "?") {
			downloadURL += "&key=" + apiKey
		} else {
			downloadURL += "?key=" + apiKey
		}
	}
	log.Debug().Str("fileID", fileID).Str("outputPath", config.OutputPath).Msg("Starting Google Drive download")
	err := danzohttp.PerformSimpleDownload(downloadURL, config.OutputPath, client, config.UserAgent, progressCh)
	if err != nil {
		return fmt.Errorf("error downloading Google Drive file: %v", err)
	}
	log.Info().Str("fileID", fileID).Str("outputPath", config.OutputPath).Msg("Google Drive download completed")
	return nil
}
