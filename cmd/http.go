package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newHTTPCmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "http [URL] [--output OUTPUT_PATH]",
		Short: "Download file via HTTP/HTTPS",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			url := args[0]
			job := utils.DanzoJob{
				JobType:          "http",
				URL:              url,
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			jobs := []utils.DanzoJob{job}
			log.Debug().Str("op", "cmd/http").Msgf("Starting scheduler with %d jobs", len(jobs))
			scheduler.Run(jobs, workers)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	return cmd
}
