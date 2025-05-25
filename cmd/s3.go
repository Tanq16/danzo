package cmd

import (
	"github.com/spf13/cobra"
)

func newS3Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "s3",
		Short: "Download files from S3",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// url := args[0]
			// utils.DownloadFromS3(url, outputPath, debug)
		},
	}
}
