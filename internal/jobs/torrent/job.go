package torrentjob

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/utils"
)

type TorrentJob struct {
	id          string
	URI         string
	OutputPath  string
	Connections int
	HTTPConfig  utils.HTTPClientConfig
}

type torrentJobState struct {
	URI         string            `json:"uri"`
	OutputPath  string            `json:"outputPath"`
	Connections int               `json:"connections"`
	ProxyURL    string            `json:"proxyURL,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func New(uri, outputPath string, connections int, httpConfig utils.HTTPClientConfig) *TorrentJob {
	id := outputPath
	if id == "" || id == "." {
		if strings.HasPrefix(uri, "magnet:") {
			id = "magnet-link"
		} else {
			id = filepath.Base(uri)
		}
	}
	return &TorrentJob{
		id:          id,
		URI:         uri,
		OutputPath:  outputPath,
		Connections: connections,
		HTTPConfig:  httpConfig,
	}
}

func (j *TorrentJob) ID() string {
	return j.id
}

func (j *TorrentJob) Type() string { return "torrent" }

func (j *TorrentJob) Run(ctx context.Context, progress chan<- highway.Progress) error {
	progress <- highway.Progress{
		JobID:   j.ID(),
		Type:    highway.ProgressTypeProgress,
		Message: "Starting torrent client",
	}

	cfg := torrent.NewDefaultClientConfig()
	if j.OutputPath != "" {
		cfg.DataDir = j.OutputPath
	} else {
		cfg.DataDir = "." // Default to current directory if OutputPath is empty
	}
	if j.Connections > 0 {
		cfg.EstablishedConnsPerTorrent = j.Connections
	}
	if j.HTTPConfig.UserAgent != "" {
		cfg.HTTPUserAgent = j.HTTPConfig.UserAgent
	}
	if j.HTTPConfig.ProxyURL != "" {
		if proxyURL, err := url.Parse(j.HTTPConfig.ProxyURL); err == nil {
			cfg.HTTPProxy = func(*http.Request) (*url.URL, error) {
				return proxyURL, nil
			}
		}
	}

	client, err := torrent.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create torrent client: %v", err)
	}
	defer client.Close()

	var t *torrent.Torrent
	if strings.HasPrefix(j.URI, "magnet:") {
		t, err = client.AddMagnet(j.URI)
	} else {
		// Try to read it as a file first.
		t, err = client.AddTorrentFromFile(j.URI)
		if err != nil {
			// If it's not a local file, it might be a URL. But for simplicity, we assume it's a file path if not a magnet link.
			return fmt.Errorf("failed to add torrent from file: %v", err)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to add torrent: %v", err)
	}

	progress <- highway.Progress{
		JobID:   j.ID(),
		Type:    highway.ProgressTypeProgress,
		Message: "Fetching info",
	}

	// Wait for info with context cancellation support
	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		return ctx.Err()
	}

	t.DownloadAll()

	progress <- highway.Progress{
		JobID:   j.ID(),
		Type:    highway.ProgressTypeProgress,
		Message: "Downloading",
		Total:   t.Length(),
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastDownloaded int64
	startTime := time.Now()
	var lastTime time.Time = startTime

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current := t.BytesCompleted()
			stats := t.Stats()

			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()

			var speed string
			if elapsed > 0 {
				speed = utils.FormatSpeed(current-lastDownloaded, elapsed)
			}
			lastDownloaded = current
			lastTime = now

			extra := fmt.Sprintf("%s - %d peers (connected: %d)", speed, stats.ActivePeers, stats.TotalPeers)

			progress <- highway.Progress{
				JobID:   j.ID(),
				Type:    highway.ProgressTypeProgress,
				Message: "Downloading",
				Current: current,
				Total:   t.Length(),
				Extra:   extra,
			}

			if current == t.Length() {
				// Finished!
				progress <- highway.Progress{
					JobID: j.ID(),
					Done:  true,
				}
				return nil
			}
		}
	}
}

func (j *TorrentJob) Marshal() ([]byte, error) {
	return json.Marshal(torrentJobState{
		URI:         j.URI,
		OutputPath:  j.OutputPath,
		Connections: j.Connections,
		ProxyURL:    j.HTTPConfig.ProxyURL,
		UserAgent:   j.HTTPConfig.UserAgent,
		Headers:     j.HTTPConfig.Headers,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state torrentJobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URI, state.OutputPath, state.Connections, utils.HTTPClientConfig{
		ProxyURL:  state.ProxyURL,
		UserAgent: state.UserAgent,
		Headers:   state.Headers,
	}), nil
}
