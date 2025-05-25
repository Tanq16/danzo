package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/utils"
)

func newCleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean [path]",
		Short: "Clean up temporary files",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				utils.Clean(filepath.Dir("."))
			} else {
				utils.Clean(filepath.Dir(args[0]))
			}
		},
	}
}
