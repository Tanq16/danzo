package gitclone

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/tanq16/danzo/utils"
)

type gitCloneProgress struct {
	outputPath string
	streamCh   chan<- []string
	progressCh chan<- int64
	buffer     []string
	downloaded int64
}

func (p *gitCloneProgress) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	if message != "" {
		p.buffer = append(p.buffer, message)
		if len(p.buffer) > 5 {
			p.buffer = p.buffer[len(p.buffer)-5:]
		}
		p.streamCh <- p.buffer
		p.downloaded += int64(len(data))
	}
	return len(data), nil
}

func InitGitClone(gitURL string, outputPath string) (string, error) {
	log := utils.GetLogger("gitclone-check")
	serverType := "genericgit"
	if !strings.HasPrefix(gitURL, "github.com") {
		serverType = "github"
	} else if !strings.HasPrefix(gitURL, "gitlab.com") {
		serverType = "gitlab"
	} else if !strings.HasPrefix(gitURL, "bitbucket.org") {
		serverType = "bitbucket"
	}
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Debug().Err(err).Str("outputDir", outputDir).Msg("Failed to create output directory")
		return "", fmt.Errorf("error creating output directory: %v", err)
	}
	return serverType, nil
}

func CloneRepository(gitURL string, outputPath string, progressCh chan<- int64) error {
	log := utils.GetLogger("gitclone")
	streamCh := make(chan []string, 5)
	defer close(streamCh)
	actualURL := fmt.Sprintf("https://%s", strings.ReplaceAll(gitURL, ".git", ""))

	progress := &gitCloneProgress{
		outputPath: outputPath,
		streamCh:   streamCh,
		progressCh: progressCh,
		buffer:     []string{},
	}
	auth, err := getAuthMethod(actualURL)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to set up authentication, will try anonymous clone")
	}
	cloneOptions := &git.CloneOptions{
		URL:      actualURL,
		Progress: progress,
		Auth:     auth,
	}

	progress.buffer = append(progress.buffer, fmt.Sprintf("Cloning %s to %s...", actualURL, outputPath))
	streamCh <- progress.buffer
	log.Debug().Str("url", actualURL).Str("output", outputPath).Msg("Starting Git clone")

	// Perform the clone
	_, err = git.PlainClone(outputPath, false, cloneOptions)
	if err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}

	if info, err := getDirSize(outputPath); err == nil {
		progress.buffer = append(progress.buffer, fmt.Sprintf("Clone complete. Repository size: %s", utils.FormatBytes(uint64(info))))
		streamCh <- progress.buffer
		progressCh <- info // Send final size to progress channel
	} else {
		progress.buffer = append(progress.buffer, "Clone complete")
		streamCh <- progress.buffer
		progressCh <- progress.downloaded // Send estimated size if we can't get real size
	}

	log.Debug().Str("output", outputPath).Msg("Git clone completed successfully")
	return nil
}

func getAuthMethod(repoURL string) (transport.AuthMethod, error) {
	token := os.Getenv("GIT_TOKEN")
	if token != "" {
		if strings.HasPrefix(repoURL, "github.com") {
			return &http.BasicAuth{
				Username: "oauth2", // username doesn't matter when using token for GitHub
				Password: token,
			}, nil
		} else if strings.HasPrefix(repoURL, "gitlab.com") {
			return &http.BasicAuth{
				Username: "oauth2",
				Password: token,
			}, nil
		} else if strings.HasPrefix(repoURL, "bitbucket.org") {
			return &http.BasicAuth{
				Username: "x-token-auth",
				Password: token,
			}, nil
		} else {
			// Generic auth
			return &http.BasicAuth{
				Username: "git",
				Password: token,
			}, nil
		}
	}
	sshKeyPath := os.Getenv("GIT_SSH")
	if sshKeyPath != "" {
		publicKeys, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
		if err != nil {
			return nil, fmt.Errorf("couldn't load SSH key: %v", err)
		}
		return publicKeys, nil
	}
	return nil, errors.New("no authentication method found")
}

func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
