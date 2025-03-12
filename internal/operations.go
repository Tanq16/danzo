package internal

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	humanize "github.com/dustin/go-humanize"
)

func getFileSize(url string) (int64, error) {
	client := &http.Client{}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Check if server supports range requests
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		return 0, errors.New("server doesn't support range requests")
	}

	contentLength := resp.Header.Get("Content-Length")
	return strconv.ParseInt(contentLength, 10, 64)
}

func downloadChunk(job *DownloadJob, chunk *DownloadChunk, wg *sync.WaitGroup, progressCh chan<- int64) {
	defer wg.Done()

	client := &http.Client{
		Timeout: job.Config.Timeout,
	}

	req, err := http.NewRequest("GET", job.Config.URL, nil)
	if err != nil {
		log.Printf("Error creating request for chunk %d: %v", chunk.ID, err)
		return
	}

	// Set Range header to request specific byte range
	rangeHeader := fmt.Sprintf("bytes=%d-%d", chunk.StartByte, chunk.EndByte)
	req.Header.Set("Range", rangeHeader)
	req.Header.Set("User-Agent", job.Config.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error downloading chunk %d: %v", chunk.ID, err)
		return
	}
	defer resp.Body.Close()

	// Create temporary file for this chunk
	tempFileName := fmt.Sprintf("%s.part%d", job.Config.OutputPath, chunk.ID)
	tempFile, err := os.Create(tempFileName)
	if err != nil {
		log.Printf("Error creating temp file for chunk %d: %v", chunk.ID, err)
		return
	}
	defer tempFile.Close()

	job.TempFiles = append(job.TempFiles, tempFileName)

	// Download data to temp file and report progress
	buffer := make([]byte, 8192)
	for {
		bytesRead, err := resp.Body.Read(buffer)
		if bytesRead > 0 {
			_, writeErr := tempFile.Write(buffer[:bytesRead])
			if writeErr != nil {
				log.Printf("Error writing to temp file: %v", writeErr)
				return
			}

			chunk.Downloaded += int64(bytesRead)
			progressCh <- int64(bytesRead)
		}

		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading response for chunk %d: %v", chunk.ID, err)
			}
			break
		}
	}

	chunk.Completed = true
}

func assembleFile(job DownloadJob) error {
	destFile, err := os.Create(job.Config.OutputPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Sort temp files by chunk ID to ensure correct order
	sort.Strings(job.TempFiles)

	// Combine all temp files into the final file
	for _, tempFilePath := range job.TempFiles {
		tempFile, err := os.Open(tempFilePath)
		if err != nil {
			return err
		}

		_, err = io.Copy(destFile, tempFile)
		tempFile.Close()

		if err != nil {
			return err
		}

		// Remove temp file after copying
		os.Remove(tempFilePath)
	}

	return nil
}

func displayProgress(totalSize int64, progressCh <-chan int64, doneCh <-chan struct{}) {
	downloaded := int64(0)
	startTime := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond) // Update twice every second
	defer ticker.Stop()

	for {
		select {
		case size := <-progressCh:
			downloaded += size

		case <-ticker.C:
			percent := float64(downloaded) / float64(totalSize) * 100
			elapsed := time.Since(startTime).Seconds()
			speed := float64(downloaded) / elapsed / 1024 // KB/s

			// Clear line and print progress
			fmt.Printf("\r\033[K")
			fmt.Printf("Downloaded: %.2f%% (%s/%s) Speed: %.2f KB/s",
				percent,
				humanize.Bytes(uint64(downloaded)),
				humanize.Bytes(uint64(totalSize)),
				speed)

		case <-doneCh:
			// Final update
			percent := float64(downloaded) / float64(totalSize) * 100
			elapsed := time.Since(startTime).Seconds()
			speed := float64(downloaded) / elapsed / 1024 // KB/s

			fmt.Printf("\r\033[K")
			fmt.Printf("Downloaded: %.2f%% (%s/%s) Speed: %.2f KB/s\n",
				percent,
				humanize.Bytes(uint64(downloaded)),
				humanize.Bytes(uint64(totalSize)),
				speed)
			fmt.Println("Download complete!")
			return
		}
	}
}

func Download(config DownloadConfig) error {
	// Get file size
	fileSize, err := getFileSize(config.URL)
	if err != nil {
		return err
	}
	log.Printf("File size: %s", humanize.Bytes(uint64(fileSize)))

	// Create download job
	job := DownloadJob{
		Config:    config,
		FileSize:  fileSize,
		StartTime: time.Now(),
	}

	// Calculate chunk sizes
	chunkSize := fileSize / int64(config.Connections)
	for i := 0; i < config.Connections; i++ {
		startByte := int64(i) * chunkSize
		endByte := startByte + chunkSize - 1

		// Handle last chunk
		if i == config.Connections-1 {
			endByte = fileSize - 1
		}

		job.Chunks = append(job.Chunks, DownloadChunk{
			ID:        i,
			StartByte: startByte,
			EndByte:   endByte,
		})
	}

	// Create channels for progress reporting
	progressCh := make(chan int64)
	doneCh := make(chan struct{})

	// Start progress display
	go displayProgress(fileSize, progressCh, doneCh)
	log.Println("Downloading...")

	// Download chunks concurrently
	var wg sync.WaitGroup
	for i := range job.Chunks {
		wg.Add(1)
		go downloadChunk(&job, &job.Chunks[i], &wg, progressCh)
	}

	// Wait for all downloads to complete
	wg.Wait()

	// Signal progress display to finish
	close(doneCh)

	// Assemble file from chunks
	return assembleFile(job)
}
