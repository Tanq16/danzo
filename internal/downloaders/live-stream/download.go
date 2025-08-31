package m3u8

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/tanq16/danzo/internal/utils"
)

func (d *M3U8Downloader) Download(job *utils.DanzoJob) error {
	tempDir := job.Metadata["tempDir"].(string)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	client := utils.NewDanzoHTTPClient(job.HTTPClientConfig)
	manifestContent, err := getM3U8Contents(job.URL, client)
	if err != nil {
		return fmt.Errorf("error fetching manifest: %v", err)
	}
	segmentURLs, err := processM3U8Content(manifestContent, job.URL, client)
	if err != nil {
		return fmt.Errorf("error processing manifest: %v", err)
	}
	if len(segmentURLs) == 0 {
		return fmt.Errorf("no segments found in manifest")
	}

	totalSize, segmentSizes, err := calculateTotalSize(segmentURLs, job.Connections, client)
	if err != nil {
		// Fallback to estimate
		totalSize = int64(len(segmentURLs)) * 1024 * 1024 // 1MB per segment estimate
		segmentSizes = make([]int64, len(segmentURLs))
		for i := range segmentSizes {
			segmentSizes[i] = 1024 * 1024
		}
	}
	job.Metadata["totalSize"] = totalSize
	job.Metadata["segmentSizes"] = segmentSizes

	segmentFiles, err := downloadSegmentsParallel(segmentURLs, tempDir, job.Connections, client, job.ProgressFunc, totalSize)
	if err != nil {
		return fmt.Errorf("error downloading segments: %v", err)
	}
	if err := mergeSegments(segmentFiles, job.OutputPath); err != nil {
		return fmt.Errorf("error merging segments: %v", err)
	}
	return nil
}

func downloadSegmentsParallel(segmentURLs []string, outputDir string, numWorkers int, client *utils.DanzoHTTPClient, progressFunc func(int64, int64), totalSize int64) ([]string, error) {
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

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				outputPath := filepath.Join(outputDir, fmt.Sprintf("segment_%04d.ts", job.index))
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

func mergeSegments(segmentFiles []string, outputPath string) error {
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}
	return nil
}
