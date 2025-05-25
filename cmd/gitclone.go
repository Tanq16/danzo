package cmd

import (
	"github.com/spf13/cobra"
)

func newGitCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gitclone",
		Short: "Clone a Git repository",
	}
}
