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
