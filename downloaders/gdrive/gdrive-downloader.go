package gdrive

import (
	"time"

	"github.com/tanq16/danzo/internal"
	"github.com/tanq16/danzo/utils"
)

// BatchDownloadWithGDriveSupport extends the BatchDownload function to handle Google Drive URLs
func BatchDownloadWithGDriveSupport(entries []utils.DownloadEntry, numLinks int, connectionsPerLink int, timeout time.Duration, kaTimeout time.Duration, userAgent, proxyURL string) error {
	log := internal.GetLogger("downloader")

	// Sort entries into Google Drive and regular URLs
	var gdriveEntries []internal.DownloadEntry
	var regularEntries []internal.DownloadEntry

	for _, entry := range entries {
		if IsGoogleDriveURL(entry.URL) {
			gdriveEntries = append(gdriveEntries, entry)
		} else {
			regularEntries = append(regularEntries, entry)
		}
	}

	// Initialize progress manager
	progressManager := NewProgressManager()
	progressManager.StartDisplay()
	defer func() {
		progressManager.Stop()
		progressManager.ShowSummary()
		for _, entry := range entries {
			internal.Clean(entry.OutputPath)
		}
	}()

	// First, handle Google Drive entries if any
	if len(gdriveEntries) > 0 {
		log.Info().Int("count", len(gdriveEntries)).Msg("Processing Google Drive downloads")

		gdWorkersCount := numLinks
		if gdWorkersCount > 4 {
			gdWorkersCount = 4 // Limit Google Drive workers to prevent API rate limiting
		}

		if err := BatchDownloadGDrive(gdriveEntries, gdWorkersCount, progressManager); err != nil {
			log.Error().Err(err).Msg("Error in Google Drive downloads")
			return err
		}
	}

	// Then handle regular entries
	if len(regularEntries) > 0 {
		log.Info().Int("count", len(regularEntries)).Msg("Processing regular downloads")

		if err := internal.BatchDownload(regularEntries, numLinks, connectionsPerLink, timeout, kaTimeout, userAgent, proxyURL); err != nil {
			log.Error().Err(err).Msg("Error in regular downloads")
			return err
		}
	}

	return nil
}
