package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newYouTubeCmd() *cobra.Command {
	var outputPath string
	var format string

	cmd := &cobra.Command{
		Use:   "yt [URL] [--output OUTPUT_PATH] [--format FORMAT]",
		Short: "Download YouTube videos",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "youtube",
				URL:              args[0],
				OutputPath:       outputPath,
				ProgressType:     "stream",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			if format != "" {
				job.Metadata["format"] = format
			}
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers, fileLog)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	cmd.Flags().StringVarP(&format, "format", "f", "decent", "Video format (best, 1080p, 720p, etc.)")
	return cmd
}
