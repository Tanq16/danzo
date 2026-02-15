package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	gitclonejob "github.com/tanq16/danzo/internal/jobs/git-clone"
	"github.com/tanq16/danzo/internal/utils"
)

func newGitCloneCmd() *cobra.Command {
	var outputPath string
	var depth int
	var token string
	var sshKey string

	cmd := &cobra.Command{
		Use:     "git-clone [REPO_URL] [--output OUTPUT_PATH] [--depth DEPTH] [--token GIT_TOKEN] [--ssh SSH_KEY_PATH]",
		Short:   "Clone a Git repository",
		Aliases: []string{"gitclone", "gitc", "git", "clone"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			hw := highway.New(workers, ".danzo-resume-state.json")
			hw.RegisterType("git-clone", gitclonejob.Unmarshal)

			disp := display.New(display.DefaultConfig())

			job := gitclonejob.New(args[0], outputPath, depth, token, sshKey)
			disp.RegisterJob(job.ID())
			hw.Submit(job)

			disp.Start(hw.Progress())
			err := hw.Run(ctx)
			disp.Stop()

			if err != nil {
				utils.PrintFatal("Clone failed", err)
			}
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output directory path")
	cmd.Flags().IntVar(&depth, "depth", 0, "Clone depth (0 for full history)")
	cmd.Flags().StringVar(&token, "token", "", "Git token for authentication")
	cmd.Flags().StringVar(&sshKey, "ssh", "", "SSH key path for authentication")
	return cmd
}
