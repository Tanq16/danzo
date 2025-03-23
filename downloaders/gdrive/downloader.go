package gdrive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/tanq16/danzo/internal"
	"google.golang.org/api/drive/v3"
)

var (
	// Regular expressions for Google Drive URLs
	driveFileRegex      = regexp.MustCompile(`https://drive\.google\.com/file/d/([^/]+)`)
	driveFolderRegex    = regexp.MustCompile(`https://drive\.google\.com/drive/folders/([^/?\s]+)`)
	driveShortLinkRegex = regexp.MustCompile(`https://drive\.google\.com/open\?id=([^&\s]+)`)
)

// Downloader handles Google Drive file downloads
type Downloader struct {
	log        zerolog.Logger
	auth       *Auth
	progress   *ProgressManager
	maxRetries int
}

// NewDownloader creates a new Google Drive downloader
func NewDownloader(progress *ProgressManager) *Downloader {
	log := zerolog.New(os.Stdout).With().Timestamp().Logger()
	return &Downloader{
		log:        log,
		auth:       NewAuth(),
		progress:   progress,
		maxRetries: 5,
	}
}

// IsGoogleDriveURL checks if a URL is a Google Drive URL
func IsGoogleDriveURL(rawURL string) bool {
	return driveFileRegex.MatchString(rawURL) ||
		driveFolderRegex.MatchString(rawURL) ||
		driveShortLinkRegex.MatchString(rawURL)
}

// extractFileID extracts the file ID from a Google Drive URL
func extractFileID(rawURL string) (string, error) {
	// Check if it's a file URL
	if matches := driveFileRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}

	// Check if it's a folder URL
	if matches := driveFolderRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}

	// Check if it's a short link
	if matches := driveShortLinkRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}

	// Try to extract from URL parameters
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	idParam := parsedURL.Query().Get("id")
	if idParam != "" {
		return idParam, nil
	}

	return "", fmt.Errorf("unable to extract file ID from URL: %s", rawURL)
}

// GetFileInfo gets information about a file or folder
func (d *Downloader) GetFileInfo(ctx context.Context, fileID string) (*drive.File, error) {
	srv, err := d.auth.GetDriveService(ctx)
	if err != nil {
		return nil, err
	}

	file, err := srv.Files.Get(fileID).Fields("id, name, size, mimeType").Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get file info: %v", err)
	}

	return file, nil
}

