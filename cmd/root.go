package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/utils"
)

var (
	// Global flags
	proxyURL      string
	proxyUsername string
	proxyPassword string
	userAgent     string
	headers       []string
	workers       int
	connections   int
	fileLog       bool
	version       string = "dev"
)

// Global HTTP client config that will be passed to subcommands
var globalHTTPConfig utils.HTTPClientConfig

// Registry for all subcommands
var commandRegistry = make(map[string]*cobra.Command)

var rootCmd = &cobra.Command{
	Use:               "danzo",
	Short:             "Danzo is a swiss-army knife CLI download manager",
	Version:           version,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Build global HTTP config from flags
		globalHTTPConfig = utils.HTTPClientConfig{
			ProxyURL:      proxyURL,
			ProxyUsername: proxyUsername,
			ProxyPassword: proxyPassword,
			UserAgent:     userAgent,
			Headers:       utils.ParseHeaderArgs(headers),
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
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&proxyURL, "proxy", "p", "", "HTTP/HTTPS proxy URL")
	rootCmd.PersistentFlags().StringVar(&proxyUsername, "proxy-username", "", "Proxy username")
	rootCmd.PersistentFlags().StringVar(&proxyPassword, "proxy-password", "", "Proxy password")
	rootCmd.PersistentFlags().StringVarP(&userAgent, "user-agent", "a", "", "User agent")
	rootCmd.PersistentFlags().StringArrayVarP(&headers, "header", "H", []string{}, "Custom headers")
	rootCmd.PersistentFlags().IntVarP(&workers, "workers", "w", 1, "Number of parallel workers")
	rootCmd.PersistentFlags().IntVarP(&connections, "connections", "c", 8, "Number of connections per download")
	rootCmd.PersistentFlags().BoolVar(&fileLog, "log", false, "Enable debug logging")

	registerCommands()
	fmt.Println()
}

func RegisterCommand(name string, cmd *cobra.Command) {
	commandRegistry[name] = cmd
	rootCmd.AddCommand(cmd)
}

func registerCommands() {
	RegisterCommand("clean", newCleanCmd())

	RegisterCommand("http", newHTTPCmd())
	RegisterCommand("m3u8", newM3U8Cmd())
	// RegisterCommand("s3", newS3Cmd())
	RegisterCommand("gitclone", newGitCloneCmd())
	RegisterCommand("ghrelease", newGHReleaseCmd())

	// RegisterCommand("youtube", newYouTubeCmd())
	// RegisterCommand("batch", newBatchCmd())
}
