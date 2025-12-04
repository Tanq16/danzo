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

	if len(m3u8Info.VideoSegmentURLs) == 0 {
		downloadErr = fmt.Errorf("no video segments found in manifest")
		return downloadErr
	}

	if m3u8Info.HasSeparateAudio {
		log.Info().Str("op", "live-stream/download").Msgf("Found %d video segments and %d audio segments", len(m3u8Info.VideoSegmentURLs), len(m3u8Info.AudioSegmentURLs))
		if err := downloadAndMergeSeparateStreams(m3u8Info, job, tempDir, client); err != nil {
			downloadErr = err
			return downloadErr
		}
	} else {
		log.Info().Str("op", "live-stream/download").Msgf("Found %d segments to download", len(m3u8Info.VideoSegmentURLs))
		if err := downloadAndMergeSingleStream(m3u8Info, job, tempDir, client); err != nil {
			downloadErr = err
			return downloadErr
		}
	}

	log.Info().Str("op", "live-stream/download").Msg("Download completed successfully")
	return nil
}

func downloadAndMergeSingleStream(m3u8Info *M3U8Info, job *utils.DanzoJob, tempDir string, client *utils.DanzoHTTPClient) error {
	segmentURLs := m3u8Info.VideoSegmentURLs
	totalSize, segmentSizes, err := calculateTotalSize(segmentURLs, job.Connections, client)
	if err != nil {
		log.Warn().Str("op", "live-stream/download").Msgf("Could not calculate total size accurately: %v. Using estimate.", err)
		totalSize = int64(len(segmentURLs)) * 1024 * 1024
		segmentSizes = make([]int64, len(segmentURLs))
		for i := range segmentSizes {
			segmentSizes[i] = 1024 * 1024
		}
	}
	job.Metadata["totalSize"] = totalSize
	job.Metadata["segmentSizes"] = segmentSizes
	log.Debug().Str("op", "live-stream/download").Msgf("Total estimated size: %s", utils.FormatBytes(uint64(totalSize)))

	isFMP4 := detectFMP4Format(job.URL, segmentURLs)
	if isFMP4 {
		log.Debug().Str("op", "live-stream/download").Msg("Detected fMP4 format segments")
	}

	var totalDownloaded int64
	wrappedProgressFunc := func(incrementalSize, _ int64) {
		newTotal := atomic.AddInt64(&totalDownloaded, incrementalSize)
		if job.ProgressFunc != nil {
			job.ProgressFunc(newTotal, totalSize)
		}
	}

	log.Info().Str("op", "live-stream/download").Msg("Starting parallel download of segments")
	segmentFiles, err := downloadSegmentsParallel(segmentURLs, tempDir, job.Connections, client, wrappedProgressFunc, totalSize, isFMP4)
	if err != nil {
		return fmt.Errorf("error downloading segments: %v", err)
	}
	log.Info().Str("op", "live-stream/download").Msg("All segments downloaded, merging with ffmpeg")
	if err := mergeSegments(segmentFiles, job.OutputPath, isFMP4, m3u8Info.VideoInitSegment, tempDir, client); err != nil {
		return fmt.Errorf("error merging segments: %v", err)
	}
	return nil
}

