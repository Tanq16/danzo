package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	ytdlpjob "github.com/tanq16/danzo/internal/jobs/ytdlp"
	"github.com/tanq16/danzo/utils"
)

var ytdlpFlags struct {
	outputPath string
}

var ytdlpCmd = &cobra.Command{
	Use:     "ytdlp [URL] [--output OUTPUT_PATH]",
	Short:   "Download using yt-dlp",
	Aliases: []string{"yt-dlp", "youtube-dl", "ytdl"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		hw := newHighway()

		disp := display.New(display.DefaultConfig())

		job := ytdlpjob.New(args[0], ytdlpFlags.outputPath)
		disp.RegisterJob(job.ID())
		hw.Submit(job)

		disp.Start(hw.Progress())
		err := hw.Run(ctx)
		disp.Stop()

		if err != nil {
			utils.PrintFatal("yt-dlp download failed", err)
		}
	},
}

func newYtdlpCmd() *cobra.Command {
	return ytdlpCmd
}

func init() {
	ytdlpCmd.Flags().StringVarP(&ytdlpFlags.outputPath, "output", "o", "", "Output path for the download")
}
