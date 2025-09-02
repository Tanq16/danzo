package youtubemusic

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

func (d *YTMusicDownloader) Download(job *utils.DanzoJob) error {
	ytdlpPath := job.Metadata["ytdlpPath"].(string)
	ffmpegPath := job.Metadata["ffmpegPath"].(string)
	tempOutput := strings.TrimSuffix(job.OutputPath, ".m4a")
	args := []string{
		"--progress",
		"--newline",
		"--no-warnings",
		"-x", // Extract audio
		"--audio-format", "m4a",
		"--audio-quality", "0", // Best quality
		"--ffmpeg-location", ffmpegPath,
		"-o", tempOutput + ".%(ext)s",
		"--no-playlist",
		job.URL,
	}
	cmd := exec.Command(ytdlpPath, args...)
	log.Debug().Str("op", "youtube-music/download").Msgf("Executing yt-dlp command: %s", cmd.String())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error().Str("op", "youtube-music/download").Err(err).Msg("Error creating stdout pipe")
		return fmt.Errorf("error creating stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Error().Str("op", "youtube-music/download").Err(err).Msg("Error creating stderr pipe")
		return fmt.Errorf("error creating stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		log.Error().Str("op", "youtube-music/download").Err(err).Msg("Error starting yt-dlp")
		return fmt.Errorf("error starting yt-dlp: %v", err)
	}

	go processStream(stdout, job.StreamFunc)
	go processStream(stderr, job.StreamFunc)
	if err := cmd.Wait(); err != nil {
		log.Error().Str("op", "youtube-music/download").Err(err).Msg("yt-dlp command failed")
		return fmt.Errorf("yt-dlp failed: %v", err)
	}
	log.Debug().Str("op", "youtube-music/download").Msgf("yt-dlp audio extraction completed for %s", job.URL)

	// Apply metadata if music client is specified
	if musicClient, ok := job.Metadata["musicClient"].(string); ok {
		musicID := job.Metadata["musicID"].(string)
		if job.StreamFunc != nil {
			job.StreamFunc(fmt.Sprintf("Fetching metadata from %s...", musicClient))
		}
		// Ensure output path ends with .m4a
		finalPath := job.OutputPath
		if !strings.HasSuffix(finalPath, ".m4a") {
			finalPath = tempOutput + ".m4a"
		}
		log.Debug().Str("op", "youtube-music/download").Msgf("Applying music metadata from %s", musicClient)
		err := addMusicMetadata(tempOutput+".m4a", finalPath, musicClient, musicID, job.StreamFunc)
		if err != nil {
			log.Warn().Str("op", "youtube-music/download").Err(err).Msg("Failed to add metadata")
			if job.StreamFunc != nil {
				job.StreamFunc(fmt.Sprintf("Warning: Failed to add metadata: %v", err))
			}
		}
	}
	log.Info().Str("op", "youtube-music/download").Msgf("YouTube music download completed for %s", job.URL)
	return nil
}

func processStream(reader io.Reader, streamFunc func(string)) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && streamFunc != nil {
			streamFunc(line)
		}
	}
}
