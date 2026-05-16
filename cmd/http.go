package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	httpjob "github.com/tanq16/danzo/internal/jobs/http"
	"github.com/tanq16/danzo/utils"
)

var httpFlags struct {
	outputPath string
}

var httpCmd = &cobra.Command{
	Use:   "http [URL] [--output OUTPUT_PATH]",
	Short: "Download file via HTTP/HTTPS",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		hw := newHighway()

		disp := display.New(display.DefaultConfig())

		job := httpjob.New(args[0], httpFlags.outputPath, connections, globalHTTPConfig)
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

func newHTTPCmd() *cobra.Command {
	return httpCmd
}

func init() {
	httpCmd.Flags().StringVarP(&httpFlags.outputPath, "output", "o", "", "Output file path")
}
