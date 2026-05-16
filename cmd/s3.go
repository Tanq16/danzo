package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	s3job "github.com/tanq16/danzo/internal/jobs/s3"
	"github.com/tanq16/danzo/utils"
)

var s3Flags struct {
	outputPath string
	profile    string
}

var s3Cmd = &cobra.Command{
	Use:   "s3 [BUCKET/KEY or s3://BUCKET/KEY] [--output OUTPUT_PATH] [--profile PROFILE]",
	Short: "Download files from AWS S3",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		hw := newHighway()

		disp := display.New(display.DefaultConfig())

		job := s3job.New(args[0], s3Flags.outputPath, connections, s3Flags.profile)
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

func newS3Cmd() *cobra.Command {
	return s3Cmd
}

func init() {
	s3Cmd.Flags().StringVarP(&s3Flags.outputPath, "output", "o", "", "Output path")
	s3Cmd.Flags().StringVar(&s3Flags.profile, "profile", "default", "AWS profile to use")
}
