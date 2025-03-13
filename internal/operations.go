package internal

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

func getFileSize(url string, userAgent string, client *http.Client) (int64, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		return 0, errors.New("server doesn't support range requests")
	}
	contentLength := resp.Header.Get("Content-Length")
	if contentLength == "" {
		return 0, errors.New("server didn't provide Content-Length header")
	}
	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid content length: %v", err)
	}
	if size <= 0 {
		return 0, errors.New("invalid file size reported by server")
	}
	return size, nil
}

func downloadChunk(job *DownloadJob, chunk *DownloadChunk, client *http.Client, wg *sync.WaitGroup, progressCh chan<- int64, mutex *sync.Mutex) {
	defer wg.Done()
	tempDir := filepath.Join(filepath.Dir(job.Config.OutputPath), ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("Error creating temp directory: %v", err)
		return
	}
	tempFileName := filepath.Join(tempDir, fmt.Sprintf("%s.part%d", filepath.Base(job.Config.OutputPath), chunk.ID))
	if fileInfo, err := os.Stat(tempFileName); err == nil {
		expectedSize := chunk.EndByte - chunk.StartByte + 1
		if fileInfo.Size() == expectedSize {
			log.Printf("Chunk %d already downloaded, skipping", chunk.ID)
			mutex.Lock()
			job.TempFiles = append(job.TempFiles, tempFileName)
			mutex.Unlock()
			chunk.Downloaded = expectedSize
			chunk.Completed = true
			progressCh <- expectedSize
			return
		}
	}
	maxRetries := 3
	for retry := range maxRetries {
		if retry > 0 {
			log.Printf("Retrying download of chunk %d (attempt %d/%d)", chunk.ID, retry+1, maxRetries)
			time.Sleep(time.Duration(retry) * 500 * time.Millisecond) // Backoff
		}
		chunk.Downloaded = 0
		if err := doDownloadChunk(job, chunk, client, tempFileName, progressCh); err != nil {
			log.Printf("Error downloading chunk %d (attempt %d): %v", chunk.ID, retry+1, err)
			continue
		}
		mutex.Lock()
		job.TempFiles = append(job.TempFiles, tempFileName)
		mutex.Unlock()
		chunk.Completed = true
		return
	}
	log.Printf("Failed to download chunk %d after %d attempts", chunk.ID, maxRetries)
}

func doDownloadChunk(job *DownloadJob, chunk *DownloadChunk, client *http.Client, tempFileName string, progressCh chan<- int64) error {
	req, err := http.NewRequest("GET", job.Config.URL, nil)
	if err != nil {
		return err
	}
	rangeHeader := fmt.Sprintf("bytes=%d-%d", chunk.StartByte, chunk.EndByte)
	req.Header.Set("Range", rangeHeader)
	req.Header.Set("User-Agent", job.Config.UserAgent)
	req.Header.Set("Connection", "keep-alive")
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
	tempFile, err := os.Create(tempFileName)
	if err != nil {
		return err
	}
	defer tempFile.Close()
	buffer := make([]byte, bufferSize)
	expectedSize := chunk.EndByte - chunk.StartByte + 1
	for {
		bytesRead, err := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := tempFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				return writeErr
			}
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
	if chunk.Downloaded != expectedSize {
		return fmt.Errorf("size mismatch: expected %d bytes, got %d bytes", expectedSize, chunk.Downloaded)
	}
	return nil
}

func extractChunkID(filename string) (int, error) {
	matches := chunkIDRegex.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return -1, fmt.Errorf("could not extract chunk ID from %s", filename)
	}
	return strconv.Atoi(matches[1])
}

func assembleFile(job DownloadJob) error {
	allChunksCompleted := true
	for i, chunk := range job.Chunks {
		if !chunk.Completed {
			log.Printf("Warning: Chunk %d was not completed", i)
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
			log.Printf("Warning: Error sorting chunk files: %v, %v", errI, errJ)
			return tempFiles[i] < tempFiles[j] // Fallback to string comparison
		}
		return idI < idJ
	})
	log.Printf("Assembling %d chunks in order:", len(tempFiles))
	for i, file := range tempFiles {
		chunkID, _ := extractChunkID(file)
		log.Printf("  %d: Chunk %d (%s)", i, chunkID, file)
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
	for _, tempFilePath := range tempFiles {
		os.Remove(tempFilePath)
	}
	tempDir := filepath.Dir(tempFiles[0])
	os.RemoveAll(tempDir)
	return nil
}

func displayProgress(totalSize int64, progressCh <-chan int64, doneCh <-chan struct{}) {
	downloaded := int64(0)
	startTime := time.Now()
	lastUpdateTime := startTime
	lastDownloaded := int64(0)
	ticker := time.NewTicker(500 * time.Millisecond) // Update twice every second
	defer ticker.Stop()
	for {
		select {
		case size := <-progressCh:
			downloaded += size
		case <-ticker.C:
			percent := float64(downloaded) / float64(totalSize) * 100
			currentTime := time.Now()
			timeDiff := currentTime.Sub(lastUpdateTime).Seconds()
			byteDiff := downloaded - lastDownloaded
			speed := float64(0)
			if timeDiff > 0 {
				speed = float64(byteDiff) / timeDiff / 1024 // KB/s
				lastUpdateTime = currentTime
				lastDownloaded = downloaded
			}
			elapsed := time.Since(startTime).Seconds()
			avgSpeed := float64(downloaded) / elapsed / 1024 // KB/s
			var eta string
			if speed > 0 {
				etaSeconds := int64(float64(totalSize-downloaded) / (speed * 1024))
				if etaSeconds < 60 {
					eta = fmt.Sprintf("%ds", etaSeconds)
				} else if etaSeconds < 3600 {
					eta = fmt.Sprintf("%dm %ds", etaSeconds/60, etaSeconds%60)
				} else {
					eta = fmt.Sprintf("%dh %dm", etaSeconds/3600, (etaSeconds%3600)/60)
				}
			} else {
				eta = "calculating..."
			}
			fmt.Printf("\r\033[K")
			fmt.Printf("Downloaded: %.2f%% (%s/%s) Speed: %.2f KB/s Avg: %.2f KB/s ETA: %s", percent, formatBytes(uint64(downloaded)), formatBytes(uint64(totalSize)), speed, avgSpeed, eta)

		case <-doneCh:
			elapsed := time.Since(startTime).Seconds()
			avgSpeed := float64(downloaded) / elapsed / 1024 // KB/s
			fmt.Printf("\r\033[K")
			fmt.Printf("Downloaded: 100%% (%s) | Average Speed: %.2f KB/s | Time: %.1fs\n", formatBytes(uint64(downloaded)), avgSpeed, elapsed)
			fmt.Println("Download complete!")
			return
		}
	}
}
