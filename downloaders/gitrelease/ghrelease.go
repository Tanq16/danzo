package gitrelease

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

var ignoredAssets = []string{
	"license", "readme", "changelog", "checksums", "sha256checksum", ".sha256",
}

func ParseGitURL(url string) (string, string, bool, error) {
	processParts := strings.Split(url, "||")
	parts := strings.Split(processParts[0][9:], "/") // Remove "github://"
	if len(parts) < 2 {
		return "", "", false, fmt.Errorf("invalid GitHub URL format, expected github://owner/repo")
	}
	return parts[0], parts[1], len(processParts) == 2 && processParts[1] == "version", nil
}

func GetReleaseAssets(owner, repo string, client *http.Client) ([]map[string]any, string, error) {
	log := utils.GetLogger("github-release")
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
	log.Debug().Str("owner", owner).Str("repo", repo).Str("tag", tagName).Int("assetCount", len(assetList)).Msg("GitHub release assets fetched")
	return assetList, tagName, nil
}

func AskReleaseAssets(owner, repo string, client *http.Client) ([]map[string]any, string, error) {
	log := utils.GetLogger("github-release")
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
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

	var releases []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, "", fmt.Errorf("error decoding API response: %v", err)
	}
	if len(releases) == 0 {
		return nil, "", fmt.Errorf("no releases found for the repository")
	}
	fmt.Printf("\nAvailable releases for %s/%s:\n", owner, repo)
	for i, release := range releases {
		tagName, _ := release["tag_name"].(string)
		fmt.Printf("%d. %s\n", i+1, tagName)
	}
	fmt.Print("\nEnter the number of the release to download: ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, "", fmt.Errorf("error reading input: %v", err)
	}

	input = strings.TrimSpace(input)
	selection, err := strconv.Atoi(input)
	if err != nil {
		return nil, "", fmt.Errorf("invalid selection: %v", err)
	}
	if selection < 1 || selection > len(releases) {
		return nil, "", fmt.Errorf("selection out of range")
	}
	linesUsed := len(releases) + 3 // Releases list + Prompt line + Input line
	fmt.Printf("\033[%dA\033[J", linesUsed)

	selectedRelease := releases[selection-1]
	tagName, _ := selectedRelease["tag_name"].(string)
	assets, ok := selectedRelease["assets"].([]any)
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
	log.Debug().Str("owner", owner).Str("repo", repo).Str("tag", tagName).Int("assetCount", len(assetList)).Msg("GitHub release assets fetched")
	return assetList, tagName, nil
}

func PromptAssetSelection(assets []map[string]any, tagName string) (string, int64, error) {
	fmt.Printf("\nRelease: %s\nAvailable assets:\n", tagName)
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
	linesUsed := len(assets) + 3 // Assets list + Release line + Prompt line + Input line
	fmt.Printf("\033[%dA\033[J", linesUsed)

	selectedAsset := assets[selection-1]
	downloadURL, _ := selectedAsset["browser_download_url"].(string)
	size, _ := selectedAsset["size"].(float64)
	return downloadURL, int64(size), nil
}

func SelectLatestAsset(assets []map[string]any) (string, int64, error) {
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

func ProcessGitHubRelease(owner, repo string, userSelect bool, client *http.Client) (string, string, int64, error) {
	var assets []map[string]any
	var tagName string
	var err error
	if userSelect {
		assets, tagName, err = AskReleaseAssets(owner, repo, client)
		if err != nil {
			return "", "", 0, err
		}
	} else {
		assets, tagName, err = GetReleaseAssets(owner, repo, client)
		if err != nil {
			return "", "", 0, err
		}
	}
	var downloadURL string
	var size int64
	if userSelect {
		downloadURL, size, err = PromptAssetSelection(assets, tagName)
		if err != nil {
			return "", "", 0, err
		}
	} else {
		downloadURL, size, err = SelectLatestAsset(assets)
		if err != nil {
			return "", "", 0, err
		}
		if downloadURL == "" {
			downloadURL, size, err = PromptAssetSelection(assets, tagName)
			if err != nil {
				return "", "", 0, err
			}
		}
	}
	urlParts := strings.Split(downloadURL, "/")
	filename := urlParts[len(urlParts)-1]
	return downloadURL, filename, size, nil
}

func ProcessRelease(url string, client *http.Client) (string, string, int64, error) {
	owner, repo, userSelect, err := ParseGitURL(url)
	if err != nil {
		return "", "", 0, err
	}
	return ProcessGitHubRelease(owner, repo, userSelect, client)
}
