package youtube

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tanq16/danzo/internal/utils"
)

func downloadYtdlp() (string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	var filename string
	switch {
	case goos == "windows" && goarch == "amd64":
		filename = "yt-dlp.exe"
	case goos == "windows" && goarch == "arm64":
		filename = "yt-dlp_arm64.exe"
	case goos == "linux" && goarch == "amd64":
		filename = "yt-dlp_linux"
	case goos == "linux" && goarch == "arm64":
		filename = "yt-dlp_linux_aarch64"
	case goos == "darwin":
		filename = "yt-dlp_macos"
	default:
		return "", fmt.Errorf("unsupported OS/arch: %s/%s", goos, goarch)
	}

	tempDir := ".danzo-temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("error creating temp directory: %v", err)
	}
	downloadURL := fmt.Sprintf("https://github.com/yt-dlp/yt-dlp/releases/latest/download/%s", filename)
	filePath := filepath.Join(tempDir, "yt-dlp")
	if goos == "windows" {
		filePath += ".exe"
	}
	if err := downloadFile(downloadURL, filePath); err != nil {
		return "", err
	}
	if goos != "windows" {
		if err := os.Chmod(filePath, 0755); err != nil {
			return "", fmt.Errorf("error setting permissions: %v", err)
		}
	}
	return filePath, nil
}

func downloadFile(url, filepath string) error {
	client := utils.NewDanzoHTTPClient(utils.HTTPClientConfig{})
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
