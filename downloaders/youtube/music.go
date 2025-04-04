package youtube

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tanq16/danzo/utils"
)

// AppleMusicResponse represents the JSON response from the Apple Music API
type AppleMusicResponse struct {
	Data []struct {
		Attributes struct {
			Name       string `json:"name"`
			ArtistName string `json:"artistName"`
			AlbumName  string `json:"albumName"`
			Artwork    struct {
				URL    string `json:"url"`
				Height int    `json:"height"`
				Width  int    `json:"width"`
			} `json:"artwork"`
			ReleaseDate string   `json:"releaseDate"`
			GenreNames  []string `json:"genreNames"`
			TrackNumber int      `json:"trackNumber"`
			DiscNumber  int      `json:"discNumber"`
			Composer    string   `json:"composerName,omitempty"`
		} `json:"attributes"`
	} `json:"data"`
}

// M4AMetadata represents the metadata to be added to an M4A file
type M4AMetadata struct {
	Title       string
	Artist      string
	Album       string
	Year        int
	Genre       string
	TrackNumber int
	TrackTotal  int
	DiscNumber  int
	Composer    string
	AlbumArt    []byte
}

// DeezerResponse represents the JSON response from the Deezer API
type DeezerResponse struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Artist struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
		Cover string `json:"cover_xl"` // URL to the cover image (xl size)
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

	// Create HTTP client
	client := utils.CreateHTTPClient(30*time.Second, 30*time.Second, "", false)

	// Fetch the song metadata from Deezer API
	apiURL := fmt.Sprintf("https://api.deezer.com/track/%s", musicId)
	resp, err := client.Get(apiURL)
	if err != nil {
		return fmt.Errorf("error fetching metadata: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status code %d", resp.StatusCode)
	}

	// Parse the response
	var deezerResp DeezerResponse
	if err := json.NewDecoder(resp.Body).Decode(&deezerResp); err != nil {
		return fmt.Errorf("error parsing response: %v", err)
	}

	// Fetch album artwork if available
	var artworkData []byte
	if deezerResp.Album.Cover != "" {
		artReq, err := http.NewRequest("GET", deezerResp.Album.Cover, nil)
		if err != nil {
			log.Debug().Err(err).Msg("Error creating artwork request")
		} else {
			artResp, err := client.Do(artReq)
			if err != nil {
				log.Debug().Err(err).Msg("Error fetching artwork")
			} else {
				defer artResp.Body.Close()
				if artResp.StatusCode == http.StatusOK {
					artworkData, _ = io.ReadAll(artResp.Body)
					log.Debug().Int("artwork_size", len(artworkData)).Msg("Downloaded artwork")
				}
			}
		}
	}

	// Parse release date to get year
	year := 0
	if len(deezerResp.ReleaseDate) >= 4 {
		year, _ = strconv.Atoi(deezerResp.ReleaseDate[:4])
	}

	// Find composer if available
	composer := ""
	for _, contributor := range deezerResp.Contributors {
		if strings.Contains(strings.ToLower(contributor.Role), "compos") {
			composer = contributor.Name
			break
		}
	}

	// Get the first genre if available
	genre := ""
	if len(deezerResp.Genre) > 0 {
		genre = deezerResp.Genre[0].Name
	}

	// Prepare metadata
	metadata := M4AMetadata{
		Title:       deezerResp.Title,
		Artist:      deezerResp.Artist.Name,
		Album:       deezerResp.Album.Title,
		Year:        year,
		Genre:       genre,
		TrackNumber: deezerResp.TrackNumber,
		DiscNumber:  deezerResp.DiskNumber,
		Composer:    composer,
		AlbumArt:    artworkData,
	}

	// Apply metadata to the M4A file
	if err := writeM4AMetadata(outputPath, metadata); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	log.Debug().Str("title", metadata.Title).Str("artist", metadata.Artist).Msg("Successfully added metadata")
	return nil
}

func addSpotifyMetadata(outputPath, musicId string) error {
	// This is a placeholder for future implementation
	return nil
}

