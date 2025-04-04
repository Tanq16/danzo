package gitclone

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"slices"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/tanq16/danzo/utils"
)

type gitCloneProgress struct {
	outputPath string
	streamCh   chan<- []string
	buffer     []string
}

func (p *gitCloneProgress) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	if message != "" {
		p.buffer = append(p.buffer, message)
		if len(p.buffer) > 5 {
			p.buffer = p.buffer[len(p.buffer)-5:]
		}
		p.streamCh <- slices.Clone(p.buffer)
	}
	return len(data), nil
}

func InitGitClone(gitURL string, outputPath string) (string, int, error) {
	log := utils.GetLogger("gitclone-check")
	parts := strings.Split(gitURL, "||")
	depth := 0
	if len(parts) > 1 {
		gitURL = parts[0]
		depth64, _ := strconv.ParseInt(parts[1], 10, 64)
		depth = int(depth64)
	}
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Debug().Err(err).Str("outputDir", outputDir).Msg("Failed to create output directory")
		return "", depth, fmt.Errorf("error creating output directory: %v", err)
	}
	return gitURL, depth, nil
}

func CloneRepository(gitURL, outputPath string, progressCh chan<- int64, streamCh chan<- []string, depth int) error {
	log := utils.GetLogger("gitclone")
	if strings.HasPrefix(gitURL, "git.com") {
		gitURL = strings.ReplaceAll(gitURL, "git.com/", "")
	}
	actualURL := fmt.Sprintf("https://%s", strings.ReplaceAll(gitURL, ".git", ""))
	progress := &gitCloneProgress{
		outputPath: outputPath,
		streamCh:   streamCh,
		buffer:     []string{},
	}

	progress.buffer = append(progress.buffer, fmt.Sprintf("Cloning %s to %s...", actualURL, outputPath))
	streamCh <- slices.Clone(progress.buffer)

	auth, err := getAuthMethod(actualURL)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to set up authentication, will try anonymous clone")
	}
	cloneOptions := &git.CloneOptions{
		URL:      actualURL,
		Progress: progress,
		Auth:     auth,
	}
	if depth > 0 {
		cloneOptions.Depth = depth
	}

	log.Debug().Str("url", actualURL).Str("output", outputPath).Msg("Starting Git clone")
	_, err = git.PlainClone(outputPath, false, cloneOptions)
	if err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}
	log.Debug().Str("url", actualURL).Str("output", outputPath).Msg("Git clone completed")

	size, err := getDirSize(outputPath)
	if err == nil {
		progressCh <- size
	} else {
		log.Debug().Err(err).Msg("Failed to get directory size")
		progressCh <- 0
	}
	streamCh <- []string{"Clone complete"}
	log.Debug().Str("output", outputPath).Msg("Git clone completed successfully")
	return nil
}

func getAuthMethod(repoURL string) (transport.AuthMethod, error) {
	token := os.Getenv("GIT_TOKEN")
	if token != "" {
		if strings.HasPrefix(repoURL, "https://github.com") {
			return &http.BasicAuth{
				Username: "oauth2", // username doesn't matter when using token for GitHub
				Password: token,
			}, nil
		} else if strings.HasPrefix(repoURL, "https://gitlab.com") {
			return &http.BasicAuth{
				Username: "oauth2",
				Password: token,
			}, nil
		} else if strings.HasPrefix(repoURL, "https://bitbucket.org") {
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
	// Use "du" if available (faster option)
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		cmd := exec.Command("du", "-s", "-b", path)
		output, err := cmd.CombinedOutput()
		if err == nil {
			parts := strings.Split(string(output), "\t")
			if len(parts) > 0 {
				size, err := strconv.ParseInt(parts[0], 10, 64)
				if err == nil {
					return size, nil
				}
			}
		}
	}
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
