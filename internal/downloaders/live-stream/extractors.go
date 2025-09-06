package m3u8

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

// JSON response from Rumble embedJS endpoint
type RumbleJSResponse struct {
	U struct {
		HLS struct {
			URL string `json:"url"`
		} `json:"hls"`
	} `json:"u"`
	UA struct {
		HLS map[string]struct {
			URL string `json:"url"`
		} `json:"hls"`
	} `json:"ua"`
}

func runExtractor(job *utils.DanzoJob) error {
	extractor, _ := job.Metadata["extract"].(string)
	switch strings.ToLower(extractor) {
	case "rumble":
		return extractRumbleURL(job)
	default:
		return fmt.Errorf("unsupported extractor: %s", extractor)
	}
}

func extractRumbleURL(job *utils.DanzoJob) error {
	log.Debug().Str("op", "live-stream/extractor").Msgf("Extracting Rumble URL from %s", job.URL)
	videoID, err := getRumbleVideoID(job.URL)
	if err != nil {
		return err
	}
	log.Debug().Str("op", "live-stream/extractor").Msgf("Found Rumble video ID: %s", videoID)
	m3u8URL, err := getRumbleM3U8FromVideoID(videoID, job.HTTPClientConfig)
	if err != nil {
		return err
	}
	job.URL = m3u8URL
	return nil
}

func getRumbleVideoID(pageURL string) (string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for rumble page: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch rumble page: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read rumble page body: %w", err)
	}

	re := regexp.MustCompile(`"embedUrl":\s*"https://rumble\.com/embed/([^/"]+)/"`)
	// re := regexp.MustCompile(`https://rumble\.com/embed/([^&",/]*)`)
	matches := re.FindStringSubmatch(string(body))
	if len(matches) >= 2 {
		return matches[1], nil
	}
	return "", fmt.Errorf("could not find rumble video ID in page source")
}

func getRumbleM3U8FromVideoID(videoID string, clientConfig utils.HTTPClientConfig) (string, error) {
	jsonURL := fmt.Sprintf("https://rumble.com/embedJS/u3/?request=video&ver=2&v=%s", videoID)
	newClientConfig := clientConfig
	newClientConfig.Headers = make(map[string]string)
	maps.Copy(newClientConfig.Headers, clientConfig.Headers)
	newClientConfig.Headers["Referer"] = "https://rumble.com/"
	client := utils.NewDanzoHTTPClient(newClientConfig)

	req, err := http.NewRequest("GET", jsonURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for rumble json: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch rumble json: %w", err)
	}
	defer resp.Body.Close()

	var data RumbleJSResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("failed to decode rumble json: %w", err)
	}
	if data.U.HLS.URL != "" {
		return data.U.HLS.URL, nil
	}
	if auto, ok := data.UA.HLS["auto"]; ok && auto.URL != "" {
		return auto.URL, nil
	}
	return "", fmt.Errorf("could not find m3u8 url in rumble json response")
}
