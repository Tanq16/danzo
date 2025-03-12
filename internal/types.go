package internal

import "time"

type DownloadConfig struct {
	URL         string
	OutputPath  string
	Connections int
	Timeout     time.Duration
	UserAgent   string
}

type DownloadChunk struct {
	ID         int
	StartByte  int64
	EndByte    int64
	Downloaded int64
	Completed  bool
}

type DownloadJob struct {
	Config    DownloadConfig
	FileSize  int64
	Chunks    []DownloadChunk
	StartTime time.Time
	TempFiles []string
}
