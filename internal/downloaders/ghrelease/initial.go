package ghrelease

import (
	"fmt"
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

	// Fetch release info
	if manual {
		assets, tagName, err = askGitHubReleaseAssets(owner, repo, client)
	} else {
		assets, tagName, err = getGitHubReleaseAssets(owner, repo, client)
	}
	if err != nil {
		return fmt.Errorf("error fetching release info: %v", err)
	}

	// Select asset
	var downloadURL string
	var size int64
	var filename string

	if manual {
		downloadURL, size, err = promptGitHubAssetSelection(assets, tagName)
		if err != nil {
			return err
		}
	} else {
		downloadURL, size, err = selectGitHubLatestAsset(assets)
		if err != nil {
			return err
		}
		// Fall back to manual selection if auto-select fails
		if downloadURL == "" {
			downloadURL, size, err = promptGitHubAssetSelection(assets, tagName)
			if err != nil {
				return err
			}
		}
	}

	// Extract filename from URL
	urlParts := strings.Split(downloadURL, "/")
	filename = urlParts[len(urlParts)-1]

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
