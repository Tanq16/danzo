package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	gitclonejob "github.com/tanq16/danzo/internal/jobs/git-clone"
	"github.com/tanq16/danzo/utils"
)

var gitCloneFlags struct {
	outputPath string
	depth      int
	token      string
	sshKey     string
}

var gitCloneCmd = &cobra.Command{
	Use:     "git-clone [REPO_URL] [--output OUTPUT_PATH] [--depth DEPTH] [--token GIT_TOKEN] [--ssh SSH_KEY_PATH]",
	Short:   "Clone a Git repository",
	Aliases: []string{"gitclone", "gitc", "git", "clone"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		hw := newHighway()

		disp := display.New(display.DefaultConfig())

		job := gitclonejob.New(args[0], gitCloneFlags.outputPath, gitCloneFlags.depth, gitCloneFlags.token, gitCloneFlags.sshKey)
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

func newGitCloneCmd() *cobra.Command {
	return gitCloneCmd
}

func init() {
	gitCloneCmd.Flags().StringVarP(&gitCloneFlags.outputPath, "output", "o", "", "Output directory path")
	gitCloneCmd.Flags().IntVar(&gitCloneFlags.depth, "depth", 0, "Clone depth (0 for full history)")
	gitCloneCmd.Flags().StringVar(&gitCloneFlags.token, "token", "", "Git token for authentication")
	gitCloneCmd.Flags().StringVar(&gitCloneFlags.sshKey, "ssh", "", "SSH key path for authentication")
}
