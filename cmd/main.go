package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/tanq16/danzo/internal"
)

func main() {
	// Basic download parameters
	url := flag.String("url", "", "URL to download")
	output := flag.String("output", "", "Output file path")

	// Performance parameters
	connections := flag.Int("connections", getDefaultConnections(), "Number of connections")
	timeout := flag.Duration("timeout", 10*time.Minute, "Connection timeout")

	// HTTP parameters
	userAgent := flag.String("user-agent", "Danzo/1.0", "User agent")

	// Advanced options
	verify := flag.Bool("verify", false, "Verify file integrity after download")
	maxRetries := flag.Int("retries", 3, "Maximum number of retries per chunk")
	retryWait := flag.Duration("retry-wait", 500*time.Millisecond, "Wait time between retries")

	flag.Parse()

	if *url == "" {
		fmt.Println("Error: URL is required")
		flag.Usage()
		os.Exit(1)
	}

	if *output == "" {
		fmt.Println("Error: Output path is required")
		flag.Usage()
		os.Exit(1)
	}

	config := internal.DownloadConfig{
		URL:         *url,
		OutputPath:  *output,
		Connections: *connections,
		Timeout:     *timeout,
		UserAgent:   *userAgent,
		MaxRetries:  *maxRetries,
		RetryWait:   *retryWait,
		VerifyFile:  *verify,
	}

	err := internal.Download(config)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		os.Exit(1)
	}
}

// getDefaultConnections returns a reasonable default number of connections
// based on the number of CPU cores
func getDefaultConnections() int {
	cpus := runtime.NumCPU()

	// Use a multiplier of 1 connection per core
	// as a reasonable default for most downloads
	connections := cpus

	// Cap at 64 connections to avoid overwhelming servers
	if connections > 64 {
		connections = 64
	}

	return connections
}
