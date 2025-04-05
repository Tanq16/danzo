package youtube

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"slices"

	"github.com/tanq16/danzo/utils"
)

var ytdlpFormats = map[string]string{
	"best":     "bestvideo+bestaudio/best",
	"bestmp4":  "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"decent":   "bestvideo[height<=1080]+bestaudio/best",
	"decent60": "bestvideo[height<=1080][fps<=60]+bestaudio/best",
	"cheap":    "bestvideo[height<=720]+bestaudio/best",
	"1080p":    "bestvideo[height=1080][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"1080p60":  "bestvideo[height=1080][fps<=60][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"720p":     "bestvideo[height=720][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"480p":     "bestvideo[height=480][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
}

func isYtDlpAvailable() string {
	// Checks if "yt-dlp" is in PATH or current directory
	path, err := exec.LookPath("yt-dlp")
	if err == nil {
		return path
	}
	executableDir, err := os.Executable()
	if err == nil {
		ytdlpPath := filepath.Join(filepath.Dir(executableDir), "yt-dlp")
		if _, err := os.Stat(ytdlpPath); err == nil {
			return ytdlpPath
		}
		ytdlpPathExe := filepath.Join(filepath.Dir(executableDir), "yt-dlp.exe")
		if _, err := os.Stat(ytdlpPathExe); err == nil {
			return ytdlpPathExe
		}
	}
	return ""
}

func ProcessURL(urlRaw string) (string, string, string, string, error) {
	urldata := strings.Split(urlRaw, "||")
	if len(urldata) > 1 {
		if urldata[1] == "audio" {
			return urldata[0], "m4a", "audio", "danzo-yt-dlp-audio.m4a", nil
		} else {
			return urldata[0], ytdlpFormats[urldata[1]], "video", "danzo-yt-dlp-video.mp4", nil
		}
	} else {
		return urldata[0], ytdlpFormats["best"], "video", "danzo-yt-dlp-video.mp4", nil
	}
}

func downloadYtdlp() (string, error) {
	log := utils.GetLogger("ytdlp-installer")
	log.Debug().Msg("yt-dlp not found, attempting to download automatically")
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	var filename string
	switch {
	case goos == "windows" && goarch == "amd64":
		filename = "yt-dlp.exe"
	case goos == "linux" && goarch == "amd64":
		filename = "yt-dlp_linux"
	case goos == "linux" && goarch == "arm64":
		filename = "yt-dlp_linux_aarch64"
	case goos == "darwin": // macOS (both Intel and Apple Silicon)
		filename = "yt-dlp_macos"
	default:
		return "", fmt.Errorf("unsupported OS/architecture combination: %s/%s", goos, goarch)
	}

	tempDir := ".danzo-temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("error creating temp directory: %v", err)
	}
	baseURL := "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"
	downloadURL := baseURL + filename
	filePath := filepath.Join(tempDir, filename)
	client := utils.CreateHTTPClient(30*time.Second, 30*time.Second, "", false,nil)
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("User-Agent", utils.ToolUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error downloading yt-dlp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error downloading yt-dlp: status code %d", resp.StatusCode)
	}
	out, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("error creating file: %v", err)
	}
	defer out.Close()

	// Set file permissions to executable (for UNIX systems)
	if goos != "windows" {
		if err := out.Chmod(0755); err != nil {
			return "", fmt.Errorf("error setting file permissions: %v", err)
		}
	}
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("error writing to file: %v", err)
	}
	log.Debug().Str("path", filePath).Msg("yt-dlp not found, downloaded automatically")
	return filePath, nil
}

func removeExtension(filePath string) string {
	ext := filepath.Ext(filePath)
	if ext == "" {
		return filePath
	}
	return filePath[:len(filePath)-len(ext)]
}

func DownloadYouTubeVideo(url, outputPathPre, format, dType string, progressCh chan<- int64, outputCh chan<- []string) error {
	log := utils.GetLogger("youtube-downloader")
	outputPath := removeExtension(outputPathPre)
	ytdlpPath := isYtDlpAvailable()
	if ytdlpPath == "" {
		var err error
		ytdlpPath, err = downloadYtdlp()
		if err != nil {
			return fmt.Errorf("yt-dlp not found and failed to download: %v", err)
		}
	}
	log.Debug().Str("url", url).Str("output", outputPath).Msg("Starting YouTube download with yt-dlp")
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}

	var cmd *exec.Cmd
	if dType == "audio" {
		cmd = exec.Command(ytdlpPath,
			"-q",
			"--progress",
			"--newline",
			"--progress-delta", "1",
			"-x",
			"--audio-format", format,
			"--audio-quality", "0",
			"-o", fmt.Sprintf("%s.%%(ext)s", outputPath),
			url,
		)
	} else {
		cmd = exec.Command(ytdlpPath,
			"-q",
			"--progress",
			"--newline",
			"--progress-delta", "1",
			"--no-warnings",
			"-f", format,
			"-o", fmt.Sprintf("%s.%%(ext)s", outputPath),
			"--no-playlist",
			url,
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting yt-dlp: %v", err)
	}
	go processOutput(stdout, outputCh, 5)
	go processOutput(stderr, outputCh, 2)
	err = cmd.Wait()

	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return fmt.Errorf("yt-dlp exited with code %d", status.ExitStatus())
			}
		}
		return fmt.Errorf("error executing yt-dlp: %v", err)
	}

	// Get file size after download is complete
	var totalSizeBytes int64
	fileInfo, err := os.Stat(outputPath)
	if err == nil {
		totalSizeBytes = fileInfo.Size()
	} else {
		log.Debug().Err(err).Msg("Unable to get file size, using estimate instead")
		totalSizeBytes = 1
	}
	progressCh <- totalSizeBytes
	log.Debug().Str("url", url).Str("output", outputPath).Int64("size", totalSizeBytes).Msg("YouTube download completed successfully")
	return nil
}

// processOutput reads from a pipe and sends lines to the output channel
func processOutput(pipe io.ReadCloser, outputCh chan<- []string, maxLines int) {
	scanner := bufio.NewScanner(pipe)
	buffer := []string{}

	for scanner.Scan() {
		line := scanner.Text()
		buffer = append(buffer, line)
		if len(buffer) > maxLines {
			buffer = buffer[len(buffer)-maxLines:]
		}
		outputCh <- slices.Clone(buffer)
	}
}
