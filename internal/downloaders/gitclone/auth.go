package gitclone

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

func getAuthMethod(repoURL string, metadata map[string]any) (transport.AuthMethod, error) {
	tokenStr, ok := metadata["token"]
	token := ""
	if ok {
		token = tokenStr.(string)
	}
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

	sshKeyPath := ""
	sshKeyStr, ok := metadata["sshKey"]
	if ok {
		sshKeyPath = sshKeyStr.(string)
	}
	if sshKeyPath != "" {
		publicKeys, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
		if err != nil {
			return nil, fmt.Errorf("couldn't load SSH key: %v", err)
		}
		return publicKeys, nil
	}

	return nil, errors.New("no authentication method found")
}
