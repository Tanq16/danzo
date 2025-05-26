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
		Use:     "yt [URL]",
		Aliases: []string{"youtube"},
		Short:   "Download YouTube videos",
		Long: `Download videos from YouTube using yt-dlp.

Supported formats:
  best, best60, bestmp4, decent, decent60, cheap,
  1080p, 1080p60, 720p, 480p

Example:
  danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ" -f 1080p`,
		Args: cobra.ExactArgs(1),
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
