package m3u8

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tanq16/danzo/internal/utils"
)

func parseM3U8URL(rawURL string) (string, error) {
	actualURL := rawURL[7:]
	_, err := url.Parse(actualURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL after m3u8:// prefix: %v", err)
	}
	return actualURL, nil
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
	return string(content), nil
}

func processM3U8Content(content, manifestURL string, client *utils.DanzoHTTPClient) ([]string, error) {
	baseURL, err := url.Parse(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest URL: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	var segmentURLs []string
	var masterPlaylistURLs []string
	var isMasterPlaylist bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments (except EXT-X-STREAM-INF)
		if line == "" || (strings.HasPrefix(line, "#") && !strings.Contains(line, "#EXT-X-STREAM-INF")) {
			continue
		}
		if strings.Contains(line, "#EXT-X-STREAM-INF") {
			isMasterPlaylist = true
			continue
		}
		// If not a comment, consider it a URL
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

	// For a master playlist, fetch the first playlist for highest quality
	if isMasterPlaylist && len(masterPlaylistURLs) > 0 {
		subContent, err := getM3U8Contents(masterPlaylistURLs[0], client)
		if err != nil {
			return nil, fmt.Errorf("error fetching sub-playlist: %v", err)
		}
		// Recursively process the sub-playlist
		return processM3U8Content(subContent, masterPlaylistURLs[0], client)
	}
	return segmentURLs, nil
}

func resolveURL(baseURL *url.URL, urlStr string) (string, error) {
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		return urlStr, nil // Already an absolute URL
	}
	relURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	absURL := baseURL.ResolveReference(relURL)
	return absURL.String(), nil
}

func downloadM3U8Segments(segmentURLs []string, outputDir string, client *utils.DanzoHTTPClient, streamCh chan<- string) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating output directory: %v", err)
	}
	var downloadedFiles []string
	for i, segmentURL := range segmentURLs {
		outputPath := filepath.Join(outputDir, fmt.Sprintf("segment_%04d.ts", i))
		req, err := http.NewRequest("GET", segmentURL, nil)
		if err != nil {
			return downloadedFiles, fmt.Errorf("error creating request for segment %d: %v", i, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return downloadedFiles, fmt.Errorf("error downloading segment %d: %v", i, err)
		}
		outFile, err := os.Create(outputPath)
		if err != nil {
			resp.Body.Close()
			return downloadedFiles, fmt.Errorf("error creating output file for segment %d: %v", i, err)
		}
		n, err := io.Copy(outFile, resp.Body)
		resp.Body.Close()
		outFile.Close()
		if err != nil {
			return downloadedFiles, fmt.Errorf("error writing segment %d: %v", i, err)
		}
		streamCh <- fmt.Sprintf("Downloaded segment %d of %d", i+1, len(segmentURLs))
		streamCh <- fmt.Sprintf("Downloaded %s", utils.FormatBytes(uint64(n)))
		downloadedFiles = append(downloadedFiles, outputPath)
	}
	return downloadedFiles, nil
}

func mergeSegments(segmentFiles []string, outputPath string) error {
	tempListFile := filepath.Join(filepath.Dir(outputPath), ".segment_list.txt")
	f, err := os.Create(tempListFile)
	if err != nil {
		return fmt.Errorf("error creating segment list file: %v", err)
	}
	for _, file := range segmentFiles {
		fmt.Fprintf(f, "file '%s'\n", file)
	}
	f.Close()
	cmd := exec.Command(
		"ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", tempListFile,
		"-c", "copy",
		"-y", // Overwrite output files without asking
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}
	os.Remove(tempListFile)
	return nil
}

func cleanupSegments(segmentFiles []string) {
	for _, file := range segmentFiles {
		os.Remove(file)
	}
	if len(segmentFiles) > 0 {
		dir := filepath.Dir(segmentFiles[0])
		os.Remove(dir) // only succeeds if empty
	}
}

func PerformM3U8Download(config utils.DownloadConfig, client *utils.DanzoHTTPClient, streamCh chan<- string) error {
	manifestURL, err := parseM3U8URL(config.URL)
	if err != nil {
		return fmt.Errorf("error parsing m3u8 URL: %v", err)
	}
	streamCh <- "Fetching M3U8 manifest"
	manifestContent, err := getM3U8Contents(manifestURL, client)
	if err != nil {
		return fmt.Errorf("error getting m3u8 manifest: %v", err)
	}
	streamCh <- "Processing M3U8 manifest..."
	segmentURLs, err := processM3U8Content(manifestContent, manifestURL, client)
	if err != nil {
		return fmt.Errorf("error processing m3u8 content: %v", err)
	}
	streamCh <- fmt.Sprintf("Found %d segments to download", len(segmentURLs))
	tempDir := filepath.Join(filepath.Dir(config.OutputPath), ".danzo-temp")
	streamCh <- "Downloading segments..."
	segmentFiles, err := downloadM3U8Segments(segmentURLs, tempDir, client, streamCh)
	if err != nil {
		return fmt.Errorf("error downloading segments: %v", err)
	}
	streamCh <- "Merging segments..."
	if err := mergeSegments(segmentFiles, config.OutputPath); err != nil {
		return fmt.Errorf("error merging segments: %v", err)
	}
	streamCh <- "Cleaning up temporary files..."
	cleanupSegments(segmentFiles)
	return nil
}
