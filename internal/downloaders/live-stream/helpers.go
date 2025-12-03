package m3u8

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

type M3U8Info struct {
	VideoSegmentURLs []string
	AudioSegmentURLs []string
	VideoInitSegment string
	AudioInitSegment string
	HasSeparateAudio bool
}

func getM3U8Contents(manifestURL string, client *utils.DanzoHTTPClient) (string, error) {
	req, err := http.NewRequest("GET", manifestURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error fetching m3u8 manifest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status code %d", resp.StatusCode)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading manifest content: %v", err)
	}
	log.Debug().Str("op", "live-stream/helpers").Msgf("Successfully read manifest from %s", manifestURL)
	return string(content), nil
}

func parseM3U8Content(content, manifestURL string, client *utils.DanzoHTTPClient) (*M3U8Info, error) {
	baseURL, err := url.Parse(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest URL: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	var segmentURLs []string
	var masterPlaylistURLs []string
	var audioPlaylistURLs []string
	var isMasterPlaylist bool
	var initSegment string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Check for init segment (fMP4)
		if strings.HasPrefix(line, "#EXT-X-MAP:") {
			if idx := strings.Index(line, `URI="`); idx != -1 {
				uriStart := idx + 5
				if uriEnd := strings.Index(line[uriStart:], `"`); uriEnd != -1 {
					uri := line[uriStart : uriStart+uriEnd]
					initSegment, err = resolveURL(baseURL, uri)
					if err != nil {
						return nil, fmt.Errorf("error resolving init segment URL: %v", err)
					}
					log.Debug().Str("op", "live-stream/helpers").Msgf("Found init segment: %s", initSegment)
				}
			}
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-MEDIA:") && strings.Contains(line, "TYPE=AUDIO") {
			isMasterPlaylist = true
			if idx := strings.Index(line, `URI="`); idx != -1 {
				uriStart := idx + 5
				if uriEnd := strings.Index(line[uriStart:], `"`); uriEnd != -1 {
					uri := line[uriStart : uriStart+uriEnd]
					audioURL, err := resolveURL(baseURL, uri)
					if err != nil {
						return nil, fmt.Errorf("error resolving audio URL: %v", err)
					}
					audioPlaylistURLs = append(audioPlaylistURLs, audioURL)
				}
			}
			continue
		}
		if strings.HasPrefix(line, "#") && !strings.Contains(line, "#EXT-X-STREAM-INF") {
			continue
		}
		if strings.Contains(line, "#EXT-X-STREAM-INF") {
			isMasterPlaylist = true
			continue
		}
		if !strings.HasPrefix(line, "#") {
			segmentURL, err := resolveURL(baseURL, line)
			if err != nil {
				return nil, fmt.Errorf("error resolving URL: %v", err)
			}
			if isMasterPlaylist {
				masterPlaylistURLs = append(masterPlaylistURLs, segmentURL)
			} else {
				segmentURLs = append(segmentURLs, segmentURL)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning m3u8 content: %v", err)
	}

	if isMasterPlaylist && len(masterPlaylistURLs) > 0 {
		log.Debug().Str("op", "live-stream/helpers").Msgf("Detected master playlist with %d video variants and %d audio tracks", len(masterPlaylistURLs), len(audioPlaylistURLs))

		videoContent, err := getM3U8Contents(masterPlaylistURLs[0], client)
		if err != nil {
			return nil, fmt.Errorf("error fetching video sub-playlist: %v", err)
		}
		videoInfo, err := parseM3U8Content(videoContent, masterPlaylistURLs[0], client)
		if err != nil {
			return nil, fmt.Errorf("error parsing video sub-playlist: %v", err)
		}

		if len(audioPlaylistURLs) > 0 {
			audioContent, err := getM3U8Contents(audioPlaylistURLs[0], client)
			if err != nil {
				return nil, fmt.Errorf("error fetching audio sub-playlist: %v", err)
			}
			audioInfo, err := parseM3U8Content(audioContent, audioPlaylistURLs[0], client)
			if err != nil {
				return nil, fmt.Errorf("error parsing audio sub-playlist: %v", err)
			}

			return &M3U8Info{
				VideoSegmentURLs: videoInfo.VideoSegmentURLs,
				AudioSegmentURLs: audioInfo.VideoSegmentURLs,
				VideoInitSegment: videoInfo.VideoInitSegment,
				AudioInitSegment: audioInfo.VideoInitSegment,
				HasSeparateAudio: true,
			}, nil
		}

		return videoInfo, nil
	}

	return &M3U8Info{
		VideoSegmentURLs: segmentURLs,
		VideoInitSegment: initSegment,
		HasSeparateAudio: false,
	}, nil
}

func resolveURL(baseURL *url.URL, urlStr string) (string, error) {
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		return urlStr, nil
	}
	relURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	absURL := baseURL.ResolveReference(relURL)
	return absURL.String(), nil
}

func calculateTotalSize(segmentURLs []string, numWorkers int, client *utils.DanzoHTTPClient) (int64, []int64, error) {
	segmentSizes := make([]int64, len(segmentURLs))
	var totalSize int64
	var mu sync.Mutex
	var sizeErr error
	type sizeJob struct {
		index int
		url   string
	}
	jobCh := make(chan sizeJob, len(segmentURLs))
	for i, url := range segmentURLs {
		jobCh <- sizeJob{index: i, url: url}
	}
	close(jobCh)
	log.Debug().Str("op", "live-stream/helpers").Msg("Calculating total size of all segments")
	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				size, err := getSize(job.url, client)
				if err != nil {
					mu.Lock()
					if sizeErr == nil {
						sizeErr = err
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				segmentSizes[job.index] = size
				totalSize += size
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if sizeErr != nil {
		return 0, nil, sizeErr
	}
	return totalSize, segmentSizes, nil
}

func getSize(url string, client *utils.DanzoHTTPClient) (int64, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("server returned status code %d", resp.StatusCode)
	}
	contentLength := resp.Header.Get("Content-Length")
	if contentLength == "" {
		return 0, fmt.Errorf("no content length")
	}
	var size int64
	fmt.Sscanf(contentLength, "%d", &size)
	return size, nil
}

func downloadSegment(segmentURL, outputPath string, client *utils.DanzoHTTPClient) (int64, error) {
	req, err := http.NewRequest("GET", segmentURL, nil)
	if err != nil {
		return 0, fmt.Errorf("error creating request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error downloading segment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("server returned status code %d", resp.StatusCode)
	}
	outFile, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("error creating output file: %v", err)
	}
	defer outFile.Close()
	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("error writing segment: %v", err)
	}
	return written, nil
}
