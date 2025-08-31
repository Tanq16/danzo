package ghrelease

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/tanq16/danzo/internal/utils"
)

type GitReleaseDownloader struct{}

func (d *GitReleaseDownloader) ValidateJob(job *utils.DanzoJob) error {
	owner, repo, err := parseGitHubURL(job.URL)
	if err != nil {
		return err
	}
	job.Metadata["owner"] = owner
	job.Metadata["repo"] = repo
	return nil
}

func (d *GitReleaseDownloader) BuildJob(job *utils.DanzoJob) error {
	owner := job.Metadata["owner"].(string)
	repo := job.Metadata["repo"].(string)
	manual := job.Metadata["manual"].(bool)
	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	var assets []map[string]any
	var tagName string
	var err error

	assets, tagName, err = getGitHubReleaseAssets(owner, repo, client)
	if err != nil {
		return fmt.Errorf("error fetching release info: %v", err)
	}
	// Try auto-select first; not working, prompt for manual or fail
	downloadURL, size, err := selectGitHubLatestAsset(assets)
	if err != nil {
		return err
	}
	if downloadURL == "" && !manual {
		return fmt.Errorf("could not automatically select asset for platform %s/%s, use --manual flag", runtime.GOOS, runtime.GOARCH)
	}
	if manual {
		job.PauseFunc()
		downloadURL, size, err = promptGitHubAssetSelection(assets, tagName)
		job.ResumeFunc()
		if err != nil {
			return err
		}
	}

	urlParts := strings.Split(downloadURL, "/")
	filename := urlParts[len(urlParts)-1]
	if job.OutputPath == "" {
		job.OutputPath = filename
	}
	job.Metadata["downloadURL"] = downloadURL
	job.Metadata["fileSize"] = size
	job.Metadata["tagName"] = tagName
	return nil
}
