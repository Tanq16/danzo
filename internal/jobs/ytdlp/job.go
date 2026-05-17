package ytdlpjob

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/utils"
)

const (
	progressPrefix   = "JSON_PROGRESS: "
	progressTemplate = progressPrefix + `{"downloaded_bytes": "%(progress.downloaded_bytes)s", "total_bytes": "%(progress.total_bytes)s", "total_bytes_estimate": "%(progress.total_bytes_estimate)s", "status": "%(progress.status)s"}`
)

var ytdlpBinary = "yt-dlp"

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
	if j.OutputPath != "" && !strings.Contains(j.OutputPath, "%(") {
		if _, err := os.Stat(j.OutputPath); err == nil {
			j.OutputPath = utils.RenewOutputPath(j.OutputPath)
		}
	}

	args := []string{j.URL, "--newline", "--progress-template", progressTemplate}
	if j.OutputPath != "" {
		args = append(args, "-o", j.OutputPath)
	}

	cmd := exec.CommandContext(ctx, ytdlpBinary, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %v", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting yt-dlp: %v", err)
	}

	streamErr := streamOutput(j.ID(), stdout, prog)
	waitErr := cmd.Wait()

	if waitErr != nil {
		return ytdlpError(waitErr, stderrBuf.String())
	}
	if streamErr != nil {
		return fmt.Errorf("error reading yt-dlp output: %v", streamErr)
	}

	prog <- highway.Progress{JobID: j.ID(), Done: true}
	return nil
}

func ytdlpError(waitErr error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("yt-dlp failed: %v", waitErr)
	}
	last := lastErrorLine(stderr)
	return fmt.Errorf("yt-dlp failed: %v: %s", waitErr, last)
}

func lastErrorLine(stderr string) string {
	var last string
	for _, ln := range strings.Split(stderr, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		last = ln
		if strings.HasPrefix(ln, "ERROR:") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "ERROR:"))
		}
	}
	return last
}

func streamOutput(jobID string, r io.Reader, prog chan<- highway.Progress) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	currentPhase := "Downloading"

	for scanner.Scan() {
		line := scanner.Text()

		if currentPhase != "Merging" && isMergingLine(line) {
			currentPhase = "Merging"
			prog <- highway.Progress{
				JobID:     jobID,
				Type:      highway.ProgressTypeSubStatus,
				Message:   currentPhase,
				SubStatus: "ffmpeg consolidating streams",
			}
			continue
		}

		if !strings.HasPrefix(line, progressPrefix) {
			continue
		}
		if currentPhase == "Merging" {
			continue
		}

		current, total, ok := parseProgressJSON(strings.TrimPrefix(line, progressPrefix))
		if !ok {
			continue
		}
		prog <- highway.Progress{
			JobID:   jobID,
			Type:    highway.ProgressTypeProgress,
			Message: currentPhase,
			Current: current,
			Total:   total,
			Extra:   utils.FormatBytes(uint64(current)) + "/" + utils.FormatBytes(uint64(total)),
		}
	}
	return scanner.Err()
}

func isMergingLine(line string) bool {
	return strings.Contains(line, "[Merger]") || strings.Contains(line, "Merging formats")
}

func parseProgressJSON(s string) (current, total int64, ok bool) {
	var p YTDLPProgress
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return 0, 0, false
	}
	current, _ = strconv.ParseInt(p.DownloadedBytes, 10, 64)
	switch {
	case p.TotalBytes != "" && p.TotalBytes != "NA":
		total, _ = strconv.ParseInt(p.TotalBytes, 10, 64)
	case p.TotalBytesEst != "" && p.TotalBytesEst != "NA":
		total, _ = strconv.ParseInt(p.TotalBytesEst, 10, 64)
	}
	if total <= 0 {
		return 0, 0, false
	}
	return current, total, true
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
