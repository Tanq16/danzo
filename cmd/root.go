package cmd

import (
	"fmt"
	"os"
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
	userAgent   string
	verify      bool
	maxRetries  int
	retryWait   time.Duration
	debug       bool
)

var rootCmd = &cobra.Command{
	Use:   "danzo",
	Short: "Danzo is a fast CLI download manager",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		internal.InitLogger(debug)
		log.Debug().Msg("Debug logging enabled")
	},
	Run: func(cmd *cobra.Command, args []string) {
		config := internal.DownloadConfig{
			URL:         url,
			OutputPath:  output,
			Connections: connections,
			Timeout:     timeout,
			UserAgent:   userAgent,
			MaxRetries:  maxRetries,
			RetryWait:   retryWait,
			VerifyFile:  verify,
		}
		err := internal.Download(config)
		if err != nil {
			log.Fatal().Err(err).Msg("Download failed")
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
	rootCmd.Flags().StringVarP(&url, "url", "u", "", "URL to download")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "Output file path")
	rootCmd.Flags().IntVarP(&connections, "connections", "c", internal.GetDefaultConnections(), "Number of connections")
	rootCmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Minute, "Connection timeout")
	rootCmd.Flags().StringVar(&userAgent, "user-agent", "Danzo/1.0", "User agent")
	rootCmd.Flags().BoolVarP(&verify, "verify", "v", false, "Verify file integrity after download")
	rootCmd.Flags().IntVarP(&maxRetries, "retries", "r", 3, "Maximum number of retries per chunk")
	rootCmd.Flags().DurationVar(&retryWait, "retry-wait", 500*time.Millisecond, "Wait time between retries")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")

	rootCmd.MarkFlagRequired("url")
	rootCmd.MarkFlagRequired("output")
}
