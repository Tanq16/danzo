package scheduler

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	gitclone "github.com/tanq16/danzo/internal/downloaders/git-clone"
	ghrelease "github.com/tanq16/danzo/internal/downloaders/github-release"
	gdrive "github.com/tanq16/danzo/internal/downloaders/google-drive"
	httpDownloader "github.com/tanq16/danzo/internal/downloaders/http"
	m3u8 "github.com/tanq16/danzo/internal/downloaders/live-stream"
	"github.com/tanq16/danzo/internal/downloaders/s3"
	"github.com/tanq16/danzo/internal/downloaders/youtube"
	youtubemusic "github.com/tanq16/danzo/internal/downloaders/youtube-music"
	"github.com/tanq16/danzo/internal/output"
	"github.com/tanq16/danzo/internal/utils"
)

type Scheduler struct {
	outputMgr       *output.Manager
	pauseRequestCh  chan struct{}
	resumeRequestCh chan struct{}
	singleJobMode   bool
}

var downloaderRegistry = map[string]utils.Downloader{
	"http":           &httpDownloader.HTTPDownloader{},
	"s3":             &s3.S3Downloader{},
	"google-drive":   &gdrive.GDriveDownloader{},
	"git-clone":      &gitclone.GitCloneDownloader{},
	"github-release": &ghrelease.GitReleaseDownloader{},
	"live-stream":    &m3u8.M3U8Downloader{},
	"youtube":        &youtube.YouTubeDownloader{},
	"youtube-music":  &youtubemusic.YTMusicDownloader{},
}

func Run(jobs []utils.DanzoJob, numWorkers int) {
	s := &Scheduler{
		outputMgr:       output.NewManager(),
		pauseRequestCh:  make(chan struct{}),
		resumeRequestCh: make(chan struct{}),
		singleJobMode:   len(jobs) == 1,
	}
	log.Debug().Str("op", "scheduler").Msgf("Starting output manager")
	s.outputMgr.StartDisplay()
	defer s.outputMgr.StopDisplay()

	outputDirs := make(map[string]bool)
	allSuccessful := true
	var mu sync.Mutex
	if s.singleJobMode {
		go s.handlePauseResume()
	}

	log.Debug().Str("op", "scheduler").Msgf("Send jobs to pipeline")
	jobCh := make(chan utils.DanzoJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	log.Debug().Str("op", "scheduler").Msgf("Start %d workers", numWorkers)
	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.processJobs(jobCh, &outputDirs, &allSuccessful, &mu)
		}()
	}

	wg.Wait()
	log.Debug().Str("op", "scheduler").Msgf("All workers done")
	if allSuccessful {
		log.Debug().Str("op", "scheduler").Msgf("Clean up output dirs after successful jobs")
		for dir := range outputDirs {
			utils.CleanFunction(dir)
		}
	} else {
		log.Error().Str("op", "scheduler").Msgf("Not all jobs were successful")
	}
}

func (s *Scheduler) handlePauseResume() {
	for {
		select {
		case <-s.pauseRequestCh:
			s.outputMgr.Pause()
		case <-s.resumeRequestCh:
			s.outputMgr.Resume()
		}
	}
}

func (s *Scheduler) processJobs(jobCh <-chan utils.DanzoJob, outputDirs *map[string]bool, allSuccessful *bool, mu *sync.Mutex) {
	for job := range jobCh {
		log.Debug().Str("op", "scheduler/processJobs").Msgf("Processing job %s", job.OutputPath)
		funcID := s.outputMgr.RegisterFunction(job.OutputPath)
		downloader, exists := downloaderRegistry[job.JobType]
		if !exists {
			s.outputMgr.ReportError(funcID, fmt.Errorf("unknown job type: %s", job.JobType))
			s.outputMgr.SetMessage(funcID, fmt.Sprintf("Error: Unknown job type %s", job.JobType))
			continue
		}

		log.Debug().Str("op", "scheduler/processJobs").Msgf("Downloader found for %s", job.JobType)
		s.outputMgr.SetStatus(funcID, "pending")
		s.outputMgr.SetMessage(funcID, fmt.Sprintf("Validating %s job", job.JobType))
		log.Debug().Str("op", "scheduler/processJobs").Msgf("Validating job %s", job.OutputPath)
		err := downloader.ValidateJob(&job)
		if err != nil {
			log.Error().Str("op", "scheduler/processJobs").Msgf("Validation failed for %s", job.OutputPath)
			s.outputMgr.ReportError(funcID, fmt.Errorf("validation failed: %v", err))
			s.outputMgr.SetMessage(funcID, fmt.Sprintf("Validation failed for %s", job.OutputPath))
			continue
		}

		log.Info().Str("op", "scheduler/processJobs").Msgf("Preparing job %s", job.OutputPath)
		s.outputMgr.SetMessage(funcID, fmt.Sprintf("Preparing %s job", job.JobType))
		if s.singleJobMode {
			job.PauseFunc = func() { s.pauseRequestCh <- struct{}{} }
			job.ResumeFunc = func() { s.resumeRequestCh <- struct{}{} }
		}
		err = downloader.BuildJob(&job)
		if err != nil {
			log.Error().Str("op", "scheduler/processJobs").Msgf("Build failed for %s", job.OutputPath)
			if err.Error() == "file already exists with same size" {
				s.outputMgr.SetStatus(funcID, "success")
				s.outputMgr.SetMessage(funcID, fmt.Sprintf("File already exists: %s", job.OutputPath))
				s.outputMgr.Complete(funcID, "")
				continue
			}
			s.outputMgr.ReportError(funcID, fmt.Errorf("build failed: %v", err))
			s.outputMgr.SetMessage(funcID, fmt.Sprintf("Build failed for %s", job.OutputPath))
			continue
		}

		log.Debug().Str("op", "scheduler/processJobs").Msgf("Setting progress type for %s", job.OutputPath)
		switch job.ProgressType {
		case "progress":
			job.ProgressFunc = func(downloaded, total int64) {
				if total > 0 {
					s.outputMgr.AddProgressBarToStream(funcID, downloaded, total)
				}
			}
		case "stream":
			job.StreamFunc = func(line string) {
				s.outputMgr.AddStreamLine(funcID, line)
			}
		}

		log.Info().Str("op", "scheduler/processJobs").Msgf("Performing download for %s", job.OutputPath)
		s.outputMgr.SetMessage(funcID, fmt.Sprintf("Downloading %s", job.OutputPath))
		err = downloader.Download(&job)
		if err != nil {
			log.Error().Str("op", "scheduler/processJobs").Msgf("Download failed for %s", job.OutputPath)
			mu.Lock()
			*allSuccessful = false
			mu.Unlock()
			s.outputMgr.ReportError(funcID, fmt.Errorf("download failed: %v", err))
			s.outputMgr.SetMessage(funcID, fmt.Sprintf("Download failed for %s", job.OutputPath))
			continue
		}
		log.Info().Str("op", "scheduler/processJobs").Msgf("Download completed for %s", job.OutputPath)
		mu.Lock()
		(*outputDirs)[job.OutputPath] = true
		mu.Unlock()

		s.outputMgr.Complete(funcID, fmt.Sprintf("Completed %s", job.OutputPath))
	}
}
