package youtube

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/tanq16/danzo/utils"
)

func isYtDlpAvailable() (string, bool) {
	// Checks if "yt-dlp" is in PATH or current directory
	path, err := exec.LookPath("yt-dlp")
	if err == nil {
		return path, true
	}
	executableDir, err := os.Executable()
	if err == nil {
		ytdlpPath := filepath.Join(filepath.Dir(executableDir), "yt-dlp")
		if _, err := os.Stat(ytdlpPath); err == nil {
			return ytdlpPath, true
		}
		ytdlpPathExe := filepath.Join(filepath.Dir(executableDir), "yt-dlp.exe")
		if _, err := os.Stat(ytdlpPathExe); err == nil {
			return ytdlpPathExe, true
		}
	}
	return "", false
}

func DownloadYouTubeVideo(url string, outputPath string) error {
	log := utils.GetLogger("youtube-downloader")
	ytdlpPath, available := isYtDlpAvailable()
	if !available {
		return fmt.Errorf("yt-dlp is not installed or not in PATH. Please install yt-dlp to download YouTube videos")
	}
	log.Debug().Str("url", url).Str("output", outputPath).Msg("Starting YouTube download with yt-dlp")
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}

	cmd := exec.Command(ytdlpPath,
		"--no-warnings",
		"-f", "bestvideo+bestaudio/best",
		"-o", outputPath,
		"--no-playlist",
		url,
	)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return fmt.Errorf("yt-dlp exited with code %d: %s", status.ExitStatus(), string(output))
			}
		}
		return fmt.Errorf("error executing yt-dlp: %v - %s", err, string(output))
	}
	log.Debug().Str("url", url).Str("output", outputPath).Msg("YouTube download completed successfully")
	return nil
}
