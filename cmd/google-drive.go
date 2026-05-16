package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	gdrivejob "github.com/tanq16/danzo/internal/jobs/google-drive"
	"github.com/tanq16/danzo/utils"
)

var gdriveFlags struct {
	outputPath      string
	apiKey          string
	credentialsFile string
}

var gdriveCmd = &cobra.Command{
	Use:     "google-drive [URL] [--output OUTPUT_PATH] [--api-key YOUR_KEY] [--creds creds.json]",
	Short:   "Download files or folders from Google Drive",
	Aliases: []string{"gdrive", "gd", "drive"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		hw := newHighway()

		disp := display.New(display.DefaultConfig())

		job := gdrivejob.New(args[0], gdriveFlags.outputPath, gdriveFlags.apiKey, gdriveFlags.credentialsFile, globalHTTPConfig)
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

func newGDriveCmd() *cobra.Command {
	return gdriveCmd
}

func init() {
	gdriveCmd.Flags().StringVarP(&gdriveFlags.outputPath, "output", "o", "", "Output path")
	gdriveCmd.Flags().StringVar(&gdriveFlags.apiKey, "api-key", "", "Google Drive API key")
	gdriveCmd.Flags().StringVar(&gdriveFlags.credentialsFile, "creds", "", "OAuth credentials JSON file")
}
