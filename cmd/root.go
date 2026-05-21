package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/utils"
)

var AppVersion = "dev-build"

var (
	proxyURL      string
	proxyUsername string
	proxyPassword string
	userAgent     string
	headers       []string
	workers       int
	connections   int
	debugFlag     bool
	forAIFlag     bool
)

var globalHTTPConfig utils.HTTPClientConfig

var rootCmd = &cobra.Command{
	Use:               "danzo",
	Short:             "Danzo is a swiss-army knife CLI download manager",
	Version:           AppVersion,
	CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		globalHTTPConfig = utils.HTTPClientConfig{
			Jar:           nil,
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
		NoColor:    false,
	}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.Disabled)
	utils.GlobalDebugFlag = false
	utils.GlobalForAIFlag = false
	if debugFlag {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		utils.GlobalDebugFlag = true
	}
	if forAIFlag {
		zerolog.SetGlobalLevel(zerolog.Disabled)
		utils.GlobalForAIFlag = true
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
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&forAIFlag, "for-ai", false, "AI-friendly output for agent consumption")
	rootCmd.MarkFlagsMutuallyExclusive("debug", "for-ai")
	cobra.OnInitialize(setupLogs)

	rootCmd.PersistentFlags().StringVarP(&proxyURL, "proxy", "p", "", "HTTP/HTTPS proxy URL")
	rootCmd.PersistentFlags().StringVar(&proxyUsername, "proxy-username", "", "Proxy username")
	rootCmd.PersistentFlags().StringVar(&proxyPassword, "proxy-password", "", "Proxy password")
	rootCmd.PersistentFlags().StringVarP(&userAgent, "user-agent", "a", "Danzo-CLI", "User agent")
	rootCmd.PersistentFlags().StringArrayVarP(&headers, "header", "H", []string{}, "Custom headers")
	rootCmd.PersistentFlags().IntVarP(&workers, "workers", "w", 1, "Number of parallel workers")
	rootCmd.PersistentFlags().IntVarP(&connections, "connections", "c", 8, "Number of connections per download")

	rootCmd.AddCommand(newCleanCmd())
	rootCmd.AddCommand(newHTTPCmd())
	rootCmd.AddCommand(newM3U8Cmd())
	rootCmd.AddCommand(newS3Cmd())
	rootCmd.AddCommand(newGHReleaseCmd())
	rootCmd.AddCommand(newResumeCmd())
	rootCmd.AddCommand(newYtdlpCmd())
	rootCmd.AddCommand(newTorrentCmd())
}
