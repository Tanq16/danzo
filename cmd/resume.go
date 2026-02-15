package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	gitclonejob "github.com/tanq16/danzo/internal/jobs/git-clone"
	ghreleasejob "github.com/tanq16/danzo/internal/jobs/github-release"
	gdrivejob "github.com/tanq16/danzo/internal/jobs/google-drive"
	httpjob "github.com/tanq16/danzo/internal/jobs/http"
	m3u8job "github.com/tanq16/danzo/internal/jobs/live-stream"
	s3job "github.com/tanq16/danzo/internal/jobs/s3"
	"github.com/tanq16/danzo/internal/utils"
)

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume interrupted downloads",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			hw := highway.New(workers, ".danzo-resume-state.json")
			hw.RegisterType("http", httpjob.Unmarshal)
			hw.RegisterType("s3", s3job.Unmarshal)
			hw.RegisterType("git-clone", gitclonejob.Unmarshal)
			hw.RegisterType("github-release", ghreleasejob.Unmarshal)
			hw.RegisterType("google-drive", gdrivejob.Unmarshal)
			hw.RegisterType("live-stream", m3u8job.Unmarshal)

			if err := hw.LoadState(); err != nil {
				utils.PrintFatal("Failed to load resume state", err)
			}

			disp := display.New(display.DefaultConfig())
			for _, id := range hw.PendingJobIDs() {
				disp.RegisterJob(id)
			}

			disp.Start(hw.Progress())
			err := hw.Run(ctx)
			disp.Stop()

			if err != nil {
				utils.PrintFatal("Resume failed", err)
			}
		},
	}
}
