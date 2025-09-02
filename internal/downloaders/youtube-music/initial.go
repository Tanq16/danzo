package youtubemusic

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/downloaders/youtube"
	"github.com/tanq16/danzo/internal/utils"
)

type YTMusicDownloader struct{}

func (d *YTMusicDownloader) ValidateJob(job *utils.DanzoJob) error {
	if !strings.Contains(job.URL, "youtube.com/watch") &&
		!strings.Contains(job.URL, "youtu.be/") &&
		!strings.Contains(job.URL, "music.youtube.com") {
		return fmt.Errorf("invalid YouTube URL")
	}
	if client, ok := job.Metadata["musicClient"].(string); ok {
		if client != "deezer" && client != "apple" {
			return fmt.Errorf("unsupported music client: %s", client)
		}
		if _, ok := job.Metadata["musicID"].(string); !ok {
			return fmt.Errorf("music ID required for %s", client)
		}
	}
	log.Info().Str("op", "youtube-music/initial").Msgf("job validated for %s", job.URL)
	return nil
}

func (d *YTMusicDownloader) BuildJob(job *utils.DanzoJob) error {
	ytdlpPath, err := youtube.EnsureYtdlp()
	if err != nil {
		return fmt.Errorf("error ensuring yt-dlp: %v", err)
	}
	job.Metadata["ytdlpPath"] = ytdlpPath
	log.Debug().Str("op", "youtube-music/initial").Msgf("Using yt-dlp at: %s", ytdlpPath)

	ffmpegPath, err := ensureFFmpeg()
	if err != nil {
		return fmt.Errorf("error ensuring ffmpeg: %v", err)
	}
	job.Metadata["ffmpegPath"] = ffmpegPath
	log.Debug().Str("op", "youtube-music/initial").Msgf("Using ffmpeg at: %s", ffmpegPath)

	if job.OutputPath == "" {
		job.OutputPath = "%(title)s.m4a"
	} else if !strings.HasSuffix(job.OutputPath, ".m4a") {
		// Force .m4a extension
		job.OutputPath = strings.TrimSuffix(job.OutputPath, filepath.Ext(job.OutputPath)) + ".m4a"
	}

	if info, err := os.Stat(job.OutputPath); err == nil && !info.IsDir() {
		job.OutputPath = utils.RenewOutputPath(job.OutputPath)
		log.Debug().Str("op", "youtube-music/initial").Msgf("Output path renewed to %s", job.OutputPath)
	}
	log.Info().Str("op", "youtube-music/initial").Msgf("job built for %s", job.URL)
	return nil
}

func ensureFFmpeg() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		return path, nil
	}
	log.Error().Str("op", "youtube-music/initial").Msg("ffmpeg not found in PATH. Please install it.")
	return "", fmt.Errorf("ffmpeg not found in PATH, please install manually")
}
