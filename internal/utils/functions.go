package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func GetRandomUserAgent() string {
	return userAgents[time.Now().UnixNano()%int64(len(userAgents))]
}

func DetermineDownloadType(url string) string {
	if strings.HasPrefix(url, "https://drive.google.com") {
		return "gdrive"
	} else if strings.HasPrefix(url, "s3://") {
		return "s3"
	} else if strings.HasPrefix(url, "https://youtu.be") || strings.HasPrefix(url, "https://www.youtube.com") || strings.HasPrefix(url, "https://music.youtube.com") {
		return "youtube"
	} else if strings.HasPrefix(url, "ftp://") || strings.HasPrefix(url, "ftps://") {
		return "ftp"
	} else if strings.HasPrefix(url, "sftp://") {
		return "sftp"
	} else if strings.HasPrefix(url, "github://") {
		return "gitrelease"
	} else if strings.HasPrefix(url, "github.com") || strings.HasPrefix(url, "gitlab.com") || strings.HasPrefix(url, "bitbucket.org") || strings.HasPrefix(url, "git.com") {
		return "gitclone"
	} else if strings.HasPrefix(url, "m3u8://") {
		return "m3u8"
	}
	return "http"
}

func RenewOutputPath(outputPath string) string {
	dir := filepath.Dir(outputPath)
	base := filepath.Base(outputPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	index := 1
	for {
		outputPath = filepath.Join(dir, fmt.Sprintf("%s-(%d)%s", name, index, ext))
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			return outputPath
		}
		index++
	}
}

func ParseHeaderArgs(headers []string) map[string]string {
	result := make(map[string]string)
	for _, header := range headers {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			result[key] = value
		}
	}
	return result
}

func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func FormatSpeed(bytes int64, elapsed float64) string {
	if elapsed == 0 {
		return "0 B/s"
	}
	bps := float64(bytes) / elapsed
	formatted := FormatBytes(uint64(bps))
	return formatted[:len(formatted)-1] + "B/s" // Slice off "B" and add "B/s"
}

func CleanLocal() error {
	tempDir := filepath.Join(filepath.Dir("."), ".danzo-temp")
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.Remove(filepath.Join(tempDir, file.Name())); err != nil {
			return err
		}
	}
	if err := os.Remove(tempDir); err != nil {
		return err
	}
	return nil
}

func CleanFunction(outputPath string) error {
	tempDir := filepath.Join(filepath.Dir(outputPath), ".danzo-temp")
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}
	partPrefix := filepath.Base(outputPath) + ".part"
	for _, file := range files {
		if strings.HasPrefix(file.Name(), partPrefix) {
			if err := os.Remove(filepath.Join(tempDir, file.Name())); err != nil {
				return err
			}
		}
	}
	remainingFiles, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}
	if len(remainingFiles) == 0 {
		if err := os.Remove(tempDir); err != nil {
			return err
		}
	}
	return nil
}
