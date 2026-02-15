package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	httpjob "github.com/tanq16/danzo/internal/jobs/http"
	"github.com/tanq16/danzo/internal/utils"
)

func newHTTPCmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "http [URL] [--output OUTPUT_PATH]",
		Short: "Download file via HTTP/HTTPS",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			hw := highway.New(workers, ".danzo-resume-state.json")
			hw.RegisterType("http", httpjob.Unmarshal)

			disp := display.New(display.DefaultConfig())

			job := httpjob.New(args[0], outputPath, connections, globalHTTPConfig)
			disp.RegisterJob(job.ID())
			hw.Submit(job)

			disp.Start(hw.Progress())
			err := hw.Run(ctx)
			disp.Stop()

			if err != nil {
				utils.PrintFatal("Download failed", err)
			}
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	return cmd
}
