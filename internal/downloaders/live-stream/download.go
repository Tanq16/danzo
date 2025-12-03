package m3u8

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/danzo/internal/utils"
)

func (d *M3U8Downloader) Download(job *utils.DanzoJob) error {
	tempDir := job.Metadata["tempDir"].(string)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	var downloadErr error
	defer func() {
		if downloadErr == nil {
			os.RemoveAll(tempDir)
		} else {
			log.Warn().Str("op", "live-stream/download").Msgf("Preserving segments in %s due to error", tempDir)
		}
	}()

	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	log.Debug().Str("op", "live-stream/download").Msgf("Fetching manifest from %s", job.URL)
	manifestContent, err := getM3U8Contents(job.URL, client)
	if err != nil {
		downloadErr = fmt.Errorf("error fetching manifest: %v", err)
		return downloadErr
	}
	m3u8Info, err := parseM3U8Content(manifestContent, job.URL, client)
	if err != nil {
		downloadErr = fmt.Errorf("error processing manifest: %v", err)
		return downloadErr
	}
	segmentURLs := m3u8Info.SegmentURLs
	if len(segmentURLs) == 0 {
		downloadErr = fmt.Errorf("no segments found in manifest")
		return downloadErr
	}
	log.Info().Str("op", "live-stream/download").Msgf("Found %d segments to download", len(segmentURLs))

	totalSize, segmentSizes, err := calculateTotalSize(segmentURLs, job.Connections, client)
	if err != nil {
		// Fallback to estimate
		log.Warn().Str("op", "live-stream/download").Msgf("Could not calculate total size accurately: %v. Using estimate.", err)
		totalSize = int64(len(segmentURLs)) * 1024 * 1024 // 1MB per segment estimate
		segmentSizes = make([]int64, len(segmentURLs))
		for i := range segmentSizes {
			segmentSizes[i] = 1024 * 1024
		}
	}
	job.Metadata["totalSize"] = totalSize
	job.Metadata["segmentSizes"] = segmentSizes
	log.Debug().Str("op", "live-stream/download").Msgf("Total estimated size: %s", utils.FormatBytes(uint64(totalSize)))

	// Detect fMP4 format
	isFMP4 := detectFMP4Format(job.URL, segmentURLs)
	if isFMP4 {
		log.Debug().Str("op", "live-stream/download").Msg("Detected fMP4 format segments")
	}

	log.Info().Str("op", "live-stream/download").Msg("Starting parallel download of segments")
	segmentFiles, err := downloadSegmentsParallel(segmentURLs, tempDir, job.Connections, client, job.ProgressFunc, totalSize, isFMP4)
	if err != nil {
		downloadErr = fmt.Errorf("error downloading segments: %v", err)
		return downloadErr
	}
	log.Info().Str("op", "live-stream/download").Msg("All segments downloaded, merging with ffmpeg")
	if err := mergeSegments(segmentFiles, job.OutputPath, isFMP4, m3u8Info.InitSegment, tempDir, client); err != nil {
		downloadErr = fmt.Errorf("error merging segments: %v", err)
		return downloadErr
	}
	log.Info().Str("op", "live-stream/download").Msg("Segments merged successfully")
	return nil
}

func detectFMP4Format(manifestURL string, segmentURLs []string) bool {
	if strings.Contains(manifestURL, "sf=fmp4") ||
		strings.Contains(manifestURL, "/fmp4/") ||
		strings.Contains(manifestURL, "frag") {
		return true
	}
	if len(segmentURLs) > 0 {
		firstSegment := segmentURLs[0]
		if strings.Contains(firstSegment, "/fmp4/") ||
			strings.Contains(firstSegment, ".m4s") ||
			strings.Contains(firstSegment, "frag") ||
			strings.Contains(firstSegment, ".mp4") {
			return true
		}
	}
	return false
}

