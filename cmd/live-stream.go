package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
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
			job := utils.DanzoJob{
				JobType:          "live-stream",
				URL:              args[0],
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			if extract != "" {
				job.Metadata["extract"] = extract
			}
			jobs := []utils.DanzoJob{job}
			log.Debug().Str("op", "cmd/live-stream").Msgf("Starting scheduler with %d jobs", len(jobs))
			scheduler.Run(jobs, workers)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: stream_[timestamp].mp4)")
	cmd.Flags().StringVarP(&extract, "extract", "e", "", "Site-specific extractor to use (e.g., rumble)")
	return cmd
}
