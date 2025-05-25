package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

func newGitCloneCmd() *cobra.Command {
	var outputPath string
	var depth int

	cmd := &cobra.Command{
		Use:   "gitclone [REPO_URL]",
		Short: "Clone a Git repository",
		Long: `Clone a Git repository from GitHub, GitLab, or Bitbucket.

Supported formats:
  - github.com/owner/repo
  - gitlab.com/owner/repo
  - bitbucket.org/owner/repo

Authentication:
  - Set GIT_TOKEN environment variable for token-based auth
  - Set GIT_SSH environment variable for SSH key path`,
		Args: cobra.ExactArgs(1),
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

			jobs := []utils.DanzoJob{job}
			scheduler.Run(jobs, workers, fileLog)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output directory path")
	cmd.Flags().IntVarP(&depth, "depth", "d", 0, "Clone depth (0 for full history)")

	return cmd
}
