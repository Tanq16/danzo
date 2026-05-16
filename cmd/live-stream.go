package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	m3u8job "github.com/tanq16/danzo/internal/jobs/live-stream"
	"github.com/tanq16/danzo/utils"
)

var m3u8Flags struct {
	outputPath string
	extract    string
}

var m3u8Cmd = &cobra.Command{
	Use:     "live-stream [URL] [--output OUTPUT_PATH] [--extract EXTRACTOR]",
	Short:   "Download HLS/M3U8 live streams",
	Aliases: []string{"hls", "m3u8", "livestream", "stream"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		url := args[0]
		extract := m3u8Flags.extract
		if extract == "" {
			if strings.Contains(url, "dailymotion.com") || strings.Contains(url, "dai.ly") {
				extract = "dailymotion"
			} else if strings.Contains(url, "rumble.com") {
				extract = "rumble"
			}
		}

		hw := newHighway()

		disp := display.New(display.DefaultConfig())

		job := m3u8job.New(url, m3u8Flags.outputPath, connections, extract, globalHTTPConfig)
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

func newM3U8Cmd() *cobra.Command {
	return m3u8Cmd
}

func init() {
	m3u8Cmd.Flags().StringVarP(&m3u8Flags.outputPath, "output", "o", "", "Output file path (default: stream_[timestamp].mp4)")
	m3u8Cmd.Flags().StringVarP(&m3u8Flags.extract, "extract", "e", "", "Site-specific extractor to use (e.g., rumble, dailymotion)")
}
