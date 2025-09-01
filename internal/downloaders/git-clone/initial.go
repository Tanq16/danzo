package gitclone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

type GitCloneDownloader struct{}

func (d *GitCloneDownloader) ValidateJob(job *utils.DanzoJob) error {
	provider, owner, repo, err := parseGitURL(job.URL)
	if err != nil {
		return err
	}
	job.Metadata["provider"] = provider
	job.Metadata["owner"] = owner
	job.Metadata["repo"] = repo
	log.Info().Str("op", "git-clone/initial").Msgf("job validated for %s/%s/%s", provider, owner, repo)
	return nil
}

func (d *GitCloneDownloader) BuildJob(job *utils.DanzoJob) error {
	provider := job.Metadata["provider"].(string)
	owner := job.Metadata["owner"].(string)
	repo := job.Metadata["repo"].(string)
	cloneURL := fmt.Sprintf("https://%s/%s/%s", provider, owner, repo)
	job.Metadata["cloneURL"] = cloneURL
	if job.OutputPath == "" {
		job.OutputPath = repo
	}
	if info, err := os.Stat(job.OutputPath); err == nil && info.IsDir() {
		job.OutputPath = utils.RenewOutputPath(job.OutputPath)
	}
	outputDir := filepath.Dir(job.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}
	log.Info().Str("op", "git-clone/initial").Msgf("job built for %s/%s/%s", provider, owner, repo)
	return nil
}

func parseGitURL(url string) (string, string, string, error) {
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	parts := strings.Split(url, "/")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid git URL format, expected provider/owner/repo")
	}
	provider := parts[0]
	owner := parts[1]
	repo := parts[2]
	switch provider {
	case "github.com", "gitlab.com", "bitbucket.org":
	default:
		return "", "", "", fmt.Errorf("unsupported git provider: %s", provider)
	}
	return provider, owner, repo, nil
}
