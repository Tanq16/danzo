package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	// "github.com/tanq16/danzo/internal"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	// OAuth 2.0 credentials for device flow
	clientID     = "your-client-id.apps.googleusercontent.com" // Replace with actual client ID
	clientSecret = "your-client-secret"                        // Replace with actual client secret
)

var (
	// OAuth 2.0 config
	config = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{drive.DriveReadonlyScope},
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
	}
)

// Auth handles Google Drive authentication
type Auth struct {
	log         zerolog.Logger
	tokenFile   string
	oauthConfig *oauth2.Config
}

// NewAuth creates a new Auth instance
func NewAuth() *Auth {
	log := zerolog.New(os.Stderr).With().Timestamp().Logger().Level(zerolog.InfoLevel).With().Str("module", "gdrive-auth").Logger()

	// Get user's home directory for storing token
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user home directory")
		homeDir = "."
	}

	// Create .danzo directory if it doesn't exist
	danzoDir := filepath.Join(homeDir, ".danzo")
	if err := os.MkdirAll(danzoDir, 0700); err != nil {
		log.Error().Err(err).Msg("Failed to create .danzo directory")
	}

	tokenFile := filepath.Join(danzoDir, "gdrive_token.json")

	return &Auth{
		log:         log,
		tokenFile:   tokenFile,
		oauthConfig: config,
	}
}

// getTokenFromFile loads the token from the local file
func (a *Auth) getTokenFromFile() (*oauth2.Token, error) {
	a.log.Debug().Str("tokenFile", a.tokenFile).Msg("Attempting to load token from file")

	f, err := os.Open(a.tokenFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

// saveToken saves the token to the local file
func (a *Auth) saveToken(token *oauth2.Token) error {
	a.log.Debug().Str("tokenFile", a.tokenFile).Msg("Saving token to file")

	f, err := os.OpenFile(a.tokenFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

// getTokenFromDeviceFlow gets a token using the device flow
func (a *Auth) getTokenFromDeviceFlow(ctx context.Context) (*oauth2.Token, error) {
	a.log.Info().Msg("Starting device authentication flow")

	// Get a device code
	// deviceConfig := &oauth2.Config{
	// 	Config:     a.oauthConfig,
	// 	HTTPClient: http.DefaultClient,
	// }
	deviceCode, err := a.oauthConfig.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device code: %v", err)
	}

	// Instructions for the user
	fmt.Printf("\n=== Google Drive Authentication Required ===\n")
	fmt.Printf("1. Visit: %s\n", deviceCode.VerificationURI)
	fmt.Printf("2. Enter code: %s\n", deviceCode.UserCode)
	fmt.Printf("3. Waiting for authentication...")

	// Wait for user to authorize
	token, err := a.oauthConfig.DeviceAccessToken(ctx, deviceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %v", err)
	}

	fmt.Println("Authentication successful!")
	return token, nil
}

// GetClient returns an HTTP client with valid OAuth credentials
func (a *Auth) GetClient(ctx context.Context) (*http.Client, error) {
	// Try to load token from file
	token, err := a.getTokenFromFile()
	if err != nil {
		a.log.Info().Msg("No stored token found, starting device flow")
		// No token found, get one via device flow
		token, err = a.getTokenFromDeviceFlow(ctx)
		if err != nil {
			return nil, err
		}
		// Save the token
		if err := a.saveToken(token); err != nil {
			a.log.Error().Err(err).Msg("Failed to save token")
		}
	}

	// Check if token is expired and refresh if needed
	if token.Expiry.Before(time.Now()) {
		a.log.Info().Msg("Token expired, refreshing")
		tokenSource := a.oauthConfig.TokenSource(ctx, token)
		newToken, err := tokenSource.Token()
		if err != nil {
			a.log.Error().Err(err).Msg("Failed to refresh token")
			// Try device flow again
			newToken, err = a.getTokenFromDeviceFlow(ctx)
			if err != nil {
				return nil, err
			}
		}

		// Save the new token
		if err := a.saveToken(newToken); err != nil {
			a.log.Error().Err(err).Msg("Failed to save refreshed token")
		}
		token = newToken
	}

	return a.oauthConfig.Client(ctx, token), nil
}

// GetDriveService returns a Drive service client
func (a *Auth) GetDriveService(ctx context.Context) (*drive.Service, error) {
	client, err := a.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create drive service: %v", err)
	}

	return srv, nil
}

// CheckAuth verifies if authentication is working
func (a *Auth) CheckAuth(ctx context.Context) error {
	a.log.Debug().Msg("Checking authentication")

	srv, err := a.GetDriveService(ctx)
	if err != nil {
		return err
	}

	// Try to list files (just 1 to verify auth works)
	_, err = srv.Files.List().PageSize(1).Do()
	if err != nil {
		return errors.New("authentication failed: " + err.Error())
	}

	a.log.Debug().Msg("Authentication check successful")
	return nil
}
