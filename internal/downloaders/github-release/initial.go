package ghrelease

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"
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
	log.Info().Str("op", "github-release/initial").Msgf("job validated for %s/%s", owner, repo)
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

	log.Debug().Str("op", "github-release/initial").Msgf("fetching release info for %s/%s", owner, repo)
	assets, tagName, err = getGitHubReleaseAssets(owner, repo, client)
	if err != nil {
		return fmt.Errorf("error fetching release info: %v", err)
	}
	// Try auto-select first; not working, prompt for manual or fail
	log.Debug().Str("op", "github-release/initial").Msgf("auto-selecting asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	downloadURL, size, err := selectGitHubLatestAsset(assets)
	if err != nil {
		return err
	}
	if downloadURL == "" && !manual {
		log.Error().Str("op", "github-release/initial").Msgf("could not automatically select asset for %s/%s, no --manual flag", runtime.GOOS, runtime.GOARCH)
		return fmt.Errorf("could not automatically select asset for platform %s/%s, use --manual flag", runtime.GOOS, runtime.GOARCH)
	}
	if manual {
		log.Debug().Str("op", "github-release/initial").Msgf("prompting for manual asset selection for %s/%s", runtime.GOOS, runtime.GOARCH)
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
	log.Info().Str("op", "github-release/initial").Msgf("job built for %s/%s", owner, repo)
	return nil
}
