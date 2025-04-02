package utils

import (
	"errors"
	"regexp"
	"time"
)

const DefaultBufferSize = 1024 * 1024 * 8 // 8MB buffer
const ToolUserAgent = "danzo/1337"

var ErrRangeRequestsNotSupported = errors.New("range requests are not supported")
var ChunkIDRegex = regexp.MustCompile(`\.part(\d+)$`)
var PMDebug = false

var Color = map[string]string{
	"b":    "\033[34m", // Blue
	"r":    "\033[31m", // Red
	"g":    "\033[32m", // Green
	"p":    "\033[35m", // Purple
	"c":    "\033[36m", // Cyan
	"y":    "\033[33m", // Yellow
	"G":    "\033[90m", // Grey
	"R":    "\033[0m",  // Reset
	"pass": "✓",
	"fail": "✗",
}

type DownloadConfig struct {
	URL         string
	OutputPath  string
	Connections int
	Timeout     time.Duration
	KATimeout   time.Duration
	ProxyURL    string
	UserAgent   string
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

type DownloadEntry struct {
	OutputPath string `yaml:"op"`
	URL        string `yaml:"link"`
	Type       string `yaml:"type"`
}

// Local-only User-Agent list
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64; rv:135.0) Gecko/20100101 Firefox/135.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.3 Safari/605.1.15",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:136.0) Gecko/20100101 Firefox/136.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64; rv:136.0) Gecko/20100101 Firefox/136.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36 Edg/132.0.0.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1.1 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 YaBrowser/27.7.7.7 Yowser/2.5 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/115.0",
	"curl/7.88.1",
	"Wget/1.21.4",
}
