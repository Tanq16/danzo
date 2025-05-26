package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newGitCloneCmd() *cobra.Command {
	var outputPath string
	var depth int
	var token string
	var sshKey string

	cmd := &cobra.Command{
		Use:   "gitclone [REPO_URL] [--output OUTPUT_PATH] [--depth DEPTH] [--token GIT_TOKEN] [--ssh SSH_KEY_PATH]",
		Short: "Clone a Git repository",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			job := utils.DanzoJob{
				JobType:          "gitclone",
				URL:              args[0],
				OutputPath:       outputPath,
				ProgressType:     "stream",
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			if depth > 0 {
				job.Metadata["depth"] = depth
			}
			if token != "" {
				job.Metadata["token"] = token
			}
			if sshKey != "" {
				job.Metadata["sshKey"] = sshKey
			}
			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers, fileLog)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output directory path")
	cmd.Flags().IntVarP(&depth, "depth", "d", 0, "Clone depth (0 for full history)")
	cmd.Flags().StringVar(&token, "token", "", "Git token for authentication")
	cmd.Flags().StringVar(&sshKey, "ssh", "", "SSH key path for authentication")
	return cmd
}
