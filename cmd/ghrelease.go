package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newGHReleaseCmd() *cobra.Command {
	var outputPath string
	var manual bool

	cmd := &cobra.Command{
		Use:   "ghrelease [USER/REPO or URL]",
		Short: "Download a release asset for a GitHub repository",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "ghrelease",
				URL:              args[0],
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			job.Metadata["manual"] = manual
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers, debug)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	cmd.Flags().BoolVar(&manual, "manual", false, "Manually select release version and asset")
	return cmd
}
