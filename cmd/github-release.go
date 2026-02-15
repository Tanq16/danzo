package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	ghreleasejob "github.com/tanq16/danzo/internal/jobs/github-release"
	"github.com/tanq16/danzo/internal/utils"
)

func newGHReleaseCmd() *cobra.Command {
	var outputPath string
	var manual bool

	cmd := &cobra.Command{
		Use:     "github-release [USER/REPO or URL] [--output OUTPUT_PATH] [--manual]",
		Short:   "Download a release asset for a GitHub repository",
		Aliases: []string{"ghrelease", "ghr"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			hw := highway.New(workers, ".danzo-resume-state.json")
			hw.RegisterType("github-release", ghreleasejob.Unmarshal)

			disp := display.New(display.DefaultConfig())

			job := ghreleasejob.New(args[0], outputPath, manual, globalHTTPConfig)
			job.PauseDisplay = disp.Pause
			job.ResumeDisplay = disp.Resume
			disp.RegisterJob(job.ID())
			hw.Submit(job)

			disp.Start(hw.Progress())
			err := hw.Run(ctx)
			disp.Stop()

			if err != nil {
				utils.PrintFatal("Download failed", err)
			}
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	cmd.Flags().BoolVar(&manual, "manual", false, "Manually select release version and asset")
	return cmd
}
