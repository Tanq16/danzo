package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newYTMusicCmd() *cobra.Command {
	var outputPath string
	var deezerID string
	var appleID string

	cmd := &cobra.Command{
		Use:     "youtube-music [URL] [--output OUTPUT_PATH] [--deezer DEEZER_ID] [--apple APPLE_ID]",
		Short:   "Download YouTube music with metadata",
		Aliases: []string{"ytm", "yt-music"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "youtube-music",
				URL:              args[0],
				OutputPath:       outputPath,
				ProgressType:     "stream",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			if deezerID != "" {
				job.Metadata["musicClient"] = "deezer"
				job.Metadata["musicID"] = deezerID
			} else if appleID != "" {
				job.Metadata["musicClient"] = "apple"
				job.Metadata["musicID"] = appleID
			}
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	cmd.Flags().StringVar(&deezerID, "deezer", "", "Deezer track ID for metadata")
	cmd.Flags().StringVar(&appleID, "apple", "", "Apple Music track ID for metadata")
	return cmd
}
