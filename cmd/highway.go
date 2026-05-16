package cmd

import (
	"github.com/tanq16/danzo/internal/highway"
	gitclonejob "github.com/tanq16/danzo/internal/jobs/git-clone"
	ghreleasejob "github.com/tanq16/danzo/internal/jobs/github-release"
	gdrivejob "github.com/tanq16/danzo/internal/jobs/google-drive"
	httpjob "github.com/tanq16/danzo/internal/jobs/http"
	m3u8job "github.com/tanq16/danzo/internal/jobs/live-stream"
	s3job "github.com/tanq16/danzo/internal/jobs/s3"
)

const resumeStatePath = ".danzo-resume-state.json"

func newHighway() *highway.Highway {
	hw := highway.New(workers, resumeStatePath)
	registerJobTypes(hw)
	return hw
}

func registerJobTypes(hw *highway.Highway) {
	hw.RegisterType("http", httpjob.Unmarshal)
	hw.RegisterType("s3", s3job.Unmarshal)
	hw.RegisterType("git-clone", gitclonejob.Unmarshal)
	hw.RegisterType("github-release", ghreleasejob.Unmarshal)
	hw.RegisterType("google-drive", gdrivejob.Unmarshal)
	hw.RegisterType("live-stream", m3u8job.Unmarshal)
}
