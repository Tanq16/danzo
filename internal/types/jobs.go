package types

import "time"

type DownloadConfig struct {
	URL              string
	OutputPath       string
	Connections      int
	HTTPClientConfig HTTPClientConfig
}

type HTTPClientConfig struct {
	Timeout       time.Duration
	KATimeout     time.Duration
	ProxyURL      string
	ProxyUsername string
	ProxyPassword string
	UserAgent     string
	Headers       map[string]string
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
