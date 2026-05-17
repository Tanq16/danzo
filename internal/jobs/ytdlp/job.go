package ytdlpjob

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/utils"
)

type YTDLPProgress struct {
	DownloadedBytes string `json:"downloaded_bytes"`
	TotalBytes      string `json:"total_bytes"`
	TotalBytesEst   string `json:"total_bytes_estimate"`
	Status          string `json:"status"`
}

type YTDLPJob struct {
	URL        string
	OutputPath string
}

func New(url, outputPath string) *YTDLPJob {
	return &YTDLPJob{
		URL:        url,
		OutputPath: outputPath,
	}
}

func (j *YTDLPJob) ID() string {
	if j.OutputPath != "" {
		return j.OutputPath
	}
	return j.URL
}

func (j *YTDLPJob) Type() string { return "ytdlp" }

func (j *YTDLPJob) Run(ctx context.Context, prog chan<- highway.Progress) error {
	args := []string{j.URL, "--newline", "--progress-template", `JSON_PROGRESS: {"downloaded_bytes": "%(progress.downloaded_bytes)s", "total_bytes": "%(progress.total_bytes)s", "total_bytes_estimate": "%(progress.total_bytes_estimate)s", "status": "%(progress.status)s"}`}

	if j.OutputPath != "" {
		args = append(args, "-o", j.OutputPath)
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	currentPhase := "Downloading"

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "[Merger]") || strings.Contains(line, "Merging formats") {
			currentPhase = "Merging"
			prog <- highway.Progress{
				JobID:     j.ID(),
				Type:      highway.ProgressTypeSubStatus,
				Message:   currentPhase,
				SubStatus: "ffmpeg consolidating streams...",
			}
			continue
		}

		if strings.HasPrefix(line, "JSON_PROGRESS: ") {
			jsonStr := strings.TrimPrefix(line, "JSON_PROGRESS: ")
			var p YTDLPProgress
			if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
				continue
			}

			if currentPhase == "Merging" {
				continue
			}

			downBytes, _ := strconv.ParseInt(p.DownloadedBytes, 10, 64)
			var totalBytes int64
			if p.TotalBytes != "NA" {
				totalBytes, _ = strconv.ParseInt(p.TotalBytes, 10, 64)
			} else if p.TotalBytesEst != "NA" {
				totalBytes, _ = strconv.ParseInt(p.TotalBytesEst, 10, 64)
			}

			if totalBytes > 0 {
				prog <- highway.Progress{
					JobID:   j.ID(),
					Type:    highway.ProgressTypeProgress,
					Message: currentPhase,
					Current: downBytes,
					Total:   totalBytes,
					Extra:   utils.FormatBytes(uint64(downBytes)) + "/" + utils.FormatBytes(uint64(totalBytes)),
				}
			}
		}
	}

	err = cmd.Wait()
	if err != nil {
		prog <- highway.Progress{JobID: j.ID(), Done: true, Error: err, ErrMsg: err.Error()}
		return err
	}

	prog <- highway.Progress{JobID: j.ID(), Done: true}
	return nil
}

type ytdlpJobState struct {
	URL        string `json:"url"`
	OutputPath string `json:"outputPath"`
}

func (j *YTDLPJob) Marshal() ([]byte, error) {
	return json.Marshal(ytdlpJobState{
		URL:        j.URL,
		OutputPath: j.OutputPath,
	})
}

func Unmarshal(data []byte) (highway.Job, error) {
	var state ytdlpJobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return New(state.URL, state.OutputPath), nil
}