func downloadSegmentsParallel(segmentURLs []string, outputDir string, numWorkers int, client *utils.DanzoHTTPClient, progressFunc func(int64, int64), totalSize int64, isFMP4 bool) ([]string, error) {
	var downloadedFiles []string
	var mu sync.Mutex
	var totalDownloaded int64
	var downloadErr error
	type segmentJob struct {
		index int
		url   string
	}
	jobCh := make(chan segmentJob, len(segmentURLs))
	for i, url := range segmentURLs {
		jobCh <- segmentJob{index: i, url: url}
	}
	close(jobCh)
	downloadedFiles = make([]string, len(segmentURLs))
	ext := ".ts"
	if isFMP4 {
		ext = ".m4s"
	}

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				outputPath := filepath.Join(outputDir, fmt.Sprintf("segment_%04d%s", job.index, ext))
				size, err := downloadSegment(job.url, outputPath, client)
				if err != nil {
					mu.Lock()
					if downloadErr == nil {
						downloadErr = fmt.Errorf("error downloading segment %d: %v", job.index, err)
					}
					mu.Unlock()
					return
				}
				mu.Lock()
				downloadedFiles[job.index] = outputPath
				mu.Unlock()
				downloaded := atomic.AddInt64(&totalDownloaded, size)
				if progressFunc != nil {
					progressFunc(downloaded, totalSize)
				}
			}
		}()
	}

	wg.Wait()
	if downloadErr != nil {
		return nil, downloadErr
	}
	return downloadedFiles, nil
}

func mergeSegments(segmentFiles []string, outputPath string, isFMP4 bool, initSegment string, tempDir string, client *utils.DanzoHTTPClient) error {
	if isFMP4 {
		return mergeFMP4Segments(segmentFiles, outputPath, initSegment, tempDir, client)
	}
	return mergeTSSegments(segmentFiles, outputPath)
}

func mergeTSSegments(segmentFiles []string, outputPath string) error {
	tempListFile := filepath.Join(filepath.Dir(outputPath), ".segment_list.txt")
	f, err := os.Create(tempListFile)
	if err != nil {
		return fmt.Errorf("error creating segment list file: %v", err)
	}
	defer os.Remove(tempListFile)
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
		"-y",
		outputPath,
	)
	log.Debug().Str("op", "live-stream/download").Msgf("Executing ffmpeg command: %s", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Str("op", "live-stream/download").Msgf("FFmpeg output:\n%s", string(output))
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}
	return nil
}

func mergeFMP4Segments(segmentFiles []string, outputPath string, initSegment string, tempDir string, client *utils.DanzoHTTPClient) error {
	tempConcatFile := filepath.Join(filepath.Dir(outputPath), ".concat_temp.m4s")
	defer os.Remove(tempConcatFile)
	log.Debug().Str("op", "live-stream/download").Msgf("Concatenating %d fMP4 segments", len(segmentFiles))
	outFile, err := os.Create(tempConcatFile)
	if err != nil {
		return fmt.Errorf("error creating temp concat file: %v", err)
	}
	if initSegment != "" {
		log.Debug().Str("op", "live-stream/download").Msg("Downloading init segment")
		initPath := filepath.Join(tempDir, "init.mp4")
		_, err := downloadSegment(initSegment, initPath, client)
		if err != nil {
			outFile.Close()
			return fmt.Errorf("error downloading init segment: %v", err)
		}
		initData, err := os.ReadFile(initPath)
		if err != nil {
			outFile.Close()
			return fmt.Errorf("error reading init segment: %v", err)
		}
		if _, err := outFile.Write(initData); err != nil {
			outFile.Close()
			return fmt.Errorf("error writing init segment: %v", err)
		}
		log.Debug().Str("op", "live-stream/download").Msgf("Init segment written (%d bytes)", len(initData))
	}
	for i, segmentFile := range segmentFiles {
		data, err := os.ReadFile(segmentFile)
		if err != nil {
			outFile.Close()
			return fmt.Errorf("error reading segment %d: %v", i, err)
		}
		if _, err := outFile.Write(data); err != nil {
			outFile.Close()
			return fmt.Errorf("error writing segment %d: %v", i, err)
		}
	}
	outFile.Close()

	log.Debug().Str("op", "live-stream/download").Msg("Remuxing concatenated fMP4 segments")
	cmd := exec.Command(
		"ffmpeg",
		"-i", tempConcatFile,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)
	log.Debug().Str("op", "live-stream/download").Msgf("Executing ffmpeg command: %s", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Str("op", "live-stream/download").Msgf("FFmpeg failed with error: %v", err)
		log.Error().Str("op", "live-stream/download").Msgf("FFmpeg output:\n%s", string(output))
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}
	return nil
}
