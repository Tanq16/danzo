package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/utils"
)

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume interrupted downloads",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			hw := newHighway()

			if err := hw.LoadState(); err != nil {
				utils.PrintFatal("Failed to load resume state", err)
			}

			disp := display.New(display.DefaultConfig())
			for _, id := range hw.PendingJobIDs() {
				disp.RegisterJob(id)
			}

			disp.Start(hw.Progress())
			err := hw.Run(ctx)
			disp.Stop()

			if err != nil {
				utils.PrintFatal("Resume failed", err)
			}
		},
	}
}
