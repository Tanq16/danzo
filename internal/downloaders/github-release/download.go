package ghrelease

import (
	"time"

	"github.com/rs/zerolog/log"
	danzohttp "github.com/tanq16/danzo/internal/downloaders/http"
	"github.com/tanq16/danzo/internal/utils"
)

func (d *GitReleaseDownloader) Download(job *utils.DanzoJob) error {
	downloadURL := job.Metadata["downloadURL"].(string)
	fileSize := job.Metadata["fileSize"].(int64)
	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	progressCh := make(chan int64)
	progressDone := make(chan struct{})
	log.Info().Str("op", "github-release/download").Msgf("downloading %s", downloadURL)

	// Progress tracking goroutine
	go func() {
		defer close(progressDone)
		var totalDownloaded int64
		startTime := time.Now()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		log.Debug().Str("op", "github-release/download").Msg("progress tracking goroutine started")
		for {
			select {
			case bytes, ok := <-progressCh:
				if !ok {
					if job.ProgressFunc != nil {
						job.ProgressFunc(totalDownloaded, fileSize)
					}
					return
				}
				totalDownloaded += bytes
			case <-ticker.C:
				if job.ProgressFunc != nil {
					job.ProgressFunc(totalDownloaded, fileSize)
				}
				job.Metadata["elapsedTime"] = time.Since(startTime).Seconds()
			}
		}
	}()

	log.Debug().Str("op", "github-release/download").Msg("calling simple download")
	err := danzohttp.PerformSimpleDownload(downloadURL, job.OutputPath, client, progressCh)
	<-progressDone
	return err
}
