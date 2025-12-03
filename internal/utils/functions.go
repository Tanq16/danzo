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
	_, err := os.Stat(tempDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.RemoveAll(tempDir); err != nil {
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
		filePath := filepath.Join(tempDir, file.Name())
		if strings.HasPrefix(file.Name(), partPrefix) {
			if file.IsDir() {
				if err := os.RemoveAll(filePath); err != nil {
					return err
				}
			} else {
				if err := os.Remove(filePath); err != nil {
					return err
				}
			}
		}
		// Also remove m3u8_* directories (from live-stream downloads)
		if file.IsDir() && strings.HasPrefix(file.Name(), "m3u8_") {
			if err := os.RemoveAll(filePath); err != nil {
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
