package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tanq16/danzo/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func GetAuthToken() (string, error) {
	credentialsFile := os.Getenv("GDRIVE_CREDENTIALS")
	if credentialsFile != "" {
		return getAccessTokenFromCredentials(credentialsFile)
	}
	apiKey := os.Getenv("GDRIVE_API_KEY")
	if apiKey == "" {
		return "", errors.New("neither GDRIVE_CREDENTIALS nor GDRIVE_API_KEY environment variables are set")
	}
	return apiKey, nil
}

func getAccessTokenFromCredentials(credentialsFile string) (string, error) {
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
			tokenSource := config.TokenSource(context.Background(), token)
			newToken, err := tokenSource.Token()
			if err != nil {
				return "", fmt.Errorf("unable to refresh token: %v", err)
			}
			token = newToken
			// Save refreshed token
			if err := saveToken(tokenFile, token); err != nil {
			}
		} else {
			return "", errors.New("OAuth token is expired and cannot be refreshed")
		}
	}
	return token.AccessToken, nil
}

func getOAuthToken(config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	token, err := tokenFromFile(tokenFile)
	if err == nil {
		return token, nil
	}
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	utils.PrintDetail("\nVisit this URL to get the authorization code:\n")
	fmt.Printf("%s\n", authURL)
	utils.PrintDetail("\nAfter authorizing, enter the authorization code:")
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %v", err)
	}
	token, err = config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to exchange auth code for token: %v", err)
	}
	if err := saveToken(tokenFile, token); err != nil {
	}
	clearLength := 6
	clearLength += len(authURL)/utils.GetTerminalWidth() + 1
	fmt.Printf("\033[%dA\033[J", clearLength)
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
	return nil
}
