package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/tanq16/danzo/utils"
)

func GetAuthToken() (string, error) {
	log := utils.GetLogger("gdrive-auth")
	credentialsFile := os.Getenv("GDRIVE_CREDENTIALS")
	if credentialsFile != "" {
		log.Debug().Str("credentials", credentialsFile).Msg("Using OAuth2 credentials")
		return getAccessTokenFromCredentials(credentialsFile)
	}
	apiKey := os.Getenv("GDRIVE_API_KEY")
	if apiKey == "" {
		return "", errors.New("neither GDRIVE_CREDENTIALS nor GDRIVE_API_KEY environment variables are set")
	}
	log.Debug().Msg("Using API key for Google Drive access")
	return apiKey, nil
}

func getAccessTokenFromCredentials(credentialsFile string) (string, error) {
	log := utils.GetLogger("gdrive-auth")
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return "", fmt.Errorf("unable to read credentials file: %v", err)
	}
	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/drive.readonly")
	if err != nil {
		return "", fmt.Errorf("unable to parse client secret file: %v", err)
	}

	tokenFile := ".danzo-token.json"
	token, err := getOAuthToken(config, tokenFile)
	if err != nil {
		return "", fmt.Errorf("unable to get OAuth token: %v", err)
	}
	if !token.Valid() {
		if token.RefreshToken != "" {
			log.Debug().Msg("Token expired, attempting to refresh")
			tokenSource := config.TokenSource(context.Background(), token)
			newToken, err := tokenSource.Token()
			if err != nil {
				return "", fmt.Errorf("unable to refresh token: %v", err)
			}
			token = newToken
			// Save refreshed token
			if err := saveToken(tokenFile, token); err != nil {
				log.Debug().Err(err).Msg("Failed to save refreshed token")
			}
		} else {
			return "", errors.New("OAuth token is expired and cannot be refreshed")
		}
	}
	log.Debug().Msg("Successfully obtained OAuth access token")
	return token.AccessToken, nil
}

func getOAuthToken(config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	log := utils.GetLogger("gdrive-auth")
	token, err := tokenFromFile(tokenFile)
	if err == nil {
		log.Debug().Str("file", tokenFile).Msg("Using existing token")
		return token, nil
	}
	log.Debug().Msg("No token found, need to authenticate with Google")
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\nVisit the URL to authenticate:\n%s\n\nEnter the authorization code here and press return\n", authURL)
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %v", err)
	}
	token, err = config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to exchange auth code for token: %v", err)
	}
	if err := saveToken(tokenFile, token); err != nil {
		log.Debug().Err(err).Msg("Failed to save token")
	}
	fmt.Printf("\033[%dA\033[J", 6)
	return token, nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

func saveToken(file string, token *oauth2.Token) error {
	log := utils.GetLogger("gdrive-auth")
	dir := filepath.Dir(file)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("unable to create token directory: %v", err)
		}
	}
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to cache oauth token: %v", err)
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(token)
	if err != nil {
		return fmt.Errorf("unable to encode token: %v", err)
	}
	log.Debug().Str("file", file).Msg("Token saved successfully")
	return nil
}
