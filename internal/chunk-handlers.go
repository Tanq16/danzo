package internal

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
)

var ErrRangeRequestsNotSupported = errors.New("server doesn't support range requests")

func chunkedDownload(job *downloadJob, chunk *downloadChunk, client *http.Client, wg *sync.WaitGroup, progressCh chan<- int64, mutex *sync.Mutex) {
	log := GetLogger("chunk").With().Int("chunkId", chunk.ID).Logger()
	defer wg.Done()
	tempDir := filepath.Join(filepath.Dir(job.Config.OutputPath), ".danzo-temp")
	tempFileName := filepath.Join(tempDir, fmt.Sprintf("%s.part%d", filepath.Base(job.Config.OutputPath), chunk.ID))
	expectedSize := chunk.EndByte - chunk.StartByte + 1
	resumeOffset := int64(0)
	if fileInfo, err := os.Stat(tempFileName); err == nil {
		resumeOffset = fileInfo.Size()
		if resumeOffset == expectedSize {
			log.Debug().Str("file", filepath.Base(tempFileName)).Int64("size", chunk.Downloaded).Msg("Chunk already downloaded, skipping")
			mutex.Lock()
			job.TempFiles = append(job.TempFiles, tempFileName)
			mutex.Unlock()
			chunk.Downloaded = resumeOffset
			chunk.Completed = true
			progressCh <- resumeOffset
			return
		} else if resumeOffset > 0 && resumeOffset < expectedSize { // Resume incomplete chunk
			log.Debug().Str("file", filepath.Base(tempFileName)).Int64("size", resumeOffset).Int64("total", expectedSize).Msg("Resuming incomplete chunk")
		} else if chunk.Downloaded > 0 {
			log.Warn().Str("file", filepath.Base(tempFileName)).Int64("size", resumeOffset).Int64("expected", expectedSize).Msg("Temporary file larger than expected, removing and redownloading")
			os.Remove(tempFileName)
			resumeOffset = 0
		}
	}
	maxRetries := 5
	for retry := range maxRetries {
		if retry > 0 {
			log.Debug().Int("attempt", retry+1).Int("maxRetries", maxRetries).Msg("Retrying download of chunk")
			time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond) // Backoff
			if fileInfo, err := os.Stat(tempFileName); err == nil {
				currentSize := fileInfo.Size()
				if currentSize != chunk.Downloaded && chunk.Downloaded > 0 {
					log.Debug().Int64("fileSize", currentSize).Int64("downloaded", chunk.Downloaded).Msg("Resetting chunk download")
					os.Remove(tempFileName)
					progressCh <- -chunk.Downloaded // Subtract from progress
					chunk.Downloaded = 0
					resumeOffset = 0
				}
			}
		}
		if err := downloadSingleChunk(job, chunk, client, tempFileName, progressCh, resumeOffset); err != nil {
			log.Error().Err(err).Int("attempt", retry+1).Msg("Error downloading chunk")
			continue
		}
		// On success
		mutex.Lock()
		job.TempFiles = append(job.TempFiles, tempFileName)
		mutex.Unlock()
		chunk.Completed = true
		return
	}
	log.Error().Int("maxRetries", maxRetries).Msg("Failed to download chunk after multiple attempts")
}

func downloadSingleChunk(job *downloadJob, chunk *downloadChunk, client *http.Client, tempFileName string, progressCh chan<- int64, resumeOffset int64) error {
	log := GetLogger("download").With().Int("chunkId", chunk.ID).Logger()
	flag := os.O_WRONLY | os.O_CREATE
	if resumeOffset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	tempFile, err := os.OpenFile(tempFileName, flag, 0644)
	if err != nil {
		return fmt.Errorf("error opening temp file: %v", err)
	}
	defer tempFile.Close()

	startByte := chunk.StartByte + resumeOffset
	rangeHeader := fmt.Sprintf("bytes=%d-%d", startByte, chunk.EndByte)
	req, err := http.NewRequest("GET", job.Config.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", rangeHeader)
	req.Header.Set("User-Agent", job.Config.UserAgent)
	req.Header.Set("Connection", "keep-alive")
	log.Debug().Str("range", rangeHeader).Msg("Sending range request")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	contentRange := resp.Header.Get("Content-Range")
	if contentRange == "" {
		return errors.New("missing Content-Range header")
	}

	if resumeOffset > 0 {
		progressCh <- resumeOffset
		chunk.Downloaded = resumeOffset
	}
	remainingBytes := chunk.EndByte - startByte + 1
	buffer := make([]byte, bufferSize)
	newBytes := int64(0)
	for {
		bytesRead, err := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := tempFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				return writeErr
			}
			newBytes += int64(bytesRead)
			chunk.Downloaded += int64(bytesRead)
			progressCh <- int64(bytesRead)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if newBytes != remainingBytes {
		log.Error().Int64("remainingBytes", remainingBytes).Int64("newBytes", newBytes).Int64("totalDownloaded", chunk.Downloaded).Int64("resumeOffset", resumeOffset).Msg("Size mismatch on chunk download")
		return fmt.Errorf("size mismatch: expected %d remaining bytes, got %d bytes this session", remainingBytes, newBytes)
	}
	totalExpectedSize := chunk.EndByte - chunk.StartByte + 1
	if chunk.Downloaded != totalExpectedSize {
		return fmt.Errorf("total size mismatch: expected %d total bytes, got %d bytes", totalExpectedSize, chunk.Downloaded)
	}
	log.Debug().Int64("totalExpectedSize", totalExpectedSize).Int64("remainingBytes", remainingBytes).Int64("downloadedThisSession", newBytes).Int64("totalDownloaded", chunk.Downloaded).Msg("Chunk download completed")
	return nil
}

func extractChunkID(filename string) (int, error) {
	matches := chunkIDRegex.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return -1, fmt.Errorf("could not extract chunk ID from %s", filename)
	}
	return strconv.Atoi(matches[1])
}

func assembleFile(job downloadJob) error {
	log := GetLogger("assembler")
	allChunksCompleted := true
	for i, chunk := range job.Chunks {
		if !chunk.Completed {
			log.Warn().Int("chunkId", i).Msg("Chunk was not completed")
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
			log.Error().Err(errors.Join(errI, errJ)).Str("file1", tempFiles[i]).Str("file2", tempFiles[j]).Msg("Error sorting chunk files")
			return tempFiles[i] < tempFiles[j] // Fallback to string comparison
		}
		return idI < idJ
	})
	log.Debug().Int("count", len(tempFiles)).Msg("Assembling chunks in order")
	for i, file := range tempFiles {
		chunkID, _ := extractChunkID(file)
		log.Debug().Int("index", i).Int("chunkId", chunkID).Str("file", file).Msg("Chunk order")
	}
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
	log.Debug().Int64("totalBytes", totalWritten).Str("outputFile", job.Config.OutputPath).Msg("File assembly completed")
	return nil
}
