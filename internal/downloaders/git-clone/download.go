package gitclone

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/tanq16/danzo/internal/utils"
)

type gitCloneProgressWriter struct {
	streamFunc func(string)
}

func (p *gitCloneProgressWriter) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	if message != "" && p.streamFunc != nil {
		p.streamFunc(message)
	}
	return len(data), nil
}

func (d *GitCloneDownloader) Download(job *utils.DanzoJob) error {
	cloneURL := job.Metadata["cloneURL"].(string)
	depth, _ := job.Metadata["depth"].(int)
	auth, err := getAuthMethod(cloneURL, job.Metadata)
	if err != nil && job.StreamFunc != nil {
		job.StreamFunc(fmt.Sprintf("Warning: %v", err))
	}
	progress := &gitCloneProgressWriter{
		streamFunc: job.StreamFunc,
	}
	cloneOptions := &git.CloneOptions{
		URL:      cloneURL,
		Progress: progress,
		Auth:     auth,
	}
	if depth > 0 {
		cloneOptions.Depth = depth
	}
	if job.StreamFunc != nil {
		job.StreamFunc(fmt.Sprintf("Cloning %s", cloneURL))
	}
	_, err = git.PlainClone(job.OutputPath, false, cloneOptions)
	if err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}
	size, err := getDirSize(job.OutputPath)
	if err == nil && job.StreamFunc != nil {
		job.StreamFunc(fmt.Sprintf("Clone complete - Total size: %s", utils.FormatBytes(uint64(size))))
	}
	return nil
}

func getDirSize(path string) (int64, error) {
	// Use "du" if available (faster option)
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
