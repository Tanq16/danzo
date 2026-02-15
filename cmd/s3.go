package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	s3job "github.com/tanq16/danzo/internal/jobs/s3"
	"github.com/tanq16/danzo/internal/utils"
)

func newS3Cmd() *cobra.Command {
	var outputPath string
	var profile string

	cmd := &cobra.Command{
		Use:   "s3 [BUCKET/KEY or s3://BUCKET/KEY] [--output OUTPUT_PATH] [--profile PROFILE]",
		Short: "Download files from AWS S3",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			hw := highway.New(workers, ".danzo-resume-state.json")
			hw.RegisterType("s3", s3job.Unmarshal)

			disp := display.New(display.DefaultConfig())

			job := s3job.New(args[0], outputPath, connections, profile)
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

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output path")
	cmd.Flags().StringVar(&profile, "profile", "default", "AWS profile to use")
	return cmd
}
