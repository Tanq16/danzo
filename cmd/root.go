package cmd

import (
	"fmt"
	u "net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal"
	"github.com/tanq16/danzo/utils"
)

var (
	output        string
	connections   int
	timeout       time.Duration
	kaTimeout     time.Duration
	userAgent     string
	proxyURL      string
	proxyUsername string
	proxyPassword string
	debug         bool
	urlListFile   string
	numLinks      int
	cleanOutput   bool
	headers       []string
	// customization string
)

var DanzoVersion = "dev"

var rootCmd = &cobra.Command{
	Use:     "danzo",
	Short:   "Danzo is a fast CLI download manager",
	Version: DanzoVersion,
	Args:    cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if cleanOutput {
			err := utils.Clean(output)
			if err != nil {
				utils.PrintError("Error cleaning up temporary files")
				os.Exit(1)
			}
			utils.PrintSuccess("Temporary files cleaned up")
			return
		}
		if len(args) == 0 && urlListFile == "" {
			utils.PrintError("No URL or URL list provided")
			os.Exit(1)
		}
		if urlListFile != "" && len(args) > 0 {
			utils.PrintError("Cannot specify url argument and --urllist together, choose one")
			os.Exit(1)
		}
		url := ""
		if userAgent == "randomize" {
			userAgent = utils.GetRandomUserAgent()
		}
		// Check if proxy URL contains auth
		parsedProxy, err := u.Parse(proxyURL)
		if err == nil && parsedProxy.User != nil && proxyUsername == "" {
			proxyUsername = parsedProxy.User.Username()
			if password, set := parsedProxy.User.Password(); set {
				proxyPassword = password
			}
			// Remove auth from URL to send in clientConfig
			parsedProxy.User = nil
			proxyURL = parsedProxy.String()
		}
		httpClientConfig := utils.HTTPClientConfig{
			Timeout:       timeout,
			KATimeout:     kaTimeout,
			ProxyURL:      proxyURL,
			ProxyUsername: proxyUsername,
			ProxyPassword: proxyPassword,
			UserAgent:     userAgent,
			Headers:       utils.ParseHeaderArgs(headers),
		}
		if len(args) > 0 {
			// Handle single URL download
			url = args[0]
			if _, err := u.Parse(url); err != nil {
				utils.PrintError("Invalid URL format")
				os.Exit(1)
			}
			entries := []utils.DownloadEntry{{URL: url, OutputPath: output, Type: utils.DetermineDownloadType(url)}}
			if _, err := os.Stat(output); err == nil {
				entries[0].OutputPath = utils.RenewOutputPath(output)
			}
			err := internal.BatchDownload(entries, 1, connections, httpClientConfig)
			if err != nil {
				fmt.Println()
				utils.PrintError("Encountered failed operation(s)")
				os.Exit(1)
			}
			return
		} else {
			// Handle batch download from URL list file
			entries, err := utils.ReadDownloadList(urlListFile)
			if err != nil {
				utils.PrintError("Failed to read URL list file")
				os.Exit(1)
			}
			connectionsPerLink := connections
			maxConnections := 64
			if numLinks*connectionsPerLink > maxConnections {
				connectionsPerLink = max(maxConnections/numLinks, 1)
			}
			err = internal.BatchDownload(entries, numLinks, connectionsPerLink, httpClientConfig)
			if err != nil {
				fmt.Println()
				utils.PrintError("Encountered failed operation(s)")
				os.Exit(1)
			}
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (Danzo infers file name if not provided)")
	rootCmd.Flags().StringVarP(&urlListFile, "urllist", "l", "", "Path to YAML file containing URLs and output paths")
	rootCmd.Flags().IntVarP(&numLinks, "workers", "w", 1, "Number of links to download in parallel")
	rootCmd.Flags().IntVarP(&connections, "connections", "c", 8, "Number of connections per download (above 8 enables high-thread-mode)")
	rootCmd.Flags().DurationVarP(&timeout, "timeout", "t", 3*time.Minute, "Connection timeout (eg. 5s, 10m)")
	rootCmd.Flags().DurationVarP(&kaTimeout, "keep-alive-timeout", "k", 90*time.Second, "Keep-alive timeout for client (eg. 10s, 1m, 80s)")
	rootCmd.Flags().StringVarP(&userAgent, "user-agent", "a", utils.ToolUserAgent, "User agent")
	rootCmd.Flags().StringVarP(&proxyURL, "proxy", "p", "", "HTTP/HTTPS proxy URL (e.g., proxy.example.com:8080)")
	rootCmd.Flags().StringVar(&proxyUsername, "proxy-username", "", "Proxy username (if not provided in proxy URL)")
	rootCmd.Flags().StringVar(&proxyPassword, "proxy-password", "", "Proxy password (if not provided in proxy URL)")
	rootCmd.Flags().StringArrayVarP(&headers, "header", "H", []string{}, "Custom headers (like 'Authorization: Basic dXNlcjpwYXNz'); can be specified multiple times")
	// rootCmd.Flags().StringVarP(&customization, "customization", "z", "", "Additional options for customizing behavior") // for future use

	// flags without shorthand
	rootCmd.Flags().BoolVar(&cleanOutput, "clean", false, "Clean up temporary files for provided output path")
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
}
