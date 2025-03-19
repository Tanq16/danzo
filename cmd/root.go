package cmd

import (
	"fmt"
	u "net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal"
)

var (
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

var DanzoVersion = "dev"

var rootCmd = &cobra.Command{
	Use:     "danzo",
	Short:   "Danzo is a fast CLI download manager",
	Version: DanzoVersion,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		internal.InitLogger(debug)
		log.Debug().Msg("Debug logging enabled")
	},
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 && urlListFile == "" {
			log.Fatal().Msg("No URL or URL list provided")
		}
		if urlListFile != "" && len(args) > 0 {
			log.Fatal().Msg("Cannot specify url argument and --urllist together, choose one")
		}
		url := ""
		if len(args) > 0 {
			// Handle single URL download
			url = args[0]
			if _, err := u.Parse(url); err != nil {
				log.Fatal().Err(err).Msg("Invalid URL format")
			}
			if output == "" {
				parsedURL, _ := u.Parse(url)
				output = strings.Split(parsedURL.Path, "/")[len(strings.Split(parsedURL.Path, "/"))-1]
				log.Debug().Str("output", output).Msg("Output file path not specified, using URL path")
			}
			entries := []internal.DownloadEntry{{URL: url, OutputPath: output}}
			if _, err := os.Stat(output); err == nil {
				entries[0].OutputPath = internal.RenewOutputPath(output)
			}
			err := internal.BatchDownload(entries, 1, connections, timeout, kaTimeout, userAgent, proxyURL)
			if err != nil {
				log.Fatal().Err(err).Msg("Download failed")
			}
			return
		} else {
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
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (required with --url/-u)")
	rootCmd.Flags().StringVarP(&urlListFile, "urllist", "l", "", "Path to YAML file containing URLs and output paths")
	rootCmd.Flags().IntVarP(&numLinks, "workers", "w", 1, "Number of links to download in parallel")
	rootCmd.Flags().IntVarP(&connections, "connections", "c", 4, "Number of connections per download")
	rootCmd.Flags().DurationVarP(&timeout, "timeout", "t", 3*time.Minute, "Connection timeout (eg. 5s, 10m)")
	rootCmd.Flags().DurationVarP(&kaTimeout, "keep-alive-timeout", "k", 90*time.Second, "Keep-alive timeout for client (eg. 10s, 1m, 80s)")
	rootCmd.Flags().StringVarP(&userAgent, "user-agent", "a", internal.ToolUserAgent, "User agent")
	rootCmd.Flags().StringVarP(&proxyURL, "proxy", "p", "", "HTTP/HTTPS proxy URL (e.g., proxy.example.com:8080)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")

	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().StringVarP(&cleanOutput, "output", "o", "", "Output file path")
}
