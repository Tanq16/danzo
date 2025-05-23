package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/tanq16/danzo/internal/output"
	"github.com/tanq16/danzo/internal/utils"
)

// downloaderRegistry maps job types to their respective downloader implementations
var downloaderRegistry = map[string]utils.Downloader{
	// "http":       &httpDownloader{},
	// "s3":         &s3Downloader{},
	// "youtube":    &youtubeDownloader{},
	// "gdrive":     &gdriveDownloader{},
	// "gitclone":   &gitCloneDownloader{},
	// "gitrelease": &gitReleaseDownloader{},
	// "m3u8":       &m3u8Downloader{},
}

// Run executes the scheduler with the given jobs and number of workers
func Run(jobs []utils.DanzoJob, numWorkers int) error {
	// Initialize output manager
	outputMgr := output.NewManager()
	outputMgr.StartDisplay()
	defer outputMgr.StopDisplay()

	// Create job channel
	jobCh := make(chan utils.DanzoJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processJobs(jobCh, outputMgr)
		}(i)
	}

	// Wait for all workers to complete
	wg.Wait()

	return nil
}

// processJobs handles job processing for a worker
func processJobs(jobCh <-chan utils.DanzoJob, outputMgr *output.Manager) {
	for job := range jobCh {
		// Register job with output manager
		funcID := outputMgr.RegisterFunction(job.OutputPath)

		// Get downloader for job type
		_, exists := downloaderRegistry[job.JobType]
		if !exists {
			outputMgr.ReportError(funcID, fmt.Errorf("unknown job type: %s", job.JobType))
			outputMgr.SetMessage(funcID, fmt.Sprintf("Error: Unknown job type %s", job.JobType))
			continue
		}

		// Validate job
		outputMgr.SetStatus(funcID, "pending")
		outputMgr.SetMessage(funcID, fmt.Sprintf("Validating %s job", job.JobType))

		// err := downloader.ValidateJob(&job)
		// if err != nil {
		// 	outputMgr.ReportError(funcID, fmt.Errorf("validation failed: %v", err))
		// 	outputMgr.SetMessage(funcID, fmt.Sprintf("Validation failed for %s", job.OutputPath))
		// 	continue
		// }

		// Simulate validation for now
		time.Sleep(500 * time.Millisecond)

		// Build job
		outputMgr.SetMessage(funcID, fmt.Sprintf("Building %s job", job.JobType))

		// err = downloader.BuildJob(&job)
		// if err != nil {
		// 	outputMgr.ReportError(funcID, fmt.Errorf("build failed: %v", err))
		// 	outputMgr.SetMessage(funcID, fmt.Sprintf("Build failed for %s", job.OutputPath))
		// 	continue
		// }

		// Simulate build for now
		time.Sleep(500 * time.Millisecond)

		// Download
		outputMgr.SetMessage(funcID, fmt.Sprintf("Downloading %s", job.OutputPath))

		// err = downloader.Download(&job)
		// if err != nil {
		// 	outputMgr.ReportError(funcID, fmt.Errorf("download failed: %v", err))
		// 	outputMgr.SetMessage(funcID, fmt.Sprintf("Download failed for %s", job.OutputPath))
		// 	continue
		// }

		// Simulate download for now
		time.Sleep(2 * time.Second)

		// Mark complete
		outputMgr.Complete(funcID, fmt.Sprintf("Completed %s", job.OutputPath))
	}
}
