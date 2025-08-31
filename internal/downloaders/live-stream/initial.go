package m3u8

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/tanq16/danzo/internal/utils"
)

type M3U8Downloader struct{}

func (d *M3U8Downloader) ValidateJob(job *utils.DanzoJob) error {
	parsedURL, err := url.Parse(job.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}
	return nil
}

func (d *M3U8Downloader) BuildJob(job *utils.DanzoJob) error {
	if job.OutputPath == "" {
		job.OutputPath = fmt.Sprintf("stream_%s.mp4", time.Now().Format("2006-01-02_15-04"))
	}
	if existingFile, err := os.Stat(job.OutputPath); err == nil && existingFile != nil {
		job.OutputPath = utils.RenewOutputPath(job.OutputPath)
	}
	tempDir := filepath.Join(filepath.Dir(job.OutputPath), ".danzo-temp", "m3u8_"+time.Now().Format("20060102150405"))
	job.Metadata["tempDir"] = tempDir
	return nil
}
