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

type audioTrack struct {
	url     string
	quality int
}

func parseM3U8Content(content, manifestURL string, client *utils.DanzoHTTPClient) (*M3U8Info, error) {
	baseURL, err := url.Parse(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest URL: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	var segmentURLs []string
	var masterPlaylistURLs []string
	var masterPlaylistBandwidths []int
	var audioTracks []audioTrack
	var isMasterPlaylist bool
	var initSegment string
	var currentBandwidth int

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
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
			log.Debug().Str("op", "live-stream/helpers").Msgf("Found audio media line: %s", line)
			var audioURL string
			if idx := strings.Index(line, `URI="`); idx != -1 {
				uriStart := idx + 5
				if uriEnd := strings.Index(line[uriStart:], `"`); uriEnd != -1 {
					uri := line[uriStart : uriStart+uriEnd]
					audioURL, err = resolveURL(baseURL, uri)
					if err != nil {
						return nil, fmt.Errorf("error resolving audio URL: %v", err)
					}
				}
			}
			quality := 0
			groupID := ""
			if idx := strings.Index(line, `GROUP-ID="`); idx != -1 {
				groupStart := idx + 10
				if groupEnd := strings.Index(line[groupStart:], `"`); groupEnd != -1 {
					groupID = strings.ToLower(line[groupStart : groupStart+groupEnd])
					log.Debug().Str("op", "live-stream/helpers").Msgf("Found GROUP-ID: %s", groupID)
					if strings.Contains(groupID, "high") {
						quality = 3
					} else if strings.Contains(groupID, "medium") {
						quality = 2
					} else if strings.Contains(groupID, "low") {
						quality = 1
					}
				}
			}
			if quality == 0 {
				if idx := strings.Index(line, `NAME="`); idx != -1 {
					nameStart := idx + 6
					if nameEnd := strings.Index(line[nameStart:], `"`); nameEnd != -1 {
						name := strings.ToLower(line[nameStart : nameStart+nameEnd])
						log.Debug().Str("op", "live-stream/helpers").Msgf("Found NAME: %s", name)
						if strings.Contains(name, "high") {
							quality = 3
						} else if strings.Contains(name, "medium") {
							quality = 2
						} else if strings.Contains(name, "low") {
							quality = 1
						}
					}
				}
			}
			if audioURL != "" {
				audioTracks = append(audioTracks, audioTrack{url: audioURL, quality: quality})
			}
			continue
		}
		if strings.HasPrefix(line, "#") && !strings.Contains(line, "#EXT-X-STREAM-INF") {
			continue
		}
		if strings.Contains(line, "#EXT-X-STREAM-INF") {
			isMasterPlaylist = true
			currentBandwidth = 0
			if idx := strings.Index(line, "BANDWIDTH="); idx != -1 {
				bandwidthStart := idx + 10
				bandwidthEnd := bandwidthStart
				for bandwidthEnd < len(line) && line[bandwidthEnd] >= '0' && line[bandwidthEnd] <= '9' {
					bandwidthEnd++
				}
				if bandwidthEnd > bandwidthStart {
					fmt.Sscanf(line[bandwidthStart:bandwidthEnd], "%d", &currentBandwidth)
				}
			}
			continue
		}
		if !strings.HasPrefix(line, "#") {
			segmentURL, err := resolveURL(baseURL, line)
			if err != nil {
				return nil, fmt.Errorf("error resolving URL: %v", err)
			}
			if isMasterPlaylist {
				masterPlaylistURLs = append(masterPlaylistURLs, segmentURL)
				masterPlaylistBandwidths = append(masterPlaylistBandwidths, currentBandwidth)
				currentBandwidth = 0
			} else {
				segmentURLs = append(segmentURLs, segmentURL)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning m3u8 content: %v", err)
	}

	if isMasterPlaylist && len(masterPlaylistURLs) > 0 {
		bestVariantIdx := 0
		maxBandwidth := 0
		for i, bandwidth := range masterPlaylistBandwidths {
			if bandwidth > maxBandwidth {
				maxBandwidth = bandwidth
				bestVariantIdx = i
			}
		}
		log.Debug().Str("op", "live-stream/helpers").Msgf("Detected master playlist with %d video variants and %d audio tracks, selecting variant with bandwidth %d", len(masterPlaylistURLs), len(audioTracks), maxBandwidth)

		videoContent, err := getM3U8Contents(masterPlaylistURLs[bestVariantIdx], client)
		if err != nil {
			return nil, fmt.Errorf("error fetching video sub-playlist: %v", err)
		}
		videoInfo, err := parseM3U8Content(videoContent, masterPlaylistURLs[bestVariantIdx], client)
		if err != nil {
			return nil, fmt.Errorf("error parsing video sub-playlist: %v", err)
		}

		if len(audioTracks) > 0 {
			bestAudioIdx := 0
			maxQuality := 0
			for i, track := range audioTracks {
				log.Debug().Str("op", "live-stream/helpers").Msgf("Audio track %d: url=%s, quality=%d", i, track.url, track.quality)
				if track.quality > maxQuality {
					maxQuality = track.quality
					bestAudioIdx = i
				}
			}
			log.Debug().Str("op", "live-stream/helpers").Msgf("Selected audio track %d with quality %d", bestAudioIdx, maxQuality)
			audioContent, err := getM3U8Contents(audioTracks[bestAudioIdx].url, client)
			if err != nil {
				return nil, fmt.Errorf("error fetching audio sub-playlist: %v", err)
			}
			audioInfo, err := parseM3U8Content(audioContent, audioTracks[bestAudioIdx].url, client)
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
