package youtube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tanq16/danzo/internal/utils"
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

type ITunesResponse struct {
	ResultCount int `json:"resultCount"`
	Results     []struct {
		TrackName        string `json:"trackName"`
		ArtistName       string `json:"artistName"`
		CollectionName   string `json:"collectionName"`
		ReleaseDate      string `json:"releaseDate"`
		PrimaryGenreName string `json:"primaryGenreName"`
		TrackNumber      int    `json:"trackNumber"`
		DiscNumber       int    `json:"discNumber"`
		TrackCount       int    `json:"trackCount"`
		DiscCount        int    `json:"discCount"`
		ArtworkUrl100    string `json:"artworkUrl100"`
	} `json:"results"`
}

var ytHTTPConfig = utils.HTTPClientConfig{
	Timeout:        30 * time.Second,
	KATimeout:      30 * time.Second,
	HighThreadMode: false,
}

func addMusicMetadata(outputPath, musicClient, musicId string) error {
	switch musicClient {
	case "deezer":
		return addDeezerMetadata(outputPath, musicId)
	// case "spotify":
	// 	return addSpotifyMetadata(outputPath, musicId) // Dropped for now because of API Token requirements
	case "apple":
		return addAppleMetadata(outputPath, musicId)
	default:
		return fmt.Errorf("unsupported music client: %s", musicClient)
	}
}

func addAppleMetadata(outputPath, musicId string) error {
	client := utils.NewDanzoHTTPClient(ytHTTPConfig)
	apiURL := fmt.Sprintf("https://itunes.apple.com/lookup?id=%s&entity=song", musicId)
	req, _ := http.NewRequest("GET", apiURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching metadata: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status code %d", resp.StatusCode)
	}
	var itunesResp ITunesResponse
	if err := json.NewDecoder(resp.Body).Decode(&itunesResp); err != nil {
		return fmt.Errorf("error parsing response: %v", err)
	}
	if itunesResp.ResultCount == 0 || len(itunesResp.Results) == 0 {
		return fmt.Errorf("no results found for iTunes ID: %s", musicId)
	}

	trackInfo := itunesResp.Results[0]
	tempDir := filepath.Join(filepath.Dir(outputPath), ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fileMarker := uuid.New().String()
	var artworkPath string

	// iTunes seems to have 100x100 by default, so brute-force higher quality once just in case
	if trackInfo.ArtworkUrl100 != "" {
		highResArtwork := strings.Replace(trackInfo.ArtworkUrl100, "100x100", "1000x1000", 1)
		midResArtwork := strings.Replace(trackInfo.ArtworkUrl100, "100x100", "600x600", 1)
		artworkPath = filepath.Join(tempDir, fileMarker+".jpg")
		err := downloadFile(highResArtwork, artworkPath, client)
		if err != nil {
			err = downloadFile(midResArtwork, artworkPath, client)
			if err != nil {
				err = downloadFile(trackInfo.ArtworkUrl100, artworkPath, client)
				if err != nil {
					artworkPath = ""
				}
			}
		}
	}

	escapeRegex := regexp.MustCompile(`[^a-zA-Z0-9\s\-_]`)
	var escapeRE = func(s string) string {
		return escapeRegex.ReplaceAllString(s, "")
	}
	metadataPath := filepath.Join(tempDir, fileMarker+".txt")
	metadataContent := fmt.Sprintf(";FFMETADATA1\ntitle=%s\nartist=%s\nalbum=%s\n", escapeRE(trackInfo.TrackName), escapeRE(trackInfo.ArtistName), escapeRE(trackInfo.CollectionName))
	if trackInfo.ReleaseDate != "" {
		if len(trackInfo.ReleaseDate) > 10 {
			extractedDate, _ := time.Parse("2006-01-02T15:04:05Z", trackInfo.ReleaseDate)
			metadataContent += fmt.Sprintf("date=%s\n", extractedDate.Format("2006-01-02"))
		} else {
			metadataContent += fmt.Sprintf("date=%s\n", escapeRE(trackInfo.ReleaseDate))
		}
	}
	if trackInfo.PrimaryGenreName != "" {
		metadataContent += fmt.Sprintf("genre=%s\n", escapeRE(trackInfo.PrimaryGenreName))
	}
	if trackInfo.TrackNumber > 0 {
		if trackInfo.TrackCount > 0 {
			metadataContent += fmt.Sprintf("track=%d/%d\n", trackInfo.TrackNumber, trackInfo.TrackCount)
		} else {
			metadataContent += fmt.Sprintf("track=%d\n", trackInfo.TrackNumber)
		}
	}
	if trackInfo.DiscNumber > 0 {
		if trackInfo.DiscCount > 0 {
			metadataContent += fmt.Sprintf("disc=%d/%d\n", trackInfo.DiscNumber, trackInfo.DiscCount)
		} else {
			metadataContent += fmt.Sprintf("disc=%d\n", trackInfo.DiscNumber)
		}
	}
	if err := os.WriteFile(metadataPath, []byte(metadataContent), 0644); err != nil {
		return fmt.Errorf("error writing metadata file: %v", err)
	}

	tempOutputPath := filepath.Join(tempDir, fileMarker+".m4a")
	err = applyMetadataWithFFmpeg(outputPath, metadataPath, artworkPath, tempOutputPath)
	if err != nil {
		return fmt.Errorf("error applying metadata with FFmpeg: %v", err)
	}
	if err := os.Rename(tempOutputPath, outputPath); err != nil {
		return fmt.Errorf("error replacing original file: %v", err)
	}
	return nil
}

func addDeezerMetadata(outputPath, musicId string) error {
	client := utils.NewDanzoHTTPClient(ytHTTPConfig)
	apiURL := fmt.Sprintf("https://api.deezer.com/track/%s", musicId)
	req, _ := http.NewRequest("GET", apiURL, nil)
	resp, err := client.Do(req)
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
			artworkPath = ""
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
	escapeRegex := regexp.MustCompile(`[^a-zA-Z0-9\s\-_]`)
	var escapeRE = func(s string) string {
		return escapeRegex.ReplaceAllString(s, "")
	}
	metadataPath := filepath.Join(tempDir, fileMarker+".txt")
	metadataContent := fmt.Sprintf(";FFMETADATA1\ntitle=%s\nartist=%s\nalbum=%s\n", escapeRE(deezerResp.Title), escapeRE(deezerResp.Artist.Name), escapeRE(deezerResp.Album.Title))

	if len(deezerResp.ReleaseDate) > 4 {
		metadataContent += fmt.Sprintf("date=%s\n", escapeRE(deezerResp.ReleaseDate))
	}
	if composer != "" {
		metadataContent += fmt.Sprintf("composer=%s\n", escapeRE(composer))
	}
	if genre != "" {
		metadataContent += fmt.Sprintf("genre=%s\n", escapeRE(genre))
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
	return nil
}

func applyMetadataWithFFmpeg(inputPath, metadataPath, artworkPath, outputPath string) error {
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
	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg error: %v", err)
	}
	return nil
}

func downloadFile(url, filepath string, client *utils.DanzoHTTPClient) error {
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)
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
