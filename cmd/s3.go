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
		Use:   "s3 [BUCKET/KEY or s3://BUCKET/KEY]",
		Short: "Download files from AWS S3",
		Long: `Download files or folders from AWS S3.

Examples:
  danzo s3 mybucket/path/to/file.zip
  danzo s3 s3://mybucket/path/to/folder/
  danzo s3 mybucket/file.zip --profile myprofile`,
		Args: cobra.ExactArgs(1),
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

			// Add profile to metadata
			job.Metadata["profile"] = profile

			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers, fileLog)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output path")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "AWS profile to use")

	return cmd
}
