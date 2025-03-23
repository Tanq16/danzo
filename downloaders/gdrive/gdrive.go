package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/gdrive"
)

// // Update rootCmd.Run to handle Google Drive URLs
// func updateRootCmdToHandleGDrive() {
// 	originalRun := rootCmd.Run
// 	rootCmd.Run = func(cmd *cobra.Command, args []string) {
// 		if len(args) == 0 && urlListFile == "" {
// 			log.Fatal().Msg("No URL or URL list provided")
// 		}
// 		if urlListFile != "" && len(args) > 0 {
// 			log.Fatal().Msg("Cannot specify url argument and --urllist together, choose one")
// 		}

// 		// Check if we have Google Drive URLs
// 		hasGDrive := false

// 		if len(args) > 0 {
// 			url := args[0]
// 			hasGDrive = gdrive.IsGoogleDriveURL(url)
// 		} else if urlListFile != "" {
// 			entries, err := utils.ReadDownloadList(urlListFile)
// 			if err != nil {
// 				log.Fatal().Err(err).Msg("Failed to read URL list file")
// 			}

// 			for _, entry := range entries {
// 				if gdrive.IsGoogleDriveURL(entry.URL) {
// 					hasGDrive = true
// 					break
// 				}
// 			}
// 		}

// 		// If no Google Drive URLs, use original command
// 		if !hasGDrive {
// 			originalRun(cmd, args)
// 			return
// 		}

// 		// Handle Google Drive URLs
// 		log.Info().Msg("Google Drive URL(s) detected, using Google Drive handler")

// 		if len(args) > 0 {
// 			// Single URL
// 			url := args[0]
// 			if output == "" {
// 				log.Fatal().Msg("Output path is required for Google Drive downloads")
// 			}

// 			entries := []utils.DownloadEntry{{URL: url, OutputPath: output}}
// 			err := gdrive.BatchDownloadWithGDriveSupport(entries, 1, connections, timeout, kaTimeout, userAgent, proxyURL)
// 			if err != nil {
// 				log.Fatal().Err(err).Msg("Download failed")
// 			}
// 		} else {
// 			// Batch download
// 			entries, err := utils.ReadDownloadList(urlListFile)
// 			if err != nil {
// 				log.Fatal().Err(err).Msg("Failed to read URL list file")
// 			}

// 			connectionsPerLink := connections
// 			maxConnections := 64
// 			if numLinks*connectionsPerLink > maxConnections {
// 				connectionsPerLink = max(maxConnections/numLinks, 1)
// 				log.Warn().Int("connections", connectionsPerLink).Int("numLinks", numLinks).Msg("adjusted connections to below max limit")
// 			}

// 			err = gdrive.BatchDownloadWithGDriveSupport(entries, numLinks, connectionsPerLink, timeout, kaTimeout, userAgent, proxyURL)
// 			if err != nil {
// 				log.Fatal().Err(err).Msg("Batch download completed with errors")
// 			}
// 		}
// 	}
// }

// func init() {
// 	updateRootCmdToHandleGDrive()
// }

var (
	gdOutput  string
	gdWorkers int
	authOnly  bool
)

var gdriveCmd = &cobra.Command{
	Use:   "gdrive [url]",
	Short: "Download from Google Drive",
	Long:  `Download files or folders from Google Drive using OAuth device authentication`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		// If just authenticating
		if authOnly {
			auth := gdrive.NewAuth()
			if err := auth.CheckAuth(ctx); err != nil {
				log.Fatal().Err(err).Msg("Authentication failed")
			}
			log.Info().Msg("Google Drive authentication successful!")
			return
		}

		// Require URL argument
		if len(args) == 0 {
			log.Fatal().Msg("No Google Drive URL provided")
		}

		url := args[0]
		if !gdrive.IsGoogleDriveURL(url) {
			log.Fatal().Str("url", url).Msg("Not a valid Google Drive URL")
		}

		// If output not specified, use current directory
		if gdOutput == "" {
			currentDir, err := os.Getwd()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to get current directory")
			}
			gdOutput = currentDir
		}

		// Make sure output directory exists
		outputDir := gdOutput
		if !strings.HasSuffix(gdOutput, string(os.PathSeparator)) {
			// Check if it's likely a file path
			if filepath.Ext(gdOutput) != "" || filepath.Base(gdOutput) != "" {
				// Ensure parent directory exists
				outputDir = filepath.Dir(gdOutput)
			}
		}

		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatal().Err(err).Str("dir", outputDir).Msg("Failed to create output directory")
		}

		// Initialize progress manager
		progressManager := gdrive.NewProgressManager()
		progressManager.StartDisplay()
		defer func() {
			progressManager.Stop()
			progressManager.ShowSummary()
		}()

		// Initialize downloader and download
		downloader := gdrive.NewDownloader(progressManager)

		log.Info().Str("url", url).Str("output", gdOutput).Msg("Starting Google Drive download")

		if err := downloader.Download(ctx, url, gdOutput); err != nil {
			log.Fatal().Err(err).Msg("Download failed")
		}

		log.Info().Msg("Download completed successfully")
	},
}

func init() {
	rootCmd.AddCommand(gdriveCmd)

	gdriveCmd.Flags().StringVarP(&gdOutput, "output", "o", "", "Output file or directory path (defaults to current directory)")
	gdriveCmd.Flags().IntVarP(&gdWorkers, "workers", "w", 2, "Number of parallel downloads for folder contents")
	gdriveCmd.Flags().BoolVar(&authOnly, "auth", false, "Only authenticate with Google Drive without downloading")
}
