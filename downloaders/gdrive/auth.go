// package main

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"net/http"
// 	"net/url"
// 	"os"
// 	"strings"
// 	"time"

// 	"golang.org/x/oauth2"
// 	"golang.org/x/oauth2/google"
// 	"google.golang.org/api/drive/v3"
// 	"google.golang.org/api/option"
// )

// // DeviceCodeResponse represents the response from Google's device code API
// type DeviceCodeResponse struct {
// 	DeviceCode              string `json:"device_code"`
// 	UserCode                string `json:"user_code"`
// 	VerificationURL         string `json:"verification_url"`
// 	ExpiresIn               int    `json:"expires_in"`
// 	Interval                int    `json:"interval"`
// 	VerificationURIComplete string `json:"verification_uri_complete"`
// }

// // TokenResponse represents the OAuth token response
// type TokenResponse struct {
// 	AccessToken  string `json:"access_token"`
// 	RefreshToken string `json:"refresh_token"`
// 	ExpiresIn    int    `json:"expires_in"`
// 	TokenType    string `json:"token_type"`
// 	IDToken      string `json:"id_token,omitempty"`
// 	Scope        string `json:"scope,omitempty"`
// }

// func main() {
// 	ctx := context.Background()

// 	// Client ID from your Google Cloud Console
// 	// This ID needs to be configured for the device auth flow
// 	clientID := "114168928839-1b0mndmhfip9oscs7u820ga1u6g1kkht.apps.googleusercontent.com"

// 	// Request a device code
// 	deviceCode, err := getDeviceCode(ctx, clientID, []string{drive.DriveFileScope})
// 	if err != nil {
// 		fmt.Printf("Error getting device code: %v\n", err)
// 		os.Exit(1)
// 	}

// 	// Show user instructions
// 	fmt.Printf("\n=== Google Drive Authentication Required ===\n")
// 	fmt.Printf("1. Go to: %s\n", deviceCode.VerificationURL)
// 	fmt.Printf("2. Enter code: %s\n", deviceCode.UserCode)
// 	fmt.Printf("3. Waiting for you to complete the authorization...\n\n")
// 	fmt.Scanf("Press Enter to continue...")

// 	// Poll for token
// 	token, err := pollForToken(ctx, clientID, deviceCode)
// 	if err != nil {
// 		fmt.Printf("Error getting token: %v\n", err)
// 		os.Exit(1)
// 	}

// 	fmt.Println("Authentication successful!")

// 	// Create OAuth2 configuration for token setup
// 	config := &oauth2.Config{
// 		ClientID: clientID,
// 		Endpoint: google.Endpoint,
// 		Scopes:   []string{drive.DriveMetadataReadonlyScope},
// 	}

// 	// Create HTTP client with token
// 	client := config.Client(ctx, token)

// 	// Create Drive service
// 	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
// 	if err != nil {
// 		fmt.Printf("Unable to create Drive service: %v\n", err)
// 		os.Exit(1)
// 	}

// 	// List files to verify authentication
// 	files, err := srv.Files.List().PageSize(10).Fields("nextPageToken, files(id, name)").Do()
// 	if err != nil {
// 		fmt.Printf("Unable to list files: %v\n", err)
// 		os.Exit(1)
// 	}

// 	// Print files
// 	if len(files.Files) == 0 {
// 		fmt.Println("No files found.")
// 	} else {
// 		fmt.Println("Files:")
// 		for _, file := range files.Files {
// 			fmt.Printf("%s (%s)\n", file.Name, file.Id)
// 		}
// 	}
// }

// // getDeviceCode requests a device code from Google's OAuth server
// func getDeviceCode(ctx context.Context, clientID string, scopes []string) (*DeviceCodeResponse, error) {
// 	client := &http.Client{Timeout: 30 * time.Second}

// 	data := url.Values{}
// 	data.Set("client_id", clientID)
// 	data.Set("scope", strings.Join(scopes, " "))

// 	req, err := http.NewRequestWithContext(
// 		ctx,
// 		"POST",
// 		"https://oauth2.googleapis.com/device/code",
// 		strings.NewReader(data.Encode()),
// 	)
// 	if err != nil {
// 		return nil, err
// 	}

// 	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		body, _ := io.ReadAll(resp.Body)
// 		return nil, fmt.Errorf("failed to get device code, status: %d, response: %s", resp.StatusCode, string(body))
// 	}

// 	var deviceCode DeviceCodeResponse
// 	err = json.NewDecoder(resp.Body).Decode(&deviceCode)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &deviceCode, nil
// }

// // pollForToken polls Google's OAuth server for a token
// func pollForToken(ctx context.Context, clientID string, deviceCode *DeviceCodeResponse) (*oauth2.Token, error) {
// 	client := &http.Client{Timeout: 30 * time.Second}

// 	// Calculate deadline based on the expires_in value
// 	// deadline := time.Now().Add(time.Duration(deviceCode.ExpiresIn) * time.Second)

// 	// Wait at least the specified interval between requests
// 	// interval := time.Duration(deviceCode.Interval) * time.Second
// 	// if interval < 5*time.Second {
// 	// 	interval = 5 * time.Second // Min 5 seconds to be safe
// 	// }

// 	data := url.Values{}
// 	data.Set("client_id", clientID)
// 	data.Set("device_code", deviceCode.DeviceCode)
// 	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

// 	req, err := http.NewRequestWithContext(
// 		ctx,
// 		"POST",
// 		"https://oauth2.googleapis.com/token",
// 		strings.NewReader(data.Encode()),
// 	)
// 	if err != nil {
// 		return nil, err
// 	}

// 	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}

// 	body, err := io.ReadAll(resp.Body)
// 	resp.Body.Close()
// 	if err != nil {
// 		return nil, err
// 	}

// 	token := &oauth2.Token{}
// 	if resp.StatusCode == http.StatusOK {
// 		var tokenResp TokenResponse
// 		err = json.Unmarshal(body, &tokenResp)
// 		if err != nil {
// 			return nil, err
// 		}

// 		token = &oauth2.Token{
// 			AccessToken:  tokenResp.AccessToken,
// 			RefreshToken: tokenResp.RefreshToken,
// 			TokenType:    tokenResp.TokenType,
// 			Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
// 		}

// 		return token, nil
// 	}
// 	return nil, fmt.Errorf("failed to get token, status: %d, response: %s", resp.StatusCode, string(body))

// 	// Check for authorization_pending error (which is expected while waiting)
// 	// var errResp map[string]interface{}
// 	// if err := json.Unmarshal(body, &errResp); err == nil {
// 	// if errCode, ok := errResp["error"].(string); ok {
// 	// if errCode == "authorization_pending" {
// 	// 	// This is normal while waiting for the user to authorize
// 	// 	fmt.Print(".") // Progress indicator
// 	// 	continue
// 	// } else if errCode == "slow_down" {
// 	// 	// Google is telling us to slow down our polling
// 	// 	ticker.Reset(interval + 5*time.Second)
// 	// 	continue
// 	// }
// 	// }
// 	// }

// 	// Only handle unexpected errors
// 	// if resp.StatusCode != http.StatusBadRequest {
// 	// return nil, fmt.Errorf("unexpected response: %s", string(body))
// 	// }
// 	// ticker := time.NewTicker(interval)
// 	// defer ticker.Stop()

// 	// for {
// 	// 	select {
// 	// 	case <-ctx.Done():
// 	// 		return nil, ctx.Err()
// 	// 	case <-ticker.C:
// 	// 		if time.Now().After(deadline) {
// 	// 			return nil, fmt.Errorf("authorization timed out")
// 	// 		}

// 	// 	}
// 	// }
// }
