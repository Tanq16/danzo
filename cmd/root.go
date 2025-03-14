package cmd

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal"
)

var (
	url         string
	output      string
	connections int
	timeout     time.Duration
	kaTimeout   time.Duration
	userAgent   string
	proxyURL    string
	debug       bool
	cleanOutput string
	urlListFile string
	numLinks    int
)

var rootCmd = &cobra.Command{
	Use:   "danzo",
	Short: "Danzo is a fast CLI download manager",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		internal.InitLogger(debug)
		log.Debug().Msg("Debug logging enabled")
	},
	Run: func(cmd *cobra.Command, args []string) {
		if urlListFile != "" && url != "" {
			log.Fatal().Msg("Cannot specify both --url and --urllist, choose one")
		}
		if urlListFile == "" && url == "" {
			log.Fatal().Msg("Must specify either --url or --urllist")
		}

		// Handle single URL download
		if url != "" {
			if output == "" {
				log.Fatal().Msg("Output file path is required with --url")
			}
			entries := []internal.DownloadEntry{{URL: url, OutputPath: output}}
			err := internal.BatchDownload(entries, 1, connections, timeout, kaTimeout, userAgent, proxyURL)
			if err != nil {
				log.Fatal().Err(err).Msg("Download failed")
			}
			return
		}

		// Handle batch download from URL list file
		entries, err := internal.ReadDownloadList(urlListFile)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to read URL list file")
		}
		connectionsPerLink := connections
		maxConnections := 64
		if numLinks*connectionsPerLink > maxConnections {
			connectionsPerLink = max(maxConnections/numLinks, 1)
			log.Warn().Int("connections", connectionsPerLink).Int("numLinks", numLinks).Msg("adjusted connections to below max limit")
		}
		err = internal.BatchDownload(entries, numLinks, connectionsPerLink, timeout, kaTimeout, userAgent, proxyURL)
		if err != nil {
			log.Fatal().Err(err).Msg("Batch download completed with errors")
		}
	},
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up temporary files",
	Run: func(cmd *cobra.Command, args []string) {
		err := internal.Clean(cleanOutput)
		if err != nil {
			log.Fatal().Err(err).Msg("Error cleaning up temporary files")
		}
		log.Info().Msg("Temporary files cleaned up")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&url, "url", "u", "", "URL to download")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (required with --url/-u)")
	rootCmd.Flags().StringVarP(&urlListFile, "urllist", "l", "", "Path to YAML file containing URLs and output paths")
	rootCmd.Flags().IntVarP(&numLinks, "workers", "w", 1, "Number of links to download in parallel (default: 1)")
	rootCmd.Flags().IntVarP(&connections, "connections", "c", min(runtime.NumCPU(), 32), "Number of connections per download (default: CPU cores)")
	rootCmd.Flags().DurationVarP(&timeout, "timeout", "t", 3*time.Minute, "Connection timeout (eg., 5s, 10m; default: 3m)")
	rootCmd.Flags().DurationVarP(&kaTimeout, "keep-alive-timeout", "k", 90*time.Second, "Keep-alive timeout for client (eg./ 10s, 1m, 80s; default: 90s)")
	rootCmd.Flags().StringVarP(&userAgent, "user-agent", "a", "Danzo/1337", "User agent")
	rootCmd.Flags().StringVarP(&proxyURL, "proxy", "p", "", "HTTP/HTTPS proxy URL (e.g., proxy.example.com:8080)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")

	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().StringVarP(&cleanOutput, "output", "o", "", "Output file path")
}
