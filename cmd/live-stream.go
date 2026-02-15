package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	m3u8job "github.com/tanq16/danzo/internal/jobs/live-stream"
	"github.com/tanq16/danzo/internal/utils"
)

func newM3U8Cmd() *cobra.Command {
	var outputPath string
	var extract string

	cmd := &cobra.Command{
		Use:     "live-stream [URL] [--output OUTPUT_PATH] [--extract EXTRACTOR]",
		Short:   "Download HLS/M3U8 live streams",
		Aliases: []string{"hls", "m3u8", "livestream", "stream"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			url := args[0]
			if extract == "" {
				if strings.Contains(url, "dailymotion.com") || strings.Contains(url, "dai.ly") {
					extract = "dailymotion"
				} else if strings.Contains(url, "rumble.com") {
					extract = "rumble"
				}
			}

			hw := highway.New(workers, ".danzo-resume-state.json")
			hw.RegisterType("live-stream", m3u8job.Unmarshal)

			disp := display.New(display.DefaultConfig())

			job := m3u8job.New(url, outputPath, connections, extract, globalHTTPConfig)
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

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: stream_[timestamp].mp4)")
	cmd.Flags().StringVarP(&extract, "extract", "e", "", "Site-specific extractor to use (e.g., rumble, dailymotion)")
	return cmd
}
