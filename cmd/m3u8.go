package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newM3U8Cmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "m3u8 [URL] [--output OUTPUT_PATH]",
		Short: "Download HLS/M3U8 streams",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "m3u8",
				URL:              args[0],
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers, fileLog)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: stream_[timestamp].mp4)")
	return cmd
}
