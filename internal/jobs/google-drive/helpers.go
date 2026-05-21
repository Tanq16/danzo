package gdrive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/tanq16/danzo/utils"
)

var (
	driveFileRegex      = regexp.MustCompile(`https://drive\.google\.com/file/d/([^/]+)`)
	driveShortLinkRegex = regexp.MustCompile(`https://drive\.google\.com/open\?id=([^&\s]+)`)
	driveFolderRegex    = regexp.MustCompile(`https://drive\.google\.com/drive/folders/([^/]+)`)
)

const driveAPIURL = "https://www.googleapis.com/drive/v3/files"

func extractFileID(rawURL string) (string, error) {
	if matches := driveFileRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}
	if matches := driveShortLinkRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}
	if matches := driveFolderRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}
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

func getFileMetadata(ctx context.Context, rawURL string, client *utils.DanzoHTTPClient, token string) (map[string]any, string, error) {
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

	req, err := http.NewRequestWithContext(ctx, "GET", metadataURL, nil)
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

func listFolderContents(ctx context.Context, folderID, token string, client *utils.DanzoHTTPClient) ([]map[string]any, error) {
	var files []map[string]any
	pageToken := ""
	isOAuth := !strings.HasPrefix(token, "AIza")
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var reqURL string
		if isOAuth {
			reqURL = fmt.Sprintf("%s?q='%s'+in+parents&fields=nextPageToken,files(id,name,size,mimeType)&pageSize=1000",
				driveAPIURL, folderID)
			if pageToken != "" {
				reqURL += "&pageToken=" + pageToken
			}
		} else {
			reqURL = fmt.Sprintf("%s?q='%s'+in+parents&fields=nextPageToken,files(id,name,size,mimeType)&pageSize=1000&key=%s",
				driveAPIURL, folderID, token)
			if pageToken != "" {
				reqURL += "&pageToken=" + pageToken
			}
		}
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, err
		}
		if isOAuth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to list folder contents: %d", resp.StatusCode)
		}

		var result map[string]any
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		if items, ok := result["files"].([]any); ok {
			for _, item := range items {
				if fileMap, ok := item.(map[string]any); ok {
					files = append(files, fileMap)
				}
			}
		}

		if nextToken, ok := result["nextPageToken"].(string); ok && nextToken != "" {
			pageToken = nextToken
		} else {
			break
		}
	}
	return files, nil
}