func downloadAndMergeSeparateStreams(m3u8Info *M3U8Info, job *utils.DanzoJob, tempDir string, client *utils.DanzoHTTPClient) error {
	videoDir := filepath.Join(tempDir, "video")
	audioDir := filepath.Join(tempDir, "audio")
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		return fmt.Errorf("error creating video directory: %v", err)
	}
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		return fmt.Errorf("error creating audio directory: %v", err)
	}

	videoSegmentURLs := m3u8Info.VideoSegmentURLs
	audioSegmentURLs := m3u8Info.AudioSegmentURLs

	totalVideoSize, _, err := calculateTotalSize(videoSegmentURLs, job.Connections, client)
	if err != nil {
		log.Warn().Str("op", "live-stream/download").Msg("Could not calculate video size, using estimate")
		totalVideoSize = int64(len(videoSegmentURLs)) * 1024 * 1024
	}
	totalAudioSize, _, err := calculateTotalSize(audioSegmentURLs, job.Connections, client)
	if err != nil {
		log.Warn().Str("op", "live-stream/download").Msg("Could not calculate audio size, using estimate")
		totalAudioSize = int64(len(audioSegmentURLs)) * 512 * 1024
	}
	totalSize := totalVideoSize + totalAudioSize
	log.Debug().Str("op", "live-stream/download").Msgf("Total estimated size: %s (video: %s, audio: %s)",
		utils.FormatBytes(uint64(totalSize)), utils.FormatBytes(uint64(totalVideoSize)), utils.FormatBytes(uint64(totalAudioSize)))

	isVideoFMP4 := detectFMP4Format(job.URL, videoSegmentURLs)
	isAudioFMP4 := detectFMP4Format(job.URL, audioSegmentURLs)

	var totalDownloaded int64
	wrappedProgressFunc := func(incrementalDownloaded, _ int64) {
		newTotal := atomic.AddInt64(&totalDownloaded, incrementalDownloaded)
		if job.ProgressFunc != nil {
			job.ProgressFunc(newTotal, totalSize)
		}
	}

	log.Info().Str("op", "live-stream/download").Msg("Starting parallel download of video segments")
	videoFiles, videoErr := downloadSegmentsParallel(videoSegmentURLs, videoDir, job.Connections, client, wrappedProgressFunc, totalVideoSize, isVideoFMP4)

	log.Info().Str("op", "live-stream/download").Msg("Starting parallel download of audio segments")
	audioFiles, audioErr := downloadSegmentsParallel(audioSegmentURLs, audioDir, job.Connections, client, wrappedProgressFunc, totalAudioSize, isAudioFMP4)

	if videoErr != nil && audioErr != nil {
		return fmt.Errorf("both video and audio downloads failed - video: %v, audio: %v", videoErr, audioErr)
	}

	tempVideoPath := filepath.Join(tempDir, "video_temp.mp4")
	tempAudioPath := filepath.Join(tempDir, "audio_temp.m4a")

	var finalErr error

	if videoErr == nil {
		log.Info().Str("op", "live-stream/download").Msg("Merging video segments")
		if err := mergeSegments(videoFiles, tempVideoPath, isVideoFMP4, m3u8Info.VideoInitSegment, videoDir, client); err != nil {
			return fmt.Errorf("error merging video segments: %v", err)
		}
	}

	if audioErr == nil {
		log.Info().Str("op", "live-stream/download").Msg("Merging audio segments")
		if err := mergeSegments(audioFiles, tempAudioPath, isAudioFMP4, m3u8Info.AudioInitSegment, audioDir, client); err != nil {
			if videoErr == nil {
				if err := os.Rename(tempVideoPath, job.OutputPath); err != nil {
					return fmt.Errorf("error saving video-only output: %v", err)
				}
				return fmt.Errorf("audio merge failed, saved video-only: %v", err)
			}
			return fmt.Errorf("error merging audio segments: %v", err)
		}
	}

	if videoErr == nil && audioErr == nil {
		log.Info().Str("op", "live-stream/download").Msg("Merging video and audio streams")
		if err := mergeVideoAndAudio(tempVideoPath, tempAudioPath, job.OutputPath); err != nil {
			return fmt.Errorf("error merging video and audio: %v", err)
		}
	} else if videoErr == nil && audioErr != nil {
		log.Warn().Str("op", "live-stream/download").Msg("Audio download failed, saving video-only")
		if err := os.Rename(tempVideoPath, job.OutputPath); err != nil {
			return fmt.Errorf("error saving video-only output: %v", err)
		}
		finalErr = fmt.Errorf("audio download failed: %v", audioErr)
	} else if audioErr == nil && videoErr != nil {
		log.Warn().Str("op", "live-stream/download").Msg("Video download failed, saving audio-only")
		if err := os.Rename(tempAudioPath, job.OutputPath); err != nil {
			return fmt.Errorf("error saving audio-only output: %v", err)
		}
		finalErr = fmt.Errorf("video download failed: %v", videoErr)
	}

	return finalErr
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
				if progressFunc != nil {
					progressFunc(size, totalSize)
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
		absPath, err := filepath.Abs(file)
		if err != nil {
			absPath = file
		}
		fmt.Fprintf(f, "file '%s'\n", absPath)
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

func mergeVideoAndAudio(videoPath, audioPath, outputPath string) error {
	cmd := exec.Command(
		"ffmpeg",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-bsf:a:0", "aac_adtstoasc",
		"-movflags", "+faststart",
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
