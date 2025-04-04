package youtube

import (
	"fmt"
)

func addMusicMetadata(outputPath, musicClient, musicId string) error {
	if musicClient == "spotify" {
		err := addSpotifyMetadata(outputPath, musicId)
		if err != nil {
			return fmt.Errorf("failed to add Spotify metadata: %v", err)
		}
	} else if musicClient == "apple" {
		err := addAppleMetadata(outputPath, musicId)
		if err != nil {
			return fmt.Errorf("failed to add Apple metadata: %v", err)
		}
	}
	return nil
}

func addSpotifyMetadata(outputPath, musicId string) error {
	// httpClient := utils.CreateHTTPClient()
	return nil
}

func addAppleMetadata(outputPath, musicId string) error {
	// httpClient := utils.CreateHTTPClient()
	return nil
}
