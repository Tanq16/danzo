package danzohttp

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tanq16/danzo/utils"
)

func chunkedDownload(job *utils.DownloadJob, chunk *utils.DownloadChunk, client *http.Client, wg *sync.WaitGroup, progressCh chan<- int64, mutex *sync.Mutex) {
	log := utils.GetLogger("chunk").With().Int("chunkId", chunk.ID).Logger()
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
			log.Debug().Str("file", filepath.Base(tempFileName)).Int64("size", resumeOffset).Int64("expected", expectedSize).Msg("Temporary file larger than expected, removing and redownloading")
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
			log.Debug().Err(err).Int("attempt", retry+1).Msg("Error downloading chunk")
			continue
		}
		// On success
		mutex.Lock()
		job.TempFiles = append(job.TempFiles, tempFileName)
		mutex.Unlock()
		chunk.Completed = true
		return
	}
	log.Debug().Int("maxRetries", maxRetries).Msg("Failed to download chunk after multiple attempts")
}

func downloadSingleChunk(job *utils.DownloadJob, chunk *utils.DownloadChunk, client *http.Client, tempFileName string, progressCh chan<- int64, resumeOffset int64) error {
	log := utils.GetLogger("download").With().Int("chunkId", chunk.ID).Logger()
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
	buffer := make([]byte, utils.DefaultBufferSize)
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
		log.Debug().Int64("remainingBytes", remainingBytes).Int64("newBytes", newBytes).Int64("totalDownloaded", chunk.Downloaded).Int64("resumeOffset", resumeOffset).Msg("Size mismatch on chunk download")
		return fmt.Errorf("size mismatch: expected %d remaining bytes, got %d bytes this session", remainingBytes, newBytes)
	}
	totalExpectedSize := chunk.EndByte - chunk.StartByte + 1
	if chunk.Downloaded != totalExpectedSize {
		return fmt.Errorf("total size mismatch: expected %d total bytes, got %d bytes", totalExpectedSize, chunk.Downloaded)
	}
	log.Debug().Int64("totalExpectedSize", totalExpectedSize).Int64("remainingBytes", remainingBytes).Int64("downloadedThisSession", newBytes).Int64("totalDownloaded", chunk.Downloaded).Msg("Chunk download completed")
	return nil
}
