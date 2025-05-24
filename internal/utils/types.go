package utils

import "time"

type Downloader interface {
	Download(job *DanzoJob) error
	BuildJob(job *DanzoJob) error
	ValidateJob(job *DanzoJob) error
}

type DanzoJob struct {
	ID               string
	JobType          string
	OutputPath       string
	ProgressType     string
	ProgressFunc     func(downloaded, total int64)
	StreamFunc       func(line string)
	URL              string
	Connections      int
	Metadata         map[string]any
	HTTPClientConfig HTTPClientConfig
}

type DownloadConfig struct {
	URL              string
	OutputPath       string
	Connections      int
	HTTPClientConfig HTTPClientConfig
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
