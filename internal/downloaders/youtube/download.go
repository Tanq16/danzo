package youtube

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

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
