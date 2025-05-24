package scheduler

import (
	"fmt"
	"sync"

	httpDownloader "github.com/tanq16/danzo/internal/downloaders/http"
	"github.com/tanq16/danzo/internal/output"
	"github.com/tanq16/danzo/internal/utils"
)

var downloaderRegistry = map[string]utils.Downloader{
	"http": &httpDownloader.HTTPDownloader{},
	// "s3":         &s3Downloader{},
	// "youtube":    &youtubeDownloader{},
	// "gdrive":     &gdriveDownloader{},
	// "gitclone":   &gitCloneDownloader{},
	// "gitrelease": &gitReleaseDownloader{},
	// "m3u8":       &m3u8Downloader{},
}

func Run(jobs []utils.DanzoJob, numWorkers int) error {
	outputMgr := output.NewManager()
	outputMgr.StartDisplay()
	defer outputMgr.StopDisplay()

	jobCh := make(chan utils.DanzoJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processJobs(jobCh, outputMgr)
		}(i)
	}

	wg.Wait()
	return nil
}

func processJobs(jobCh <-chan utils.DanzoJob, outputMgr *output.Manager) {
	for job := range jobCh {
		funcID := outputMgr.RegisterFunction(job.OutputPath)
		downloader, exists := downloaderRegistry[job.JobType]
		if !exists {
			outputMgr.ReportError(funcID, fmt.Errorf("unknown job type: %s", job.JobType))
			outputMgr.SetMessage(funcID, fmt.Sprintf("Error: Unknown job type %s", job.JobType))
			continue
		}

		outputMgr.SetStatus(funcID, "pending")
		outputMgr.SetMessage(funcID, fmt.Sprintf("Validating %s job", job.JobType))
		err := downloader.ValidateJob(&job)
		if err != nil {
			outputMgr.ReportError(funcID, fmt.Errorf("validation failed: %v", err))
			outputMgr.SetMessage(funcID, fmt.Sprintf("Validation failed for %s", job.OutputPath))
			continue
		}

		outputMgr.SetMessage(funcID, fmt.Sprintf("Preparing %s job", job.JobType))
		err = downloader.BuildJob(&job)
		if err != nil {
			if err.Error() == "file already exists with same size" {
				outputMgr.SetStatus(funcID, "success")
				outputMgr.SetMessage(funcID, fmt.Sprintf("File already exists: %s", job.OutputPath))
				outputMgr.Complete(funcID, "")
				continue
			}
			outputMgr.ReportError(funcID, fmt.Errorf("build failed: %v", err))
			outputMgr.SetMessage(funcID, fmt.Sprintf("Build failed for %s", job.OutputPath))
			continue
		}

		if job.ProgressType == "progress" {
			job.ProgressFunc = func(downloaded, total int64) {
				if total > 0 {
					outputMgr.AddProgressBarToStream(funcID, downloaded, total, utils.FormatBytes(uint64(downloaded)))
				}
			}
		} else if job.ProgressType == "stream" {
			job.StreamFunc = func(line string) {
				outputMgr.AddStreamLine(funcID, line)
			}
		}

		outputMgr.SetMessage(funcID, fmt.Sprintf("Downloading %s", job.OutputPath))
		err = downloader.Download(&job)
		if err != nil {
			outputMgr.ReportError(funcID, fmt.Errorf("download failed: %v", err))
			outputMgr.SetMessage(funcID, fmt.Sprintf("Download failed for %s", job.OutputPath))
			continue
		}
		outputMgr.Complete(funcID, fmt.Sprintf("Completed %s", job.OutputPath))
	}
}
