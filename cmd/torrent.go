package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	torrentjob "github.com/tanq16/danzo/internal/jobs/torrent"
	"github.com/tanq16/danzo/utils"
)

var torrentFlags struct {
	outputPath string
}

var torrentCmd = &cobra.Command{
	Use:   "torrent [URI] [--output OUTPUT_PATH]",
	Short: "Download file via Torrent or Magnet link",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		hw := newHighway()

		disp := display.New(display.DefaultConfig())

		job := torrentjob.New(args[0], torrentFlags.outputPath, connections, globalHTTPConfig)
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

func newTorrentCmd() *cobra.Command {
	return torrentCmd
}

func init() {
	torrentCmd.Flags().StringVarP(&torrentFlags.outputPath, "output", "o", "", "Output directory path")
}
