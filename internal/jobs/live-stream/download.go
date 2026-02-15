package m3u8

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tanq16/danzo/internal/utils"
)

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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}
	return nil
}

func mergeFMP4Segments(segmentFiles []string, outputPath string, initSegment string, tempDir string, client *utils.DanzoHTTPClient) error {
	tempConcatFile := filepath.Join(filepath.Dir(outputPath), ".concat_temp.m4s")
	defer os.Remove(tempConcatFile)
	outFile, err := os.Create(tempConcatFile)
	if err != nil {
		return fmt.Errorf("error creating temp concat file: %v", err)
	}
	if initSegment != "" {
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

	cmd := exec.Command(
		"ffmpeg",
		"-i", tempConcatFile,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}
	return nil
}
