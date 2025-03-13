package internal

import (
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"time"
)

const bufferSize = 1024 * 512 // 512 KB
var chunkIDRegex = regexp.MustCompile(`\.part(\d+)$`)

type DownloadConfig struct {
	URL         string
	OutputPath  string
	Connections int
	Timeout     time.Duration
	UserAgent   string
	RetryWait   time.Duration
	MaxRetries  int
	VerifyFile  bool
}

type DownloadChunk struct {
	ID         int
	StartByte  int64
	EndByte    int64
	Downloaded int64
	Completed  bool
	Retries    int
	LastError  error
	StartTime  time.Time
	FinishTime time.Time
}

type DownloadJob struct {
	Config    DownloadConfig
	FileSize  int64
	Chunks    []DownloadChunk
	StartTime time.Time
	TempFiles []string
	FileHash  string
}

func createHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // for connection reuse
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		MaxConnsPerHost:     0,
		DisableKeepAlives:   false,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func GetDefaultConnections() int {
	cpus := runtime.NumCPU()
	// 1 connection per core (heuristic default)
	connections := cpus
	// Cap at 64
	if connections > 64 {
		connections = 64
	}
	return connections
}
