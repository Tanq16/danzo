package gitclone

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

func getAuthMethod(repoURL string) (transport.AuthMethod, error) {
	// Check for token first
	token := os.Getenv("GIT_TOKEN")
	if token != "" {
		if strings.Contains(repoURL, "github.com") {
			return &http.BasicAuth{
				Username: "oauth2",
				Password: token,
			}, nil
		} else if strings.Contains(repoURL, "gitlab.com") {
			return &http.BasicAuth{
				Username: "oauth2",
				Password: token,
			}, nil
		} else if strings.Contains(repoURL, "bitbucket.org") {
			return &http.BasicAuth{
				Username: "x-token-auth",
				Password: token,
			}, nil
		}
	}

	// Check for SSH key
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
