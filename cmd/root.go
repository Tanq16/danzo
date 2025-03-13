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
	userAgent   string
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
	rootCmd.Flags().IntVarP(&connections, "connections", "c", min(runtime.NumCPU(), 64), "Number of connections (default: # CPU cores)")
	rootCmd.Flags().DurationVarP(&timeout, "timeout", "t", 3*time.Minute, "Connection timeout")
	rootCmd.Flags().StringVarP(&userAgent, "user-agent", "a", "Danzo/1.0", "User agent")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")

	rootCmd.MarkFlagRequired("url")
	rootCmd.MarkFlagRequired("output")
}
