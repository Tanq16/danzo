package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newYouTubeCmd() *cobra.Command {
	var outputPath string
	var format string

	cmd := &cobra.Command{
		Use:     "youtube [URL] [--output OUTPUT_PATH] [--format FORMAT]",
		Short:   "Download YouTube videos",
		Aliases: []string{"yt"},
		Args:    cobra.ExactArgs(1),
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
			log.Debug().Str("op", "cmd/youtube").Msgf("Starting scheduler with %d jobs", len(jobs))
			scheduler.Run(jobs, workers)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	cmd.Flags().StringVar(&format, "format", "decent", "Video format (best, 1080p, 720p, etc.)")
	return cmd
}
