package youtube

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tanq16/danzo/internal/utils"
)

type YouTubeDownloader struct{}

var ytdlpFormats = map[string]string{
	"best":     "bestvideo+bestaudio/best",
	"best60":   "bestvideo[fps<=60]+bestaudio/best",
	"bestmp4":  "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"decent":   "bestvideo[height<=1080]+bestaudio/best",
	"decent60": "bestvideo[height<=1080][fps<=60]+bestaudio/best",
	"cheap":    "bestvideo[height<=720]+bestaudio/best",
	"1080p":    "bestvideo[height=1080][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"1080p60":  "bestvideo[height=1080][fps<=60][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"720p":     "bestvideo[height=720][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"480p":     "bestvideo[height=480][ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]",
	"audio":    "bestaudio[ext=m4a]/bestaudio",
}

func (d *YouTubeDownloader) ValidateJob(job *utils.DanzoJob) error {
	if !strings.Contains(job.URL, "youtube.com/watch") &&
		!strings.Contains(job.URL, "youtu.be/") &&
		!strings.Contains(job.URL, "music.youtube.com") {
		return fmt.Errorf("invalid YouTube URL")
	}
	if format, ok := job.Metadata["format"].(string); ok {
		if _, exists := ytdlpFormats[format]; !exists {
			return fmt.Errorf("unsupported format: %s", format)
		}
	}
	return nil
}

func (d *YouTubeDownloader) BuildJob(job *utils.DanzoJob) error {
	format, ok := job.Metadata["format"].(string)
	if !ok || format == "" {
		format = "best"
		job.Metadata["format"] = format
	}
	job.Metadata["ytdlpFormat"] = ytdlpFormats[format]
	ytdlpPath, err := EnsureYtdlp()
	if err != nil {
		return fmt.Errorf("error ensuring yt-dlp: %v", err)
	}
	job.Metadata["ytdlpPath"] = ytdlpPath

	ffmpegPath, err := EnsureFFmpeg()
	if err != nil {
		return fmt.Errorf("error ensuring ffmpeg: %v", err)
	}
	job.Metadata["ffmpegPath"] = ffmpegPath
	ffprobePath, err := ensureFFprobe()
	if err != nil {
		return fmt.Errorf("error ensuring ffprobe: %v", err)
	}
	job.Metadata["ffprobePath"] = ffprobePath

	if job.OutputPath == "" {
		job.OutputPath = "%(title)s.%(ext)s"
	}
	return nil
}

func EnsureYtdlp() (string, error) {
	path, err := exec.LookPath("yt-dlp")
	if err == nil {
		return path, nil
	}
	execDir, err := os.Executable()
	if err == nil {
		ytdlpPath := filepath.Join(filepath.Dir(execDir), "yt-dlp")
		if runtime.GOOS == "windows" {
			ytdlpPath += ".exe"
		}
		if _, err := os.Stat(ytdlpPath); err == nil {
			return ytdlpPath, nil
		}
	}
	return downloadYtdlp()
}

func EnsureFFmpeg() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		return path, nil
	}
	execDir, err := os.Executable()
	if err == nil {
		ffmpegPath := filepath.Join(filepath.Dir(execDir), "ffmpeg")
		if runtime.GOOS == "windows" {
			ffmpegPath += ".exe"
		}
		if _, err := os.Stat(ffmpegPath); err == nil {
			return ffmpegPath, nil
		}
	}
	return "", fmt.Errorf("ffmpeg not found in PATH, please install manually")
}

func ensureFFprobe() (string, error) {
	path, err := exec.LookPath("ffprobe")
	if err == nil {
		return path, nil
	}
	execDir, err := os.Executable()
	if err == nil {
		ffprobePath := filepath.Join(filepath.Dir(execDir), "ffprobe")
		if runtime.GOOS == "windows" {
			ffprobePath += ".exe"
		}
		if _, err := os.Stat(ffprobePath); err == nil {
			return ffprobePath, nil
		}
	}
	return "", fmt.Errorf("ffprobe not found in PATH, please install manually")
}
