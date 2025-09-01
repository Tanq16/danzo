package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/utils"
)

var DanzoVersion = "dev"

var (
	// Global flags
	proxyURL      string
	proxyUsername string
	proxyPassword string
	userAgent     string
	headers       []string
	workers       int
	connections   int
	debugFlag     string
)

// Global HTTP client config that will be passed to subcommands
var globalHTTPConfig utils.HTTPClientConfig

var rootCmd = &cobra.Command{
	Use:               "danzo",
	Short:             "Danzo is a swiss-army knife CLI download manager",
	Version:           DanzoVersion,
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

func setupLogs() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.DateTime,
		NoColor:    false, // Enable color output
	}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.Disabled)
	switch debugFlag {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		utils.GlobalDebugFlag = true
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		utils.GlobalDebugFlag = true
	case "disabled":
		zerolog.SetGlobalLevel(zerolog.Disabled)
		utils.GlobalDebugFlag = false
	default:
		zerolog.SetGlobalLevel(zerolog.Disabled)
		utils.GlobalDebugFlag = false
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.PersistentFlags().StringVar(&debugFlag, "debug", "disabled", "Enable logging for debug, info or disabled (TUI mode) level")
	cobra.OnInitialize(setupLogs)

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&proxyURL, "proxy", "p", "", "HTTP/HTTPS proxy URL")
	rootCmd.PersistentFlags().StringVar(&proxyUsername, "proxy-username", "", "Proxy username")
	rootCmd.PersistentFlags().StringVar(&proxyPassword, "proxy-password", "", "Proxy password")
	rootCmd.PersistentFlags().StringVarP(&userAgent, "user-agent", "a", "Danzo-CLI", "User agent")
	rootCmd.PersistentFlags().StringArrayVarP(&headers, "header", "H", []string{}, "Custom headers")
	rootCmd.PersistentFlags().IntVarP(&workers, "workers", "w", 1, "Number of parallel workers")
	rootCmd.PersistentFlags().IntVarP(&connections, "connections", "c", 8, "Number of connections per download")

	registerCommands()
	fmt.Println()
}

func registerCommands() {
	rootCmd.AddCommand(newCleanCmd())
	rootCmd.AddCommand(newHTTPCmd())
	rootCmd.AddCommand(newM3U8Cmd())
	rootCmd.AddCommand(newS3Cmd())
	rootCmd.AddCommand(newGitCloneCmd())
	rootCmd.AddCommand(newGHReleaseCmd())
	rootCmd.AddCommand(newGDriveCmd())
	rootCmd.AddCommand(newYouTubeCmd())
	rootCmd.AddCommand(newYTMusicCmd())
	rootCmd.AddCommand(newBatchCmd())
}
