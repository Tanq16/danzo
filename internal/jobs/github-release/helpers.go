package ghrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/tanq16/danzo/utils"
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

func getOSArchKeywords() ([]string, []string) {
	var osKeys, archKeys []string
	switch runtime.GOOS {
	case "linux":
		osKeys = []string{"linux", "gnu"}
	case "windows":
		osKeys = []string{"windows", "win", ".exe"}
	case "darwin":
		osKeys = []string{"darwin", "mac", "apple", "osx"}
	}
	switch runtime.GOARCH {
	case "amd64":
		archKeys = []string{"amd64", "x86_64", "x86-64", "x64", "64-bit", "64bit"}
	case "arm64":
		archKeys = []string{"arm64", "aarch64"}
	}
	return osKeys, archKeys
}

func getConflictingKeywords() []string {
	var conflicts []string
	if runtime.GOOS != "linux" {
		conflicts = append(conflicts, "linux", "gnu")
	}
	if runtime.GOOS != "windows" {
		conflicts = append(conflicts, "windows", "win", ".exe")
	}
	if runtime.GOOS != "darwin" {
		conflicts = append(conflicts, "darwin", "mac", "apple", "osx")
	}

	if runtime.GOARCH != "amd64" {
		conflicts = append(conflicts, "amd64", "x86_64", "x86-64", "x64")
	}
	if runtime.GOARCH != "arm64" {
		conflicts = append(conflicts, "arm64", "aarch64")
	}
	if runtime.GOARCH != "386" {
		conflicts = append(conflicts, "i386", "386", "x86_32", "x86-32")
	}
	if runtime.GOARCH != "arm" {
		conflicts = append(conflicts, "armv6", "armv7", "arm32")
	}

	return conflicts
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

func getGitHubReleaseAssets(ctx context.Context, owner, repo string, client *utils.DanzoHTTPClient) ([]map[string]any, string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
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
	input, err := utils.PromptInput("Enter the number of the asset to download:", "")
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
	linesUsed := len(assets) + 4
	if !utils.GlobalDebugFlag && !utils.GlobalForAIFlag {
		fmt.Printf("\033[%dA\033[J", linesUsed)
	}

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

	osKeys, archKeys := getOSArchKeywords()
	conflicts := getConflictingKeywords()

	bestScore := -1000
	var bestURL string
	var bestSize int64

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

		score := 0

		for _, key := range osKeys {
			if strings.Contains(assetNameLower, key) {
				score += 5
				break
			}
		}

		for _, key := range archKeys {
			if strings.Contains(assetNameLower, key) {
				score += 5
				break
			}
		}

		for _, key := range conflicts {
			if strings.Contains(assetNameLower, key) {
				score -= 100
			}
		}

		if strings.HasSuffix(assetNameLower, ".tar.gz") || strings.HasSuffix(assetNameLower, ".zip") {
			score += 1
		}

		if score > bestScore {
			bestScore = score
			bestURL, _ = asset["browser_download_url"].(string)
			bestSize = int64(asset["size"].(float64))
		}
	}

	if bestScore > 0 && bestURL != "" {
		return bestURL, bestSize, nil
	}

	return "", 0, nil
}
