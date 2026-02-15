package gitclone

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/tanq16/danzo/internal/highway"
)

type gitCloneProgressWriter struct {
	progressCh chan<- highway.Progress
	jobID      string
}

func (p *gitCloneProgressWriter) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	if message != "" && p.progressCh != nil {
		p.progressCh <- highway.Progress{
			JobID: p.jobID, Type: highway.ProgressTypeSubStatus,
			SubStatus: message,
		}
	}
	return len(data), nil
}

func getDirSize(path string) (int64, error) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		cmd := exec.Command("du", "-s", "-b", path)
		output, err := cmd.CombinedOutput()
		if err == nil {
			parts := strings.Split(string(output), "\t")
			if len(parts) > 0 {
				size, err := strconv.ParseInt(parts[0], 10, 64)
				if err == nil {
					return size, nil
				}
			}
		}
	}
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
