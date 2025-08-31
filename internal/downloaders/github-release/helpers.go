package ghrelease

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/tanq16/danzo/internal/utils"
)

var assetSelectMap = map[string][]string{
	"linuxamd64":   {"linux-amd64", "linux_amd64", "linux-x86_64", "linux-x86-64", "linux_x86_64", "linux_x86-64", "amd64-linux", "x86_64-linux", "x86-64-linux", "amd64_linux", "x86_64_linux", "x86-64_linux"},
	"linuxarm64":   {"linux-arm64", "linux_arm64", "linux-aarch64", "linux_aarch64", "arm64-linux", "aarch64-linux", "arm64_linux", "aarch64_linux"},
	"windowsamd64": {"windows-amd64", "windows_amd64", "windows-x86_64", "windows-x86-64", "windows_x86_64", "windows_x86-64", "amd64-windows", "x86_64-windows", "x86-64-windows", "amd64_windows", "x86_64_windows", "x86-64_windows"},
	"windowsarm64": {"windows-arm64", "windows_arm64", "windows-aarch64", "windows_aarch64", "arm64-windows", "aarch64-windows", "arm64_windows", "aarch64_windows"},
	"darwinamd64":  {"darwin-amd64", "darwin_amd64", "darwin-x86_64", "darwin-x86-64", "darwin_x86_64", "darwin_x86-64", "amd64-darwin", "x86_64-darwin", "x86-64-darwin", "amd64_darwin", "x86_64_darwin", "x86-64_darwin"},
	"darwinarm64":  {"darwin-arm64", "darwin_arm64", "darwin-aarch64", "darwin_aarch64", "arm64-darwin", "aarch64-darwin", "arm64_darwin", "aarch64_darwin"},
}

var assetSelectMapFallback = map[string][]string{
	"linuxamd64":   {"linux", "gnu", "x86-64", "x86_64", "amd64", "amd"},
	"linuxarm64":   {"linux", "gnu", "arm", "arm64"},
	"windowsamd64": {"exe", "x86-64", "x86_64", "amd64", "amd"},
	"windowsarm64": {"exe", "arm", "arm64"},
	"darwinamd64":  {"darwin", "apple", "x86-64", "x86_64", "amd64", "amd"},
	"darwinarm64":  {"darwin", "apple", "arm", "arm64"},
}

var repoPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/?.*$`),
	regexp.MustCompile(`^github\.com/([^/]+)/([^/]+)/?.*$`),
	regexp.MustCompile(`^([^/]+)/([^/]+)$`),
}

var ignoredAssets = []string{
	"license", "readme", "changelog", "checksums", "sha256checksum", ".sha256",
}

func parseGitHubURL(url string) (string, string, error) {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	for _, pattern := range repoPatterns {
		matches := pattern.FindStringSubmatch(url)
		if len(matches) >= 3 {
			return matches[1], matches[2], nil
		}
	}
	return "", "", fmt.Errorf("invalid GitHub repository format: %s", url)
}

func getGitHubReleaseAssets(owner, repo string, client *utils.DanzoHTTPClient) ([]map[string]any, string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating API request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error making API request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	var release map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, "", fmt.Errorf("error decoding API response: %v", err)
	}
	tagName, _ := release["tag_name"].(string)
	assets, ok := release["assets"].([]any)
	if !ok {
		return nil, "", fmt.Errorf("no assets found in the release")
	}
	var assetList []map[string]any
	for _, asset := range assets {
		assetMap, ok := asset.(map[string]any)
		if ok {
			assetList = append(assetList, assetMap)
		}
	}
	if len(assetList) == 0 {
		return nil, "", fmt.Errorf("no assets found in the release")
	}
	return assetList, tagName, nil
}

func promptGitHubAssetSelection(assets []map[string]any, tagName string) (string, int64, error) {
	fmt.Printf("Release: %s\nAvailable assets:\n", tagName)
	for i, asset := range assets {
		name, _ := asset["name"].(string)
		size, _ := asset["size"].(float64)
		fmt.Printf("%d. %s (%.2f MB)\n", i+1, name, float64(size)/1024/1024)
	}
	fmt.Print("\nEnter the number of the asset to download: ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", 0, fmt.Errorf("error reading input: %v", err)
	}

	input = strings.TrimSpace(input)
	selection, err := strconv.Atoi(input)
	if err != nil {
		return "", 0, fmt.Errorf("invalid selection: %v", err)
	}
	if selection < 1 || selection > len(assets) {
		return "", 0, fmt.Errorf("selection out of range")
	}
	linesUsed := len(assets) + 4 // Assets list + Release line + Prompt line + Input line + newline
	fmt.Printf("\033[%dA\033[J", linesUsed)

	selectedAsset := assets[selection-1]
	downloadURL, _ := selectedAsset["browser_download_url"].(string)
	size, _ := selectedAsset["size"].(float64)
	return downloadURL, int64(size), nil
}

func selectGitHubLatestAsset(assets []map[string]any) (string, int64, error) {
	platformKey := fmt.Sprintf("%s%s", runtime.GOOS, runtime.GOARCH)
	for _, asset := range assets {
		assetName, _ := asset["name"].(string)
		assetNameLower := strings.ToLower(assetName)
		isIgnored := false
		for _, ignored := range ignoredAssets {
			if strings.Contains(assetNameLower, ignored) {
				isIgnored = true
				break
			}
		}
		if isIgnored {
			continue
		}
		for _, key := range assetSelectMap[platformKey] {
			if strings.Contains(assetNameLower, key) {
				downloadURL, _ := asset["browser_download_url"].(string)
				size, _ := asset["size"].(float64)
				return downloadURL, int64(size), nil
			}
		}
	}
	return "", 0, nil
}
