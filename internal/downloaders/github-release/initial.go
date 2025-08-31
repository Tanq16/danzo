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

	// Store parsed values
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

	// Always get latest release first
	assets, tagName, err = getGitHubReleaseAssets(owner, repo, client)
	if err != nil {
		return fmt.Errorf("error fetching release info: %v", err)
	}

	// Try auto-select first
	downloadURL, size, err := selectGitHubLatestAsset(assets)
	if err != nil {
		return err
	}

	// If auto-select failed and manual not specified, fail
	if downloadURL == "" && !manual {
		return fmt.Errorf("could not automatically select asset for platform %s/%s, use --manual flag", runtime.GOOS, runtime.GOARCH)
	}

	// If manual mode or auto-select failed with manual flag
	if manual {
		job.PauseFunc()

		// Get user selection
		downloadURL, size, err = promptGitHubAssetSelection(assets, tagName)
		job.ResumeFunc()

		if err != nil {
			return err
		}
	}

	// Extract filename from URL
	urlParts := strings.Split(downloadURL, "/")
	filename := urlParts[len(urlParts)-1]

	// Set output path if not specified
	if job.OutputPath == "" {
		job.OutputPath = filename
	}

	// Store download info
	job.Metadata["downloadURL"] = downloadURL
	job.Metadata["fileSize"] = size
	job.Metadata["tagName"] = tagName

	return nil
}
