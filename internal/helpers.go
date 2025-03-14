package internal

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const bufferSize = 1024 * 1024 * 2 // 2MB buffer
var chunkIDRegex = regexp.MustCompile(`\.part(\d+)$`)

type DownloadConfig struct {
	URL         string
	OutputPath  string
	Connections int
	Timeout     time.Duration
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
}

func ReadDownloadList(filePath string) ([]DownloadEntry, error) {
	log := GetLogger("config")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading YAML file: %v", err)
	}
	var entries []DownloadEntry
	err = yaml.Unmarshal(data, &entries)
	if err != nil {
		return nil, fmt.Errorf("error parsing YAML file: %v", err)
	}
	for i, entry := range entries {
		if entry.URL == "" {
			return nil, fmt.Errorf("missing URL for entry %d", i+1)
		}
		if entry.OutputPath == "" {
			return nil, fmt.Errorf("missing output path for entry %d", i+1)
		}
	}
	log.Info().Int("count", len(entries)).Msg("Entries loaded from YAML")
	return entries, nil
}

func createHTTPClient(timeout time.Duration, proxyURL string) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // for connection reuse
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		MaxConnsPerHost:     0,
		// These two seem to reduce performance drastically with custom dial context
		// DisableKeepAlives:   false,
		// ForceAttemptHTTP2:   true,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
			// Increased socket buffer size for better speed
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					setSocketOptions(fd)
				})
			},
		}).DialContext,
	}
	if proxyURL != "" {
		proxyURLParsed, err := url.Parse(proxyURL)
		if err != nil {
			log.Error().Err(err).Str("proxy", proxyURL).Msg("Invalid proxy URL, proceeding without proxy")
		} else {
			transport.Proxy = http.ProxyURL(proxyURLParsed)
			log.Debug().Str("proxy", proxyURL).Msg("Using proxy for connections")
		}
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

func Clean(outputPath string) error {
	tempDir := filepath.Join(filepath.Dir(outputPath), ".danzo-temp")
	if err := os.RemoveAll(tempDir); err != nil {
		return err
	}
	return nil
}
