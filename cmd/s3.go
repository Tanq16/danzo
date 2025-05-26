package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newS3Cmd() *cobra.Command {
	var outputPath string
	var profile string

	cmd := &cobra.Command{
		Use:   "s3 [BUCKET/KEY or s3://BUCKET/KEY] [--output OUTPUT_PATH] [--profile PROFILE]",
		Short: "Download files from AWS S3",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "s3",
				URL:              args[0],
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			job.Metadata["profile"] = profile
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers, fileLog)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output path")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "AWS profile to use")
	return cmd
}
