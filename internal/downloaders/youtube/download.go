package youtube

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

func (d *YouTubeDownloader) Download(job *utils.DanzoJob) error {
	ytdlpPath := job.Metadata["ytdlpPath"].(string)
	ytdlpFormat := job.Metadata["ytdlpFormat"].(string)
	ffmpegPath := job.Metadata["ffmpegPath"].(string)
	args := []string{
		"--progress",
		"--newline",
		"--no-warnings",
		"-f", ytdlpFormat,
		"--ffmpeg-location", ffmpegPath,
		"-o", job.OutputPath,
		"--no-playlist",
		job.URL,
	}
	cmd := exec.Command(ytdlpPath, args...)
	log.Debug().Str("op", "youtube/download").Msgf("Executing yt-dlp command: %s", cmd.String())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error().Str("op", "youtube/download").Err(err).Msg("Error creating stdout pipe")
		return fmt.Errorf("error creating stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Error().Str("op", "youtube/download").Err(err).Msg("Error creating stderr pipe")
		return fmt.Errorf("error creating stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		log.Error().Str("op", "youtube/download").Err(err).Msg("Error starting yt-dlp")
		return fmt.Errorf("error starting yt-dlp: %v", err)
	}

	go processStream(stdout, job.StreamFunc)
	go processStream(stderr, job.StreamFunc)
	if err := cmd.Wait(); err != nil {
		log.Error().Str("op", "youtube/download").Err(err).Msg("yt-dlp command failed")
		return fmt.Errorf("yt-dlp failed: %v", err)
	}
	log.Info().Str("op", "youtube/download").Msgf("yt-dlp download completed for %s", job.URL)
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
