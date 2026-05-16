package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/utils"
)

func newCleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean [path]",
		Short: "Clean up temporary and state files",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			os.Remove(resumeStatePath)
			if len(args) == 0 {
				if err := utils.CleanLocal(); err != nil {
					utils.PrintError("Failed to clean local files", err)
				} else {
					utils.PrintSuccess("Cleaned temporary files")
				}
			} else {
				if err := utils.CleanFunction(args[0]); err != nil {
					utils.PrintError("Failed to clean files", err)
				} else {
					utils.PrintSuccess("Cleaned temporary files for " + args[0])
				}
			}
		},
	}
}
