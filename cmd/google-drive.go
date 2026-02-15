package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	gdrivejob "github.com/tanq16/danzo/internal/jobs/google-drive"
	"github.com/tanq16/danzo/internal/utils"
)

func newGDriveCmd() *cobra.Command {
	var outputPath string
	var apiKey string
	var credentialsFile string

	cmd := &cobra.Command{
		Use:     "google-drive [URL] [--output OUTPUT_PATH] [--api-key YOUR_KEY] [--creds creds.json]",
		Short:   "Download files or folders from Google Drive",
		Aliases: []string{"gdrive", "gd", "drive"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			hw := highway.New(workers, ".danzo-resume-state.json")
			hw.RegisterType("google-drive", gdrivejob.Unmarshal)

			disp := display.New(display.DefaultConfig())

			job := gdrivejob.New(args[0], outputPath, apiKey, credentialsFile, globalHTTPConfig)
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

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output path")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Google Drive API key")
	cmd.Flags().StringVar(&credentialsFile, "creds", "", "OAuth credentials JSON file")
	return cmd
}
