package cmd

import (
	"github.com/spf13/cobra"
)

func newGHCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ghclone",
		Short: "Clone a GitHub repository",
	}
}

func newGHReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ghrelease",
		Short: "Download a GitHub release",
	}
}