func addAppleMetadata(outputPath, musicId string) error {
	log := utils.GetLogger("apple-music")
	log.Debug().Str("musicId", musicId).Msg("Fetching Apple Music metadata")

	// Create HTTP client
	client := utils.CreateHTTPClient(30*time.Second, 30*time.Second, "", false)

	// Fetch the song metadata from Apple Music API
	apiURL := fmt.Sprintf("https://api.music.apple.com/v1/catalog/us/songs/%s", musicId)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// Apple Music API requires a developer token
	// For a proof of concept, we'll assume the token is stored in an environment variable
	developerToken := os.Getenv("APPLE_MUSIC_TOKEN")
	if developerToken == "" {
		return fmt.Errorf("Apple Music developer token not found. Set APPLE_MUSIC_TOKEN environment variable")
	}

	req.Header.Set("Authorization", "Bearer "+developerToken)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching metadata: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status code %d", resp.StatusCode)
	}

	// Parse the response
	var musicResp AppleMusicResponse
	if err := json.NewDecoder(resp.Body).Decode(&musicResp); err != nil {
		return fmt.Errorf("error parsing response: %v", err)
	}

	if len(musicResp.Data) == 0 {
		return fmt.Errorf("no data found for the provided Apple Music ID")
	}

	songData := musicResp.Data[0].Attributes

	// Fetch album artwork if available
	var artworkData []byte
	if songData.Artwork.URL != "" {
		artworkURL := songData.Artwork.URL
		// Replace width and height placeholders in the URL
		artworkURL = fmt.Sprintf(artworkURL, songData.Artwork.Width, songData.Artwork.Height)

		artReq, err := http.NewRequest("GET", artworkURL, nil)
		if err != nil {
			log.Debug().Err(err).Msg("Error creating artwork request")
		} else {
			artResp, err := client.Do(artReq)
			if err != nil {
				log.Debug().Err(err).Msg("Error fetching artwork")
			} else {
				defer artResp.Body.Close()
				if artResp.StatusCode == http.StatusOK {
					artworkData, _ = io.ReadAll(artResp.Body)
				}
			}
		}
	}

	// Parse release date to get year
	year := 0
	if len(songData.ReleaseDate) >= 4 {
		fmt.Sscanf(songData.ReleaseDate[:4], "%d", &year)
	}

	// Prepare metadata
	metadata := M4AMetadata{
		Title:       songData.Name,
		Artist:      songData.ArtistName,
		Album:       songData.AlbumName,
		Year:        year,
		TrackNumber: songData.TrackNumber,
		DiscNumber:  songData.DiscNumber,
		Composer:    songData.Composer,
		AlbumArt:    artworkData,
	}

	// Set genre if available
	if len(songData.GenreNames) > 0 {
		metadata.Genre = songData.GenreNames[0]
	}

	// Apply metadata to the M4A file
	if err := writeM4AMetadata(outputPath, metadata); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	log.Debug().Str("title", metadata.Title).Str("artist", metadata.Artist).Msg("Successfully added metadata")
	return nil
}

// parsedAtom represents an atom in the M4A file
type parsedAtom struct {
	Size uint32
	Type string
	Data []byte
}

// parseAtoms parses the atoms in an M4A file
func parseAtoms(data []byte) (map[string]parsedAtom, error) {
	atoms := make(map[string]parsedAtom)
	offset := 0

	for offset < len(data) {
		if offset+8 > len(data) {
			break // Not enough data for atom header
		}

		// Read atom size and type
		size := binary.BigEndian.Uint32(data[offset : offset+4])
		if size == 0 {
			break // End of atoms
		}

		atomType := string(data[offset+4 : offset+8])

		// Ensure we have enough data for the full atom
		if offset+int(size) > len(data) {
			return nil, fmt.Errorf("incomplete atom: %s (size: %d)", atomType, size)
		}

		// Store the atom
		atoms[atomType] = parsedAtom{
			Size: size,
			Type: atomType,
			Data: data[offset+8 : offset+int(size)],
		}

		// Move to the next atom
		offset += int(size)
	}

	return atoms, nil
}

