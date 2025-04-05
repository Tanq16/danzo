package youtube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tanq16/danzo/utils"
)

type DeezerResponse struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Artist struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
		Cover string `json:"cover_xl"`
	} `json:"album"`
	ReleaseDate string `json:"release_date"`
	TrackNumber int    `json:"track_position"`
	DiskNumber  int    `json:"disk_number"`
	Genre       []struct {
		Name string `json:"name"`
	} `json:"genres,omitempty"`
	Contributors []struct {
		Name string `json:"name"`
		Role string `json:"role"`
	} `json:"contributors"`
}

func addMusicMetadata(outputPath, musicClient, musicId string) error {
	switch musicClient {
	case "deezer":
		return addDeezerMetadata(outputPath, musicId)
	case "spotify":
		return addSpotifyMetadata(outputPath, musicId)
	case "apple":
		return addAppleMetadata(outputPath, musicId)
	default:
		return fmt.Errorf("unsupported music client: %s", musicClient)
	}
}

func addDeezerMetadata(outputPath, musicId string) error {
	log := utils.GetLogger("deezer")
	log.Debug().Str("musicId", musicId).Msg("Fetching Deezer metadata")
	client := utils.CreateHTTPClient(30*time.Second, 30*time.Second, "", false)

	apiURL := fmt.Sprintf("https://api.deezer.com/track/%s", musicId)
	resp, err := client.Get(apiURL)
	if err != nil {
		return fmt.Errorf("error fetching metadata: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status code %d", resp.StatusCode)
	}
	var deezerResp DeezerResponse
	if err := json.NewDecoder(resp.Body).Decode(&deezerResp); err != nil {
		return fmt.Errorf("error parsing response: %v", err)
	}

	tempDir := filepath.Join(filepath.Dir(outputPath), ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fileMarker := uuid.New().String()
	var artworkPath string
	if deezerResp.Album.Cover != "" {
		artworkPath = filepath.Join(tempDir, fileMarker+".jpg")
		err := downloadFile(deezerResp.Album.Cover, artworkPath, client)
		if err != nil {
			log.Debug().Err(err).Msg("Error downloading artwork")
			artworkPath = ""
		} else {
			log.Debug().Str("path", artworkPath).Msg("Downloaded artwork")
		}
	}
	composer := ""
	for _, contributor := range deezerResp.Contributors {
		if strings.Contains(strings.ToLower(contributor.Role), "compos") {
			composer = contributor.Name
			break
		}
	}
	genre := ""
	if len(deezerResp.Genre) > 0 {
		genre = deezerResp.Genre[0].Name
	}
	metadataPath := filepath.Join(tempDir, fileMarker+".txt")
	metadataContent := fmt.Sprintf(";FFMETADATA1\ntitle=%s\nartist=%s\nalbum=%s\n", escapeMetadataValue(deezerResp.Title), escapeMetadataValue(deezerResp.Artist.Name), escapeMetadataValue(deezerResp.Album.Title))

	if len(deezerResp.ReleaseDate) > 4 {
		metadataContent += fmt.Sprintf("date=%s\n", escapeMetadataValue(deezerResp.ReleaseDate[:4]))
	}
	if composer != "" {
		metadataContent += fmt.Sprintf("composer=%s\n", escapeMetadataValue(composer))
	}
	if genre != "" {
		metadataContent += fmt.Sprintf("genre=%s\n", escapeMetadataValue(genre))
	}
	if deezerResp.TrackNumber > 0 {
		metadataContent += fmt.Sprintf("track=%d\n", deezerResp.TrackNumber)
	}
	if deezerResp.DiskNumber > 0 {
		metadataContent += fmt.Sprintf("disc=%d\n", deezerResp.DiskNumber)
	}
	if err := os.WriteFile(metadataPath, []byte(metadataContent), 0644); err != nil {
		return fmt.Errorf("error writing metadata file: %v", err)
	}
	tempOutputPath := filepath.Join(tempDir, fileMarker+".m4a")

	err = applyMetadataWithFFmpeg(outputPath, metadataPath, artworkPath, tempOutputPath)
	if err != nil {
		return fmt.Errorf("error applying metadata: %v", err)
	}
	if err := os.Rename(tempOutputPath, outputPath); err != nil {
		return fmt.Errorf("error replacing original file: %v", err)
	}
	log.Debug().Str("title", deezerResp.Title).Str("artist", deezerResp.Artist.Name).Msg("Successfully added metadata")
	return nil
}

func applyMetadataWithFFmpeg(inputPath, metadataPath, artworkPath, outputPath string) error {
	log := utils.GetLogger("ffmpeg-metadata")
	args := []string{
		"-i", inputPath,
		"-i", metadataPath,
	}
	if artworkPath != "" {
		args = append(args, "-i", artworkPath, "-map", "0", "-map", "2")
		args = append(args, "-disposition:v:0", "attached_pic")
	}
	args = append(args, "-map_metadata", "1", "-codec", "copy")
	args = append(args, "-id3v2_version", "3", outputPath)

	cmd := exec.Command("ffmpeg", args...)
	log.Debug().Str("command", cmd.String()).Msg("Running FFmpeg")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug().Str("output", string(output)).Msg("FFmpeg error output")
		return fmt.Errorf("FFmpeg error: %v", err)
	}
	return nil
}

func downloadFile(url, filepath string, client *http.Client) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func escapeMetadataValue(value string) string {
	// Escape special characters for FFmpeg metadata
	value = strings.ReplaceAll(value, "=", "-")
	value = strings.ReplaceAll(value, ";", "-")
	value = strings.ReplaceAll(value, "#", "-")
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\r", "")
	return value
}

func addSpotifyMetadata(outputPath, musicId string) error {
	// TODO
	return fmt.Errorf("Spotify metadata not implemented yet")
}

func addAppleMetadata(outputPath, musicId string) error {
	// TODO
	return fmt.Errorf("Apple Music metadata not implemented yet")
}
