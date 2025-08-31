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
		Use:     "github-release [USER/REPO or URL] [--output OUTPUT_PATH] [--manual]",
		Short:   "Download a release asset for a GitHub repository",
		Aliases: []string{"ghrelease", "ghr"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "github-release",
				URL:              args[0],
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			job.Metadata["manual"] = manual
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	cmd.Flags().BoolVar(&manual, "manual", false, "Manually select release version and asset")
	return cmd
}
