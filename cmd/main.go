package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tanq16/danzo/internal"
)

func main() {
	url := flag.String("url", "", "URL to download")
	output := flag.String("output", "", "Output file path")
	connections := flag.Int("connections", 8, "Number of connections")
	timeout := flag.Duration("timeout", 60*time.Second, "Connection timeout")
	userAgent := flag.String("user-agent", "Mozilla/Firefox", "User agent")

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
	}

	err := internal.Download(config)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		os.Exit(1)
	}
}
