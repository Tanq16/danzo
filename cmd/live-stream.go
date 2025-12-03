package cmd

import (
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newM3U8Cmd() *cobra.Command {
	var outputPath string
	var extract string
	var videoPassword string
	var vimeoUsername string
	var vimeoPassword string

	cmd := &cobra.Command{
		Use:     "live-stream [URL] [--output OUTPUT_PATH] [--extract EXTRACTOR] [--password PASSWORD]",
		Short:   "Download HLS/M3U8 live streams",
		Aliases: []string{"hls", "m3u8", "livestream", "stream"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "live-stream",
				URL:              args[0],
				OutputPath:       outputPath,
				Connections:      connections,
				ProgressType:     "progress",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			if extract == "" {
				if strings.Contains(job.URL, "vimeo.com") {
					extract = "vimeo"
				} else if strings.Contains(job.URL, "dailymotion.com") || strings.Contains(job.URL, "dai.ly") {
					extract = "dailymotion"
				} else if strings.Contains(job.URL, "rumble.com") {
					extract = "rumble"
				}
			}
			if extract != "" {
				job.Metadata["extract"] = extract
			}
			if videoPassword != "" {
				job.Metadata["password"] = videoPassword
			}
			if vimeoUsername != "" && vimeoPassword != "" {
				job.Metadata["vimeo-username"] = vimeoUsername
				job.Metadata["vimeo-password"] = vimeoPassword
			}
			jobs := []utils.DanzoJob{job}
			log.Debug().Str("op", "cmd/live-stream").Msgf("Starting scheduler with %d jobs", len(jobs))
			scheduler.Run(jobs, workers)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: stream_[timestamp].mp4)")
	cmd.Flags().StringVarP(&extract, "extract", "e", "", "Site-specific extractor to use (e.g., rumble, dailymotion, vimeo)")
	cmd.Flags().StringVar(&videoPassword, "video-password", "", "Password for protected videos")
	cmd.Flags().StringVar(&vimeoUsername, "vimeo-username", "", "Username for Vimeo authentication")
	cmd.Flags().StringVar(&vimeoPassword, "vimeo-password", "", "Password for Vimeo authentication")
	return cmd
}
