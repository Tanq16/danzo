package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/scheduler"
	"github.com/tanq16/danzo/internal/utils"
)

type BatchEntry struct {
	OutputPath string `yaml:"op,omitempty"`
	Link       string `yaml:"link"`
}

type BatchFile map[string][]BatchEntry

func newBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch [YAML_FILE] [OPTIONS]",
		Short: "Process multiple downloads from a YAML file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			yamlFile := args[0]
			data, err := os.ReadFile(yamlFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading YAML file: %v\n", err)
				os.Exit(1)
			}
			var batchFile BatchFile
			if err := yaml.Unmarshal(data, &batchFile); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing YAML file: %v\n", err)
				os.Exit(1)
			}
			jobs := buildJobsFromBatch(batchFile)
			if len(jobs) == 0 {
				fmt.Fprintf(os.Stderr, "No valid jobs found in the batch file\n")
				os.Exit(1)
			}
			scheduler.Run(jobs, workers)
		},
	}
	return cmd
}

func buildJobsFromBatch(batchFile BatchFile) []utils.DanzoJob {
	var jobs []utils.DanzoJob
	for jobType, entries := range batchFile {
		normalizedType := normalizeJobType(jobType)
		if normalizedType == "" {
			fmt.Fprintf(os.Stderr, "Warning: Unknown job type '%s', skipping...\n", jobType)
			continue
		}
		for _, entry := range entries {
			if entry.Link == "" {
				fmt.Fprintf(os.Stderr, "Warning: Empty link found in %s section, skipping...\n", jobType)
				continue
			}
			job := utils.DanzoJob{
				JobType:          normalizedType,
				URL:              entry.Link,
				OutputPath:       entry.OutputPath,
				HTTPClientConfig: globalHTTPConfig,
				Metadata:         make(map[string]any),
			}
			switch normalizedType {
			case "http", "google-drive", "github-release", "live-stream":
				job.Connections = connections
				job.ProgressType = "progress"
			case "s3":
				job.Connections = connections
				job.ProgressType = "progress"
				job.Metadata["profile"] = "default"
			case "youtube", "youtube-music", "git-clone":
				job.ProgressType = "stream"
			default:
				job.ProgressType = "progress"
			}
			addJobTypeSpecificMetadata(&job, normalizedType)
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func normalizeJobType(jobType string) string {
	typeMap := map[string]string{
		"http":            "http",
		"https":           "http",
		"s3":              "s3",
		"gdrive":          "google-drive",
		"googledrive":     "google-drive",
		"google-drive":    "google-drive",
		"gitclone":        "git-clone",
		"git-clone":       "git-clone",
		"git":             "git-clone",
		"ghr":             "github-release",
		"ghrelease":       "github-release",
		"gh-release":      "github-release",
		"github":          "github-release",
		"github-release":  "github-release",
		"m3u8":            "live-stream",
		"hls":             "live-stream",
		"http-livestream": "live-stream",
		"live-stream":     "live-stream",
		"youtube":         "youtube",
		"yt":              "youtube",
		"ytm":             "yt-music",
		"ytmusic":         "yt-music",
		"youtube-music":   "yt-music",
		"yt-music":        "yt-music",
	}
	normalized := ""
	for key, value := range typeMap {
		if key == jobType || key == strings.ToLower(jobType) {
			normalized = value
			break
		}
	}
	return normalized
}

func addJobTypeSpecificMetadata(job *utils.DanzoJob, jobType string) {
	switch jobType {
	case "youtube":
		if _, ok := job.Metadata["format"]; !ok {
			job.Metadata["format"] = "decent"
		}
	case "github-release":
		job.Metadata["manual"] = false
	case "git-clone":
		if _, ok := job.Metadata["depth"]; !ok {
			job.Metadata["depth"] = 0
		}
	}
}
