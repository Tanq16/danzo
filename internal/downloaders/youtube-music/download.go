package youtubemusic

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

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

	go processStream(stdout, job.StreamFunc)
	go processStream(stderr, job.StreamFunc)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("yt-dlp failed: %v", err)
	}

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
		err := addMusicMetadata(tempOutput+".m4a", finalPath, musicClient, musicID, job.StreamFunc)
		if err != nil {
			if job.StreamFunc != nil {
				job.StreamFunc(fmt.Sprintf("Warning: Failed to add metadata: %v", err))
			}
		}
	}
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