func writeM4AMetadata(filePath string, metadata M4AMetadata) error {
	log := utils.GetLogger("m4a-metadata")

	// Open the file for reading and writing
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	// Read the entire file into memory
	fileData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading file: %v", err)
	}

	// A more structured approach to finding and modifying the metadata
	// Look for 'moov' -> 'udta' -> 'meta' -> 'ilst' (metadata container)
	moovStart := -1
	for i := 0; i < len(fileData)-4; i++ {
		if string(fileData[i:i+4]) == "moov" {
			moovStart = i - 4 // Include size field
			break
		}
	}

	if moovStart == -1 {
		return fmt.Errorf("moov atom not found in the file")
	}

	// Extract the moov atom
	moovSize := int(binary.BigEndian.Uint32(fileData[moovStart : moovStart+4]))
	if moovStart+moovSize > len(fileData) {
		return fmt.Errorf("invalid moov atom size")
	}

	moovData := fileData[moovStart : moovStart+moovSize]

	// Create metadata atoms
	metadataAtoms := createM4AMetadataAtoms(metadata)

	// Create the udta atom if it doesn't exist
	udtaAtom := []byte{}
	udtaAtom = append(udtaAtom, 0, 0, 0, 0) // Placeholder for size
	udtaAtom = append(udtaAtom, []byte("udta")...)

	// Create the meta atom
	metaAtom := []byte{}
	metaAtom = append(metaAtom, 0, 0, 0, 0) // Placeholder for size
	metaAtom = append(metaAtom, []byte("meta")...)
	metaAtom = append(metaAtom, 0, 0, 0, 0) // Version/flags

	// Create the ilst atom
	ilstAtom := []byte{}
	ilstAtom = append(ilstAtom, 0, 0, 0, 0) // Placeholder for size
	ilstAtom = append(ilstAtom, []byte("ilst")...)

	// Append the metadata atoms to ilst
	ilstAtom = append(ilstAtom, metadataAtoms...)

	// Update ilst size
	binary.BigEndian.PutUint32(ilstAtom[:4], uint32(len(ilstAtom)))

	// Append ilst to meta
	metaAtom = append(metaAtom, ilstAtom...)

	// Update meta size
	binary.BigEndian.PutUint32(metaAtom[:4], uint32(len(metaAtom)))

	// Append meta to udta
	udtaAtom = append(udtaAtom, metaAtom...)

	// Update udta size
	binary.BigEndian.PutUint32(udtaAtom[:4], uint32(len(udtaAtom)))

	// Create a new moov atom with updated content
	newMoov := []byte{}
	newMoov = append(newMoov, 0, 0, 0, 0) // Placeholder for size
	newMoov = append(newMoov, []byte("moov")...)

	// Copy existing moov content (excluding header) and append our udta
	newMoov = append(newMoov, moovData[8:]...)
	newMoov = append(newMoov, udtaAtom...)

	// Update moov size
	binary.BigEndian.PutUint32(newMoov[:4], uint32(len(newMoov)))

	// Create the new file data
	newFileData := []byte{}
	newFileData = append(newFileData, fileData[:moovStart]...)
	newFileData = append(newFileData, newMoov...)
	newFileData = append(newFileData, fileData[moovStart+moovSize:]...)

	// Write the modified file back
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("error seeking to beginning of file: %v", err)
	}

	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("error truncating file: %v", err)
	}

	if _, err := file.Write(newFileData); err != nil {
		return fmt.Errorf("error writing modified file: %v", err)
	}

	log.Debug().Int("fileSize", len(newFileData)).Msg("Successfully wrote metadata to file")
	return nil
}

