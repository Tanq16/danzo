package m3u8

import (
	"bytes"
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

// JSON response from Dailymotion metadata endpoint
type DailymotionMetadata struct {
	Qualities map[string][]struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"qualities"`
}

// Vimeo Player Config Structure
type VimeoConfig struct {
	Request struct {
		Files struct {
			Hls struct {
				Cdns map[string]struct {
					URL string `json:"url"`
				} `json:"cdns"`
				DefaultCdn string `json:"default_cdn"`
			} `json:"hls"`
		} `json:"files"`
	} `json:"request"`
	Video struct {
		Title string `json:"title"`
	} `json:"video"`
}

type vimeoViewerInfo struct {
	XSRFT string `json:"xsrft"`
}

func runExtractor(job *utils.DanzoJob) error {
	extractor, _ := job.Metadata["extract"].(string)
	switch strings.ToLower(extractor) {
	case "rumble":
		return extractRumbleURL(job)
	case "dailymotion":
		return extractDailymotionURL(job)
	case "vimeo":
		return extractVimeoURL(job)
	default:
		return nil
	}
}

func extractVimeoURL(job *utils.DanzoJob) error {
	log.Debug().Str("op", "live-stream/extractor").Msgf("Extracting Vimeo URL from %s", job.URL)
	// Regex to catch ID and optional Hash (for unlisted videos)
	re := regexp.MustCompile(`vimeo\.com/(?:channels/[\w]+/|groups/[\w]+/videos/|video/|)(\d+)(?:/([a-zA-Z0-9]+))?`)
	matches := re.FindStringSubmatch(job.URL)
	if len(matches) < 2 {
		return fmt.Errorf("could not parse Vimeo video ID from URL")
	}
	videoID := matches[1]
	hash := ""
	if len(matches) > 2 {
		hash = matches[2]
	}
	videoPageURL := fmt.Sprintf("https://vimeo.com/%s", videoID)
	if hash != "" {
		videoPageURL = fmt.Sprintf("%s/%s", videoPageURL, hash)
	}

	jar, _ := cookiejar.New(nil)
	newVimeoHTTPConfig := job.HTTPClientConfig
	newVimeoHTTPConfig.Headers = make(map[string]string)
	maps.Copy(newVimeoHTTPConfig.Headers, job.HTTPClientConfig.Headers)
	newVimeoHTTPConfig.Headers["Referer"] = videoPageURL
	newVimeoHTTPConfig.Headers["User-Agent"] = utils.GetRandomUserAgent()
	newVimeoHTTPConfig.Jar = jar
	httpClient := utils.NewDanzoHTTPClient(newVimeoHTTPConfig)

	if password, ok := job.Metadata["password"].(string); ok && password != "" {
		log.Debug().Str("op", "live-stream/extractor").Msgf("Attempting password authentication for video %s", videoID)
		if err := authenticateVimeoPassword(httpClient, videoPageURL, password); err != nil {
			return err
		}
		log.Debug().Str("op", "live-stream/extractor").Msg("Password authentication successful, cookies set")
	}
	job.HTTPClientConfig.Jar = jar

	configURL := fmt.Sprintf("https://player.vimeo.com/video/%s/config", videoID)
	if hash != "" {
		configURL += fmt.Sprintf("?h=%s", hash)
	}
	log.Debug().Str("op", "live-stream/extractor").Msgf("Fetching Vimeo config from %s", configURL)
	req, err := http.NewRequest("GET", configURL, nil)
	if err != nil {
		return fmt.Errorf("error creating vimeo request: %v", err)
	}

	req.Header.Set("Referer", videoPageURL)
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching vimeo config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 403 {
			return fmt.Errorf("vimeo video is password protected or restricted (403); did you provide the correct --password?")
		}
		return fmt.Errorf("vimeo config returned status %d", resp.StatusCode)
	}

	var config VimeoConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return fmt.Errorf("error decoding vimeo config: %v", err)
	}
	hlsData := config.Request.Files.Hls
	if len(hlsData.Cdns) == 0 {
		return fmt.Errorf("no HLS streams found in vimeo config (video might be processing or unavailable)")
	}

	var masterURL string
	if defaultCDN, ok := hlsData.Cdns[hlsData.DefaultCdn]; ok {
		masterURL = defaultCDN.URL
	} else {
		for _, cdn := range hlsData.Cdns {
			masterURL = cdn.URL
			break
		}
	}
	if masterURL == "" {
		return fmt.Errorf("extracted HLS URL is empty")
	}
	log.Info().Str("op", "live-stream/extractor").Msgf("Extracted Vimeo HLS URL")
	job.URL = masterURL

	if job.HTTPClientConfig.Headers == nil {
		job.HTTPClientConfig.Headers = make(map[string]string)
	}
	job.HTTPClientConfig.Headers["Referer"] = videoPageURL
	if job.OutputPath == "" && config.Video.Title != "" {
		safeTitle := regexp.MustCompile(`[^a-zA-Z0-9\-\.]`).ReplaceAllString(config.Video.Title, "_")
		job.OutputPath = safeTitle + ".mp4"
	}
	return nil
}

func authenticateVimeoPassword(httpClient *utils.DanzoHTTPClient, videoPageURL, password string) error {
	if err := warmVimeoSession(httpClient, videoPageURL); err != nil {
		log.Debug().Str("op", "live-stream/extractor").Msgf("Failed to warm Vimeo session: %v", err)
	}
	token, err := fetchVimeoXsrfToken(httpClient)
	if err != nil {
		log.Error().Str("op", "live-stream/extractor").Msgf("Failed to fetch Vimeo viewer token: %v", err)
		return fmt.Errorf("failed to fetch Vimeo auth token: %v", err)
	}
	if err := submitVimeoPassword(httpClient, videoPageURL, password, token); err != nil {
		log.Error().Str("op", "live-stream/extractor").Msgf("Vimeo password submission failed: %v", err)
		return err
	}
	return nil
}

func warmVimeoSession(httpClient *utils.DanzoHTTPClient, videoPageURL string) error {
	req, err := http.NewRequest("GET", videoPageURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", videoPageURL)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func fetchVimeoXsrfToken(httpClient *utils.DanzoHTTPClient) (string, error) {
	req, err := http.NewRequest("GET", "https://vimeo.com/_next/viewer", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vimeo viewer endpoint returned status %d", resp.StatusCode)
	}
	var viewer vimeoViewerInfo
	if err := json.NewDecoder(resp.Body).Decode(&viewer); err != nil {
		return "", fmt.Errorf("failed to decode viewer info: %v", err)
	}
	if viewer.XSRFT == "" {
		return "", fmt.Errorf("viewer token missing in response")
	}
	return viewer.XSRFT, nil
}

func submitVimeoPassword(httpClient *utils.DanzoHTTPClient, videoPageURL, password, token string) error {
	payload := map[string]string{
		"password": password,
		"token":    token,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error creating password payload: %v", err)
	}
	authURL := strings.TrimSuffix(videoPageURL, "/") + "/password"
	req, err := http.NewRequest("POST", authURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error creating auth request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", videoPageURL)
	req.Header.Set("Origin", "https://vimeo.com")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("password auth request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTeapot {
		return fmt.Errorf("vimeo password authentication failed: incorrect password (418)")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("password authentication failed with status: %d (check your password)", resp.StatusCode)
	}
	return nil
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

func extractDailymotionURL(job *utils.DanzoJob) error {
	log.Debug().Str("op", "live-stream/extractor").Msgf("Extracting Dailymotion URL from %s", job.URL)
	videoID, err := getDailymotionVideoID(job.URL)
	if err != nil {
		return err
	}
	log.Debug().Str("op", "live-stream/extractor").Msgf("Found Dailymotion video ID: %s", videoID)
	m3u8URL, err := getDailymotionM3U8FromVideoID(videoID, job.HTTPClientConfig)
	if err != nil {
		return err
	}
	job.URL = m3u8URL
	return nil
}

func getDailymotionVideoID(pageURL string) (string, error) {
	re := regexp.MustCompile(`dai\.ly/([^/?&#]+)`) // dai.ly/{id}
	if matches := re.FindStringSubmatch(pageURL); len(matches) >= 2 {
		return matches[1], nil
	}
	re = regexp.MustCompile(`dailymotion\.[a-z]{2,3}/video/([^/?&#]+)`) // dailymotion.com/video/{id}
	if matches := re.FindStringSubmatch(pageURL); len(matches) >= 2 {
		return matches[1], nil
	}
	re = regexp.MustCompile(`[?&]video=([^&#]+)`) // player.html?video={id}
	if matches := re.FindStringSubmatch(pageURL); len(matches) >= 2 {
		return matches[1], nil
	}
	return "", fmt.Errorf("could not extract Dailymotion video ID from URL: %s", pageURL)
}

func getDailymotionM3U8FromVideoID(videoID string, clientConfig utils.HTTPClientConfig) (string, error) {
	metadataURL := fmt.Sprintf("https://www.dailymotion.com/player/metadata/video/%s", videoID)
	newClientConfig := clientConfig
	newClientConfig.Headers = make(map[string]string)
	maps.Copy(newClientConfig.Headers, clientConfig.Headers)
	newClientConfig.Headers["Referer"] = "https://www.dailymotion.com/"
	newClientConfig.Headers["Origin"] = "https://www.dailymotion.com"
	client := utils.NewDanzoHTTPClient(newClientConfig)
	req, err := http.NewRequest("GET", metadataURL+"?app=com.dailymotion.neon", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for dailymotion metadata: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch dailymotion metadata: %w", err)
	}
	defer resp.Body.Close()
	var metadata DailymotionMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return "", fmt.Errorf("failed to decode dailymotion metadata: %w", err)
	}
	qualityPriority := []string{"auto", "1080", "720", "480", "380", "240"}
	for _, quality := range qualityPriority {
		if mediaList, ok := metadata.Qualities[quality]; ok {
			for _, media := range mediaList {
				if media.Type == "application/x-mpegURL" && media.URL != "" {
					log.Debug().Str("op", "live-stream/extractor").Msgf("Found m3u8 URL at quality %s", quality)
					return media.URL, nil
				}
			}
		}
	}
	for quality, mediaList := range metadata.Qualities {
		for _, media := range mediaList {
			if media.Type == "application/x-mpegURL" && media.URL != "" {
				log.Debug().Str("op", "live-stream/extractor").Msgf("Found m3u8 URL at quality %s", quality)
				return media.URL, nil
			}
		}
	}
	return "", fmt.Errorf("could not find m3u8 URL in dailymotion metadata response")
}
