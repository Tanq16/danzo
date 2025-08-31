package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newGDriveCmd() *cobra.Command {
	var outputPath string
	var apiKey string
	var credentialsFile string

	cmd := &cobra.Command{
		Use:     "google-drive [URL] [--output OUTPUT_PATH] [--api-key YOUR_KEY] [--creds creds.json]",
		Short:   "Download files or folders from Google Drive",
		Aliases: []string{"gdrive", "gd", "drive"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "google-drive",
				URL:              args[0],
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			if apiKey != "" {
				job.Metadata["apiKey"] = apiKey
			}
			if credentialsFile != "" {
				job.Metadata["credentialsFile"] = credentialsFile
			}
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output path")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Google Drive API key")
	cmd.Flags().StringVar(&credentialsFile, "creds", "", "OAuth credentials JSON file")
	return cmd
}
