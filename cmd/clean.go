package cmd

import (
	"path/filepath"

	"github.com/rs/zerolog/log"
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
				log.Debug().Str("op", "cmd/clean").Msgf("Cleaning local files in current directory")
				utils.CleanLocal()
			} else {
				log.Debug().Str("op", "cmd/clean").Msgf("Cleaning local files in %s", filepath.Dir(args[0]))
				utils.CleanFunction(filepath.Dir(args[0]))
			}
		},
	}
}