// DownloadFile downloads a file from Google Drive
func (d *Downloader) DownloadFile(ctx context.Context, fileID, outputPath string) error {
	d.log.Info().Str("fileID", fileID).Str("output", outputPath).Msg("Downloading Google Drive file")

	srv, err := d.auth.GetDriveService(ctx)
	if err != nil {
		return err
	}

	// Get file information
	file, err := srv.Files.Get(fileID).Fields("id, name, size, mimeType").Do()
	if err != nil {
		return fmt.Errorf("unable to get file info: %v", err)
	}

	// Check if it's a folder
	if file.MimeType == "application/vnd.google-apps.folder" {
		return d.DownloadFolder(ctx, fileID, outputPath)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// If output path is a directory, use file name from Drive
	fileInfo, err := os.Stat(outputPath)
	if err == nil && fileInfo.IsDir() {
		outputPath = filepath.Join(outputPath, file.Name)
	}

	// Register with progress manager
	size := file.Size
	d.progress.Register(outputPath, size)

	// Setup progress channel
	progressCh := make(chan int64)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		var totalDownloaded int64
		for bytesDownloaded := range progressCh {
			d.progress.Update(outputPath, bytesDownloaded)
			totalDownloaded += bytesDownloaded
		}
		d.progress.Complete(outputPath, totalDownloaded)
	}()

	// Create output file
	out, err := os.Create(outputPath)
	if err != nil {
		close(progressCh)
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer out.Close()

	// Download the file with retries
	success := false
	var lastErr error

	for attempt := 0; attempt < d.maxRetries; attempt++ {
		if attempt > 0 {
			d.log.Info().Int("attempt", attempt+1).Msg("Retrying download")
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		var resp *http.Response
		resp, err = srv.Files.Get(fileID).Download()
		if err != nil {
			lastErr = fmt.Errorf("failed to download file: %v", err)
			continue
		}
		defer resp.Body.Close()

		// Reset file position
		if _, err = out.Seek(0, 0); err != nil {
			lastErr = fmt.Errorf("failed to reset file position: %v", err)
			continue
		}

		if err = out.Truncate(0); err != nil {
			lastErr = fmt.Errorf("failed to truncate file: %v", err)
			continue
		}

		// Copy data with progress
		buffer := make([]byte, 1024*1024*8) // 8MB buffer, same as in internal/helpers.go
		var totalWritten int64

		for {
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				_, writeErr := out.Write(buffer[:n])
				if writeErr != nil {
					lastErr = fmt.Errorf("error writing to file: %v", writeErr)
					break
				}
				totalWritten += int64(n)
				progressCh <- int64(n)
			}

			if err != nil {
				if err == io.EOF {
					success = true
					break
				}
				lastErr = fmt.Errorf("error reading response: %v", err)
				break
			}
		}

		if success {
			break
		}
	}

	close(progressCh)
	wg.Wait()

	if !success {
		return fmt.Errorf("failed to download file after %d attempts: %v", d.maxRetries, lastErr)
	}

	d.log.Info().Str("output", outputPath).Msg("Google Drive file download completed")
	return nil
}

// DownloadFolder downloads an entire folder from Google Drive
func (d *Downloader) DownloadFolder(ctx context.Context, folderID, outputPath string) error {
	d.log.Info().Str("folderID", folderID).Str("output", outputPath).Msg("Downloading Google Drive folder")

	srv, err := d.auth.GetDriveService(ctx)
	if err != nil {
		return err
	}

	// Get folder information
	folder, err := srv.Files.Get(folderID).Fields("name").Do()
	if err != nil {
		return fmt.Errorf("unable to get folder info: %v", err)
	}

	// Create folder structure
	folderPath := outputPath
	fileInfo, err := os.Stat(outputPath)
	if err == nil && fileInfo.IsDir() {
		folderPath = filepath.Join(outputPath, folder.Name)
	}

	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return fmt.Errorf("failed to create folder: %v", err)
	}

	// List all files in the folder
	query := fmt.Sprintf("'%s' in parents", folderID)
	fileList, err := srv.Files.List().Q(query).Fields("files(id, name, mimeType, size)").Do()
	if err != nil {
		return fmt.Errorf("unable to list files in folder: %v", err)
	}

	// Download each file
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := []error{}

	for _, file := range fileList.Files {
		wg.Add(1)
		go func(file *drive.File) {
			defer wg.Done()
			filePath := filepath.Join(folderPath, file.Name)

			// If it's a nested folder
			if file.MimeType == "application/vnd.google-apps.folder" {
				if err := d.DownloadFolder(ctx, file.Id, folderPath); err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("error downloading subfolder %s: %v", file.Name, err))
					mu.Unlock()
				}
				return
			}

			// Download the file
			if err := d.DownloadFile(ctx, file.Id, filePath); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("error downloading file %s: %v", file.Name, err))
				mu.Unlock()
			}
		}(file)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("folder download completed with %d errors: %v", len(errors), errors)
	}

	d.log.Info().Str("output", folderPath).Msg("Google Drive folder download completed")
	return nil
}

// Download handles downloading from Google Drive URLs
func (d *Downloader) Download(ctx context.Context, rawURL, outputPath string) error {
	fileID, err := extractFileID(rawURL)
	if err != nil {
		return err
	}

	return d.DownloadFile(ctx, fileID, outputPath)
}

// BatchDownloadGDrive handles downloading multiple Google Drive URLs
func BatchDownloadGDrive(entries []internal.DownloadEntry, numWorkers int, progressManager *ProgressManager) error {
	log := zerolog.New(os.Stdout).With().Timestamp().Str("module", "gdrive-batch").Logger()
	log.Info().Int("totalFiles", len(entries)).Int("workers", numWorkers).Msg("Initiating Google Drive batch download")

	ctx := context.Background()
	downloader := NewDownloader(progressManager)

	// Check authentication
	if err := downloader.auth.CheckAuth(ctx); err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	var wg sync.WaitGroup
	errorCh := make(chan error, len(entries))
	entriesCh := make(chan internal.DownloadEntry, len(entries))

	for _, entry := range entries {
		entriesCh <- entry
	}
	close(entriesCh)

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			workerLog := log.With().Int("workerID", workerID).Logger()

			for entry := range entriesCh {
				workerLog.Debug().Str("output", entry.OutputPath).Str("url", entry.URL).Msg("Worker starting Google Drive download")

				if err := downloader.Download(ctx, entry.URL, entry.OutputPath); err != nil {
					workerLog.Error().Err(err).Str("url", entry.URL).Msg("Download failed")
					errorCh <- fmt.Errorf("error downloading %s: %v", entry.URL, err)
				} else {
					workerLog.Debug().Str("output", entry.OutputPath).Msg("Download completed successfully")
				}
			}
		}(i + 1)
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(errorCh)

	var downloadErrors []error
	for err := range errorCh {
		downloadErrors = append(downloadErrors, err)
	}

	if len(downloadErrors) > 0 {
		return fmt.Errorf("batch download completed with %d errors: %v", len(downloadErrors), downloadErrors)
	}

	return nil
}
