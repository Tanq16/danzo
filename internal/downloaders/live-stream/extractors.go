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
	username, hasUsername := job.Metadata["vimeo-username"].(string)
	password, hasPassword := job.Metadata["vimeo-password"].(string)
	if !hasUsername || username == "" {
		log.Error().Str("op", "live-stream/extractor").Msg("Vimeo username not provided (use --vimeo-username)")
	} else {
		log.Debug().Str("op", "live-stream/extractor").Msgf("Vimeo username found: %s", username)
	}
	if !hasPassword || password == "" {
		log.Error().Str("op", "live-stream/extractor").Msg("Vimeo password not provided (use --vimeo-password)")
	} else {
		log.Debug().Str("op", "live-stream/extractor").Msg("Vimeo password found")
	}

	if !hasUsername || username == "" || !hasPassword || password == "" {
		return fmt.Errorf("vimeo extractor requires --vimeo-username and --vimeo-password")
	}

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

	if err := authenticateVimeoUser(httpClient, username, password); err != nil {
		return err
	}
	log.Debug().Str("op", "live-stream/extractor").Msg("User authentication successful")
	if videoPassword, ok := job.Metadata["password"].(string); ok && videoPassword != "" {
		log.Debug().Str("op", "live-stream/extractor").Msgf("Submitting video password for video %s", videoID)
		if err := submitVimeoVideoPassword(httpClient, videoPageURL, videoPassword); err != nil {
			return fmt.Errorf("video password authentication failed: %v", err)
		}
		log.Debug().Str("op", "live-stream/extractor").Msg("Video password submitted successfully")
	}
	job.HTTPClientConfig.Jar = jar
	log.Debug().Str("op", "live-stream/extractor").Msg("Fetching video info from Vimeo API")

	var apiURL string
	if hash != "" {
		apiURL = fmt.Sprintf("https://api.vimeo.com/videos/%s:%s", videoID, hash)
	} else {
		apiURL = fmt.Sprintf("https://api.vimeo.com/videos/%s", videoID)
	}
	apiReq, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("error creating API request: %v", err)
	}
	viewerInfo, err := fetchVimeoViewerInfo(httpClient)
	if err == nil {
		if jwt, ok := viewerInfo["jwt"].(string); ok && jwt != "" {
			apiReq.Header.Set("Authorization", fmt.Sprintf("jwt %s", jwt))
			log.Debug().Str("op", "live-stream/extractor").Msg("Using JWT authentication for API")
		}
	}

	apiReq.Header.Set("Accept", "application/json")
	apiReq.Header.Set("Referer", videoPageURL)
	apiResp, err := httpClient.Do(apiReq)
	if err != nil {
		return fmt.Errorf("error fetching video API: %v", err)
	}
	apiBody, _ := io.ReadAll(apiResp.Body)
	apiResp.Body.Close()

	if apiResp.StatusCode != 200 {
		return fmt.Errorf("failed to fetch video API: status %d", apiResp.StatusCode)
	}
	var apiData map[string]interface{}
	if err := json.Unmarshal(apiBody, &apiData); err != nil {
		return fmt.Errorf("error decoding API response: %v", err)
	}
	var hlsURL string
	if play, ok := apiData["play"].(map[string]interface{}); ok {
		if hls, ok := play["hls"].(map[string]interface{}); ok {
			if cdns, ok := hls["cdns"].(map[string]interface{}); ok {
				for cdnName, cdnData := range cdns {
					if cdn, ok := cdnData.(map[string]interface{}); ok {
						if link, ok := cdn["url"].(string); ok {
							if strings.Contains(link, "/sep/video/") {
								hlsURL = link
								log.Debug().Str("op", "live-stream/extractor").Msgf("Found separated stream from CDN: %s", cdnName)
								break
							}
							if hlsURL == "" {
								hlsURL = link
								log.Debug().Str("op", "live-stream/extractor").Msgf("Found HLS stream from CDN: %s", cdnName)
							}
						}
					}
				}
			}
			if hlsURL == "" {
				if link, ok := hls["link"].(string); ok {
					hlsURL = link
				}
			}
		}
	}

	if hlsURL == "" {
		return fmt.Errorf("no HLS stream found in API response")
	}
	log.Info().Str("op", "live-stream/extractor").Msg("Extracted Vimeo HLS URL from API")
	job.URL = hlsURL
	if job.HTTPClientConfig.Headers == nil {
		job.HTTPClientConfig.Headers = make(map[string]string)
	}
	job.HTTPClientConfig.Headers["Referer"] = videoPageURL

	if job.OutputPath == "" {
		if name, ok := apiData["name"].(string); ok && name != "" {
			safeTitle := regexp.MustCompile(`[^a-zA-Z0-9\-\.]`).ReplaceAllString(name, "_")
			job.OutputPath = safeTitle + ".mp4"
		}
	}
	return nil
}

func authenticateVimeoUser(httpClient *utils.DanzoHTTPClient, username, password string) error {
	token, err := fetchVimeoXsrfToken(httpClient)
	if err != nil {
		log.Error().Str("op", "live-stream/extractor").Msgf("Failed to fetch Vimeo XSRF token: %v", err)
		return fmt.Errorf("failed to fetch Vimeo XSRF token: %v", err)
	}
	formData := fmt.Sprintf("action=login&email=%s&password=%s&service=vimeo&token=%s",
		username, password, token)
	req, err := http.NewRequest("POST", "https://vimeo.com/log_in", strings.NewReader(formData))
	if err != nil {
		return fmt.Errorf("error creating login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://vimeo.com/log_in")
	req.Header.Set("Origin", "https://vimeo.com")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("user authentication failed with status: %d (check credentials)", resp.StatusCode)
	}
	return nil
}

func submitVimeoVideoPassword(httpClient *utils.DanzoHTTPClient, videoPageURL, password string) error {
	viewerInfo, err := fetchVimeoViewerInfo(httpClient)
	if err != nil {
		return fmt.Errorf("failed to fetch viewer info: %v", err)
	}
	token, ok := viewerInfo["xsrft"].(string)
	if !ok || token == "" {
		return fmt.Errorf("no XSRF token available")
	}

	formData := fmt.Sprintf("password=%s&token=%s", password, token)
	authURL := strings.TrimSuffix(videoPageURL, "/") + "/password"
	req, err := http.NewRequest("POST", authURL, strings.NewReader(formData))
	if err != nil {
		return fmt.Errorf("error creating password request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", videoPageURL)
	req.Header.Set("Origin", "https://vimeo.com")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("password request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("password authentication failed with status: %d", resp.StatusCode)
	}
	return nil
}

func fetchVimeoViewerInfo(httpClient *utils.DanzoHTTPClient) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", "https://vimeo.com/_next/viewer", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vimeo viewer endpoint returned status %d", resp.StatusCode)
	}
	var viewer map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&viewer); err != nil {
		return nil, fmt.Errorf("failed to decode viewer info: %v", err)
	}
	return viewer, nil
}

func fetchVimeoXsrfToken(httpClient *utils.DanzoHTTPClient) (string, error) {
	viewer, err := fetchVimeoViewerInfo(httpClient)
	if err != nil {
		return "", err
	}
	if token, ok := viewer["xsrft"].(string); ok && token != "" {
		return token, nil
	}
	return "", fmt.Errorf("viewer token missing in response")
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
