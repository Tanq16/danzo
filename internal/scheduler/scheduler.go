package scheduler

import (
	"fmt"
	"sync"

	"github.com/tanq16/danzo/internal/downloaders/gdrive"
	"github.com/tanq16/danzo/internal/downloaders/ghrelease"
	"github.com/tanq16/danzo/internal/downloaders/gitclone"
	httpDownloader "github.com/tanq16/danzo/internal/downloaders/http"
	"github.com/tanq16/danzo/internal/downloaders/m3u8"
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
	"http":      &httpDownloader.HTTPDownloader{},
	"s3":        &s3.S3Downloader{},
	"gdrive":    &gdrive.GDriveDownloader{},
	"gitclone":  &gitclone.GitCloneDownloader{},
	"ghrelease": &ghrelease.GitReleaseDownloader{},
	"m3u8":      &m3u8.M3U8Downloader{},
	"youtube":   &youtube.YouTubeDownloader{},
	"ytmusic":   &youtubemusic.YTMusicDownloader{},
}

func Run(jobs []utils.DanzoJob, numWorkers int, fileLog bool) {
	s := &Scheduler{
		outputMgr:       output.NewManager(),
		pauseRequestCh:  make(chan struct{}),
		resumeRequestCh: make(chan struct{}),
		singleJobMode:   len(jobs) == 1,
	}
	s.outputMgr.StartDisplay()
	defer s.outputMgr.StopDisplay()

	outputDirs := make(map[string]bool)
	allSuccessful := true
	var mu sync.Mutex
	if s.singleJobMode {
		go s.handlePauseResume()
	}

	jobCh := make(chan utils.DanzoJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.processJobs(jobCh, &outputDirs, &allSuccessful, &mu)
		}()
	}

	wg.Wait()
	if allSuccessful {
		for dir := range outputDirs {
			utils.CleanFunction(dir)
		}
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
		funcID := s.outputMgr.RegisterFunction(job.OutputPath)
		downloader, exists := downloaderRegistry[job.JobType]
		if !exists {
			s.outputMgr.ReportError(funcID, fmt.Errorf("unknown job type: %s", job.JobType))
			s.outputMgr.SetMessage(funcID, fmt.Sprintf("Error: Unknown job type %s", job.JobType))
			continue
		}

		s.outputMgr.SetStatus(funcID, "pending")
		s.outputMgr.SetMessage(funcID, fmt.Sprintf("Validating %s job", job.JobType))
		err := downloader.ValidateJob(&job)
		if err != nil {
			s.outputMgr.ReportError(funcID, fmt.Errorf("validation failed: %v", err))
			s.outputMgr.SetMessage(funcID, fmt.Sprintf("Validation failed for %s", job.OutputPath))
			continue
		}

		s.outputMgr.SetMessage(funcID, fmt.Sprintf("Preparing %s job", job.JobType))
		if s.singleJobMode {
			job.PauseFunc = func() { s.pauseRequestCh <- struct{}{} }
			job.ResumeFunc = func() { s.resumeRequestCh <- struct{}{} }
		}
		err = downloader.BuildJob(&job)
		if err != nil {
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

		if job.ProgressType == "progress" {
			job.ProgressFunc = func(downloaded, total int64) {
				if total > 0 {
					s.outputMgr.AddProgressBarToStream(funcID, downloaded, total, utils.FormatBytes(uint64(downloaded)))
				}
			}
		} else if job.ProgressType == "stream" {
			job.StreamFunc = func(line string) {
				s.outputMgr.AddStreamLine(funcID, line)
			}
		}

		s.outputMgr.SetMessage(funcID, fmt.Sprintf("Downloading %s", job.OutputPath))
		err = downloader.Download(&job)
		if err != nil {
			mu.Lock()
			*allSuccessful = false
			mu.Unlock()
			s.outputMgr.ReportError(funcID, fmt.Errorf("download failed: %v", err))
			s.outputMgr.SetMessage(funcID, fmt.Sprintf("Download failed for %s", job.OutputPath))
			continue
		}
		mu.Lock()
		(*outputDirs)[job.OutputPath] = true
		mu.Unlock()

		s.outputMgr.Complete(funcID, fmt.Sprintf("Completed %s", job.OutputPath))
	}
}
