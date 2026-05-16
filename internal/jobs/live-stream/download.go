package m3u8

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tanq16/danzo/utils"
	"golang.org/x/sync/errgroup"
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

func downloadSegmentsParallel(ctx context.Context, segmentURLs []string, outputDir string, numWorkers int, client *utils.DanzoHTTPClient, progressFunc func(int64, int64), totalSize int64, isFMP4 bool) ([]string, error) {
	downloadedFiles := make([]string, len(segmentURLs))
	ext := ".ts"
	if isFMP4 {
		ext = ".m4s"
	}

	g, ctx := errgroup.WithContext(ctx)
	if numWorkers < 1 {
		numWorkers = 1
	}
	g.SetLimit(numWorkers)
	for i, segmentURL := range segmentURLs {
		g.Go(func() error {
			outputPath := filepath.Join(outputDir, fmt.Sprintf("segment_%04d%s", i, ext))
			size, err := downloadSegment(ctx, segmentURL, outputPath, client)
			if err != nil {
				return fmt.Errorf("error downloading segment %d: %v", i, err)
			}
			downloadedFiles[i] = outputPath
			if progressFunc != nil {
				progressFunc(size, totalSize)
			}
			return nil
		})
	}
	return downloadedFiles, g.Wait()
}

func mergeSegments(ctx context.Context, segmentFiles []string, outputPath string, isFMP4 bool, initSegment string, tempDir string, client *utils.DanzoHTTPClient) error {
	if isFMP4 {
		return mergeFMP4Segments(ctx, segmentFiles, outputPath, initSegment, tempDir, client)
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

func mergeFMP4Segments(ctx context.Context, segmentFiles []string, outputPath string, initSegment string, tempDir string, client *utils.DanzoHTTPClient) error {
	tempConcatFile := filepath.Join(filepath.Dir(outputPath), ".concat_temp.m4s")
	defer os.Remove(tempConcatFile)
	outFile, err := os.Create(tempConcatFile)
	if err != nil {
		return fmt.Errorf("error creating temp concat file: %v", err)
	}
	if initSegment != "" {
		initPath := filepath.Join(tempDir, "init.mp4")
		_, err := downloadSegment(ctx, initSegment, initPath, client)
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
