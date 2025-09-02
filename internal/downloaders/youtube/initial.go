package youtube

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"
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
	log.Info().Str("op", "youtube/initial").Msgf("job validated for %s", job.URL)
	return nil
}

func (d *YouTubeDownloader) BuildJob(job *utils.DanzoJob) error {
	format, ok := job.Metadata["format"].(string)
	if !ok || format == "" {
		format = "decent"
		job.Metadata["format"] = format
	}
	job.Metadata["ytdlpFormat"] = ytdlpFormats[format]
	log.Debug().Str("op", "youtube/initial").Msgf("Using format key '%s' for yt-dlp format '%s'", format, ytdlpFormats[format])

	ytdlpPath, err := EnsureYtdlp()
	if err != nil {
		return fmt.Errorf("error ensuring yt-dlp: %v", err)
	}
	job.Metadata["ytdlpPath"] = ytdlpPath
	log.Debug().Str("op", "youtube/initial").Msgf("Using yt-dlp at: %s", ytdlpPath)

	ffmpegPath, err := EnsureFFmpeg()
	if err != nil {
		return fmt.Errorf("error ensuring ffmpeg: %v", err)
	}
	job.Metadata["ffmpegPath"] = ffmpegPath
	log.Debug().Str("op", "youtube/initial").Msgf("Using ffmpeg at: %s", ffmpegPath)

	ffprobePath, err := ensureFFprobe()
	if err != nil {
		return fmt.Errorf("error ensuring ffprobe: %v", err)
	}
	job.Metadata["ffprobePath"] = ffprobePath
	log.Debug().Str("op", "youtube/initial").Msgf("Using ffprobe at: %s", ffprobePath)

	if job.OutputPath == "" {
		job.OutputPath = "%(title)s.%(ext)s"
	}
	log.Info().Str("op", "youtube/initial").Msgf("job built for %s", job.URL)
	return nil
}

func EnsureYtdlp() (string, error) {
	path, err := exec.LookPath("yt-dlp")
	if err == nil {
		log.Debug().Str("op", "youtube/initial").Msgf("yt-dlp found in PATH: %s", path)
		return path, nil
	}
	execDir, err := os.Executable()
	if err == nil {
		ytdlpPath := filepath.Join(filepath.Dir(execDir), "yt-dlp")
		if runtime.GOOS == "windows" {
			ytdlpPath += ".exe"
		}
		if _, err := os.Stat(ytdlpPath); err == nil {
			log.Debug().Str("op", "youtube/initial").Msgf("yt-dlp found in executable directory: %s", ytdlpPath)
			return ytdlpPath, nil
		}
	}
	log.Warn().Str("op", "youtube/initial").Msg("yt-dlp not found, attempting download")
	return downloadYtdlp()
}

func EnsureFFmpeg() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		log.Debug().Str("op", "youtube/initial").Msgf("ffmpeg found in PATH: %s", path)
		return path, nil
	}
	execDir, err := os.Executable()
	if err == nil {
		ffmpegPath := filepath.Join(filepath.Dir(execDir), "ffmpeg")
		if runtime.GOOS == "windows" {
			ffmpegPath += ".exe"
		}
		if _, err := os.Stat(ffmpegPath); err == nil {
			log.Debug().Str("op", "youtube/initial").Msgf("ffmpeg found in executable directory: %s", ffmpegPath)
			return ffmpegPath, nil
		}
	}
	log.Error().Str("op", "youtube/initial").Msg("ffmpeg not found in PATH or executable directory. Please install it.")
	return "", fmt.Errorf("ffmpeg not found in PATH, please install manually")
}

func ensureFFprobe() (string, error) {
	path, err := exec.LookPath("ffprobe")
	if err == nil {
		log.Debug().Str("op", "youtube/initial").Msgf("ffprobe found in PATH: %s", path)
		return path, nil
	}
	execDir, err := os.Executable()
	if err == nil {
		ffprobePath := filepath.Join(filepath.Dir(execDir), "ffprobe")
		if runtime.GOOS == "windows" {
			ffprobePath += ".exe"
		}
		if _, err := os.Stat(ffprobePath); err == nil {
			log.Debug().Str("op", "youtube/initial").Msgf("ffprobe found in executable directory: %s", ffprobePath)
			return ffprobePath, nil
		}
	}
	log.Error().Str("op", "youtube/initial").Msg("ffprobe not found in PATH or executable directory. Please install it.")
	return "", fmt.Errorf("ffprobe not found in PATH, please install manually")
}
