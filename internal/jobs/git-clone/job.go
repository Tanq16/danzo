package gitclone

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/internal/utils"
)

type GitCloneJob struct {
	URL        string
	OutputPath string
	Depth      int
	Token      string
	SSHKey     string
}

type gitCloneJobState struct {
	URL        string `json:"url"`
	OutputPath string `json:"outputPath"`
	Depth      int    `json:"depth,omitempty"`
	Token      string `json:"token,omitempty"`
	SSHKey     string `json:"sshKey,omitempty"`
}

func New(url, outputPath string, depth int, token, sshKey string) *GitCloneJob {
	return &GitCloneJob{
		URL:        url,
		OutputPath: outputPath,
		Depth:      depth,
		Token:      token,
		SSHKey:     sshKey,
	}
}

func (j *GitCloneJob) ID() string {
	if j.OutputPath != "" {
		return j.OutputPath
	}
	return j.URL
}

func (j *GitCloneJob) Type() string { return "git-clone" }

func (j *GitCloneJob) Run(ctx context.Context, progress chan<- highway.Progress) error {
	provider, owner, repo, err := parseGitURL(j.URL)
	if err != nil {
		return err
	}

	cloneURL := fmt.Sprintf("https://%s/%s/%s", provider, owner, repo)

	if j.OutputPath == "" {
		j.OutputPath = repo
	}
	if info, err := os.Stat(j.OutputPath); err == nil && info.IsDir() {
		j.OutputPath = utils.RenewOutputPath(j.OutputPath)
	}
	outputDir := filepath.Dir(j.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}

	auth, _ := getAuthMethod(cloneURL, j.Token, j.SSHKey)

	progressWriter := &gitCloneProgressWriter{
		progressCh: progress,
		jobID:      j.ID(),
	}

	cloneOptions := &git.CloneOptions{
		URL:      cloneURL,
		Progress: progressWriter,
		Auth:     auth,
	}
	if j.Depth > 0 {
		cloneOptions.Depth = j.Depth
	}

	progress <- highway.Progress{
		JobID: j.ID(), Type: highway.ProgressTypeSubStatus,
		SubStatus: fmt.Sprintf("Cloning %s", cloneURL),
	}

	_, err = git.PlainClone(j.OutputPath, false, cloneOptions)
	if err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}

	size, sizeErr := getDirSize(j.OutputPath)
	msg := "Clone complete"
	if sizeErr == nil {
		msg = fmt.Sprintf("Clone complete — %s", utils.FormatBytes(uint64(size)))
	}

	progress <- highway.Progress{
		JobID: j.ID(), Type: highway.ProgressTypeSubStatus,
		SubStatus: msg, Done: true,
	}
	return nil
}

func (j *GitCloneJob) Marshal() ([]byte, error) {
	return json.Marshal(gitCloneJobState{
		URL:        j.URL,
		OutputPath: j.OutputPath,
		Depth:      j.Depth,
		Token:      j.Token,
		SSHKey:     j.SSHKey,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state gitCloneJobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URL, state.OutputPath, state.Depth, state.Token, state.SSHKey), nil
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
