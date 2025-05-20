package danzohttp

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tanq16/danzo/internal/utils"
)

func PerformMultiDownload(config utils.DownloadConfig, client *http.Client, fileSize int64, progressCh chan<- int64) error {
	job := utils.DownloadJob{
		Config:    config,
		FileSize:  fileSize,
		StartTime: time.Now(),
	}
	tempDir := filepath.Join(filepath.Dir(config.OutputPath), ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}

	// Setup chunks
	mutex := &sync.Mutex{}
	chunkSize := fileSize / int64(config.Connections)
	var currentPosition int64 = 0
	for i := range config.Connections {
		startByte := currentPosition
		endByte := startByte + chunkSize - 1
		if i == config.Connections-1 {
			endByte = fileSize - 1
		}
		if endByte >= fileSize {
			endByte = fileSize - 1
		}
		if endByte >= startByte {
			job.Chunks = append(job.Chunks, utils.DownloadChunk{
				ID:        i,
				StartByte: startByte,
				EndByte:   endByte,
			})
		}
		currentPosition = endByte + 1
	}

	// Start connection goroutines
	var wg sync.WaitGroup
	for i := range job.Chunks {
		wg.Add(1)
		go chunkedDownload(&job, &job.Chunks[i], client, &wg, progressCh, mutex)
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(progressCh)
	allCompleted := true
	var incompleteChunks []int
	for i, chunk := range job.Chunks {
		if !chunk.Completed {
			allCompleted = false
			incompleteChunks = append(incompleteChunks, i)
		}
	}
	if !allCompleted {
		return fmt.Errorf("download incomplete: %d chunks failed: %v", len(incompleteChunks), incompleteChunks)
	}

	// Assemble the file
	err := assembleFile(job)
	if err != nil {
		return fmt.Errorf("error assembling file: %v", err)
	}
	return nil
}

func assembleFile(job utils.DownloadJob) error {
	allChunksCompleted := true
	for _, chunk := range job.Chunks {
		if !chunk.Completed {
			allChunksCompleted = false
		}
	}
	if !allChunksCompleted {
		return errors.New("not all chunks were completed successfully")
	}
	tempFiles := make([]string, len(job.TempFiles))
	copy(tempFiles, job.TempFiles)
	sort.Slice(tempFiles, func(i, j int) bool {
		idI, errI := extractChunkID(tempFiles[i])
		idJ, errJ := extractChunkID(tempFiles[j])
		if errI != nil || errJ != nil {
			return tempFiles[i] < tempFiles[j] // Fallback to string comparison
		}
		return idI < idJ
	})
	// for _, file := range tempFiles {
	// 	chunkID, _ := extractChunkID(file)
	// }
	destFile, err := os.Create(job.Config.OutputPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	var totalWritten int64 = 0
	for _, tempFilePath := range tempFiles {
		tempFile, err := os.Open(tempFilePath)
		if err != nil {
			return fmt.Errorf("error opening chunk file %s: %v", tempFilePath, err)
		}
		fileInfo, err := tempFile.Stat()
		if err != nil {
			tempFile.Close()
			return fmt.Errorf("error getting chunk file info: %v", err)
		}
		chunkSize := fileInfo.Size()
		written, err := io.Copy(destFile, tempFile)
		tempFile.Close()
		if err != nil {
			return fmt.Errorf("error copying chunk data: %v", err)
		}
		if written != chunkSize {
			return fmt.Errorf("error: wrote %d bytes but chunk size is %d", written, chunkSize)
		}
		totalWritten += written
	}
	if totalWritten != job.FileSize {
		return fmt.Errorf("error: total written bytes (%d) doesn't match expected file size (%d)", totalWritten, job.FileSize)
	}

	// Cleanup temporary files
	for _, tempFilePath := range tempFiles {
		os.Remove(tempFilePath)
	}
	return nil
}

func extractChunkID(filename string) (int, error) {
	matches := utils.ChunkIDRegex.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return -1, fmt.Errorf("could not extract chunk ID from %s", filename)
	}
	return strconv.Atoi(matches[1])
}
