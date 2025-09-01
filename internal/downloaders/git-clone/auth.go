package gitclone

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/rs/zerolog/log"
)

func getAuthMethod(repoURL string, metadata map[string]any) (transport.AuthMethod, error) {
	tokenStr, ok := metadata["token"]
	token := ""
	if ok {
		token = tokenStr.(string)
		log.Debug().Str("op", "git-clone/auth").Msg("token found")
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
		log.Debug().Str("op", "git-clone/auth").Msg("sshKey found")
		sshKeyPath = sshKeyStr.(string)
	}
	if sshKeyPath != "" {
		publicKeys, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
		if err != nil {
			return nil, fmt.Errorf("couldn't load SSH key: %v", err)
		}
		return publicKeys, nil
	}
	log.Debug().Str("op", "git-clone/auth").Msg("no authentication method found")
	return nil, errors.New("no authentication method found")
}