func createM4AMetadataAtoms(metadata M4AMetadata) []byte {
	buffer := new(bytes.Buffer)

	// Add metadata atoms
	addTextMetadataAtom(buffer, "©nam", metadata.Title)    // Title
	addTextMetadataAtom(buffer, "©ART", metadata.Artist)   // Artist
	addTextMetadataAtom(buffer, "©alb", metadata.Album)    // Album
	addTextMetadataAtom(buffer, "©wrt", metadata.Composer) // Composer
	addTextMetadataAtom(buffer, "©gen", metadata.Genre)    // Genre

	// Add year
	if metadata.Year > 0 {
		yearStr := fmt.Sprintf("%d", metadata.Year)
		addTextMetadataAtom(buffer, "©day", yearStr)
	}

	// Add track number - proper trkn atom structure
	if metadata.TrackNumber > 0 {
		trackData := []byte{
			0, 0, // Empty bytes (reserved)
			0, 0, // Empty bytes (reserved)
			byte(metadata.TrackNumber >> 8), byte(metadata.TrackNumber), // Track number (big endian)
			byte(metadata.TrackTotal >> 8), byte(metadata.TrackTotal), // Total tracks (big endian)
		}
		addDataMetadataAtom(buffer, "trkn", trackData, 0)
	}

	// Add disc number - proper disk atom structure
	if metadata.DiscNumber > 0 {
		discData := []byte{
			0, 0, // Empty bytes (reserved)
			0, 0, // Empty bytes (reserved)
			byte(metadata.DiscNumber >> 8), byte(metadata.DiscNumber), // Disc number (big endian)
			0, 0, // Total discs (not specified)
		}
		addDataMetadataAtom(buffer, "disk", discData, 0)
	}

	// Add album art if available
	if len(metadata.AlbumArt) > 0 {
		addCoverArt(buffer, metadata.AlbumArt)
	}

	return buffer.Bytes()
}

// addTextMetadataAtom adds a text metadata atom to the buffer
func addTextMetadataAtom(buffer *bytes.Buffer, name string, value string) {
	if value == "" {
		return
	}

	innerBuffer := new(bytes.Buffer)

	// Write data atom
	innerBuffer.Write([]byte{0, 0, 0, 1}) // Version/flags = 1 (text)
	innerBuffer.Write([]byte{0, 0, 0, 0}) // Reserved
	innerBuffer.Write([]byte(value))

	// Calculate atom size
	atomSize := 8 + innerBuffer.Len() // atom header + data atom

	// Create the containing atom
	binary.Write(buffer, binary.BigEndian, uint32(atomSize))
	buffer.Write([]byte(name))
	buffer.Write(innerBuffer.Bytes())
}

// addDataMetadataAtom adds a data metadata atom with raw data
func addDataMetadataAtom(buffer *bytes.Buffer, name string, data []byte, dataType int) {
	innerBuffer := new(bytes.Buffer)

	// Write data atom
	binary.Write(innerBuffer, binary.BigEndian, uint32(dataType)) // Data type
	innerBuffer.Write([]byte{0, 0, 0, 0})                         // Reserved
	innerBuffer.Write(data)

	// Calculate atom size
	atomSize := 8 + innerBuffer.Len() // atom header + data atom

	// Create the containing atom
	binary.Write(buffer, binary.BigEndian, uint32(atomSize))
	buffer.Write([]byte(name))
	buffer.Write(innerBuffer.Bytes())
}

// addCoverArt adds album artwork to the metadata
func addCoverArt(buffer *bytes.Buffer, imageData []byte) {
	// Determine image type
	var imageType int
	if len(imageData) > 2 && imageData[0] == 0xFF && imageData[1] == 0xD8 {
		// JPEG signature
		imageType = 13 // JPEG
	} else if len(imageData) > 8 &&
		imageData[0] == 0x89 && imageData[1] == 0x50 &&
		imageData[2] == 0x4E && imageData[3] == 0x47 {
		// PNG signature
		imageType = 14 // PNG
	} else {
		// Unknown format, default to JPEG
		imageType = 13
	}

	innerBuffer := new(bytes.Buffer)

	// Write data atom
	binary.Write(innerBuffer, binary.BigEndian, uint32(imageType)) // Image type
	innerBuffer.Write([]byte{0, 0, 0, 0})                          // Reserved
	innerBuffer.Write(imageData)

	// Calculate atom size
	atomSize := 8 + innerBuffer.Len() // atom header + data atom

	// Create the containing atom
	binary.Write(buffer, binary.BigEndian, uint32(atomSize))
	buffer.Write([]byte("covr"))
	buffer.Write(innerBuffer.Bytes())
}
