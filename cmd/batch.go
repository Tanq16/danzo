package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tanq16/danzo/internal/display"
	"github.com/tanq16/danzo/internal/highway"
	ghreleasejob "github.com/tanq16/danzo/internal/jobs/github-release"
	httpjob "github.com/tanq16/danzo/internal/jobs/http"
	m3u8job "github.com/tanq16/danzo/internal/jobs/live-stream"
	s3job "github.com/tanq16/danzo/internal/jobs/s3"
	torrentjob "github.com/tanq16/danzo/internal/jobs/torrent"
	ytdlpjob "github.com/tanq16/danzo/internal/jobs/ytdlp"
	"github.com/tanq16/danzo/utils"
	"go.yaml.in/yaml/v4"
)

// YAMLJob represents a single job's configuration parsed from YAML/JSON
type YAMLJob struct {
	URL                string `yaml:"url" json:"url"`
	Output             string `yaml:"output" json:"output"`
	Type               string `yaml:"type" json:"type"`
	Connections        int    `yaml:"connections" json:"connections"`
	Cookies            string `yaml:"cookies" json:"cookies"`
	CookiesFromBrowser string `yaml:"cookies_from_browser" json:"cookies_from_browser"`
	Profile            string `yaml:"profile" json:"profile"`
	Manual             *bool  `yaml:"manual" json:"manual"`
	Extract            string `yaml:"extract" json:"extract"`
}

var batchFlags struct {
	cookies            string
	cookiesFromBrowser string
	s3Profile          string
	extract            string
	manual             bool
}

var batchCmd = &cobra.Command{
	Use:   "batch [FILE]",
	Short: "Download multiple jobs in batch from a file or stdin",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := ""
		if len(args) > 0 {
			filePath = args[0]
		}

		jobConfigs, err := parseBatchInput(filePath)
		if err != nil {
			utils.PrintFatal("Failed to parse batch input", err)
		}

		if len(jobConfigs) == 0 {
			fmt.Println("No jobs found in batch input.")
			return
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		hw := newHighway()
		disp := display.New(display.DefaultConfig())

		var submittedJobs []highway.Job
		for _, cfg := range jobConfigs {
			job, err := buildJob(cfg)
			if err != nil {
				utils.PrintFatal("Failed to configure job", err)
			}
			disp.RegisterJob(job.ID())
			hw.Submit(job)
			submittedJobs = append(submittedJobs, job)
		}

		disp.Start(hw.Progress())
		runErr := hw.Run(ctx)
		disp.Stop()

		if runErr != nil {
			utils.PrintFatal("Batch execution finished with failures", runErr)
		}
	},
}

func parseBatchInput(filePath string) ([]YAMLJob, error) {
	var r io.Reader
	if filePath == "" || filePath == "-" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return nil, fmt.Errorf("no input file specified and stdin is not a pipe/redirect")
		}
		r = os.Stdin
	} else {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
		}
		defer f.Close()
		r = f
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch content: %w", err)
	}

	isYAML := false
	if filePath != "" && (strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") || strings.HasSuffix(filePath, ".json")) {
		isYAML = true
	} else {
		trimmed := strings.TrimSpace(string(data))
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "[") {
			isYAML = true
		}
	}

	if isYAML {
		var jobs []YAMLJob
		if err := yaml.Unmarshal(data, &jobs); err == nil {
			return jobs, nil
		} else {
			return nil, fmt.Errorf("failed to parse YAML/JSON batch file: %w", err)
		}
	}

	// Plain-text parser
	var jobs []YAMLJob
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := splitLine(line)
		if len(parts) == 0 {
			continue
		}
		urlStr := parts[0]
		output := ""
		if len(parts) > 1 {
			output = parts[1]
		}
		jobs = append(jobs, YAMLJob{
			URL:    urlStr,
			Output: output,
		})
	}

	return jobs, nil
}

func splitLine(line string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if c == ' ' || c == '\t' {
			if inQuotes {
				current.WriteByte(c)
			} else if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func parsePrefix(rawURL string) (string, string) {
	parts := strings.SplitN(rawURL, "::", 2)
	if len(parts) == 2 {
		prefix := strings.ToLower(strings.TrimSpace(parts[0]))
		actualURL := strings.TrimSpace(parts[1])
		return prefix, actualURL
	}
	return "", rawURL
}

func getJobType(prefix, rawURL, overrideType string) string {
	if overrideType != "" {
		return strings.ToLower(overrideType)
	}
	if prefix != "" {
		switch prefix {
		case "http", "https":
			return "http"
		case "hls", "m3u8", "livestream", "live-stream", "stream":
			return "live-stream"
		case "ghr", "github-release", "ghrelease":
			return "github-release"
		case "s3":
			return "s3"
		case "ytdlp", "yt-dlp", "youtube-dl", "ytdl":
			return "ytdlp"
		case "torrent":
			return "torrent"
		}
	}
	// Dynamic fallbacks
	if strings.HasPrefix(rawURL, "s3://") {
		return "s3"
	}
	if strings.HasPrefix(rawURL, "magnet:") || strings.HasSuffix(rawURL, ".torrent") {
		return "torrent"
	}
	if strings.Contains(rawURL, ".m3u8") {
		return "live-stream"
	}
	return "http"
}

func buildJob(cfg YAMLJob) (highway.Job, error) {
	prefix, actualURL := parsePrefix(cfg.URL)
	jobType := getJobType(prefix, actualURL, cfg.Type)

	conns := connections // inherited global connections flag
	if cfg.Connections > 0 {
		conns = cfg.Connections
	}

	switch jobType {
	case "http":
		return httpjob.New(actualURL, cfg.Output, conns, globalHTTPConfig), nil

	case "live-stream":
		extract := cfg.Extract
		if extract == "" {
			extract = batchFlags.extract
		}
		if extract == "" {
			if strings.Contains(actualURL, "dailymotion.com") || strings.Contains(actualURL, "dai.ly") {
				extract = "dailymotion"
			} else if strings.Contains(actualURL, "rumble.com") {
				extract = "rumble"
			}
		}
		return m3u8job.New(actualURL, cfg.Output, conns, extract, globalHTTPConfig), nil

	case "github-release":
		man := batchFlags.manual
		if cfg.Manual != nil {
			man = *cfg.Manual
		}
		return ghreleasejob.New(actualURL, cfg.Output, man, globalHTTPConfig), nil

	case "s3":
		prof := cfg.Profile
		if prof == "" {
			prof = batchFlags.s3Profile
		}
		if prof == "" {
			prof = "default"
		}
		return s3job.New(actualURL, cfg.Output, conns, prof), nil

	case "ytdlp":
		cookies := cfg.Cookies
		if cookies == "" {
			cookies = batchFlags.cookies
		}
		cookiesFromBrowser := cfg.CookiesFromBrowser
		if cookiesFromBrowser == "" {
			cookiesFromBrowser = batchFlags.cookiesFromBrowser
		}
		return ytdlpjob.New(actualURL, cfg.Output, cookies, cookiesFromBrowser, globalHTTPConfig), nil

	case "torrent":
		return torrentjob.New(actualURL, cfg.Output, conns, globalHTTPConfig), nil

	default:
		return nil, fmt.Errorf("unsupported job type: %s", jobType)
	}
}

func newBatchCmd() *cobra.Command {
	return batchCmd
}

func init() {
	batchCmd.Flags().StringVar(&batchFlags.cookies, "cookies", "", "File name to read cookies from for yt-dlp")
	batchCmd.Flags().StringVar(&batchFlags.cookiesFromBrowser, "cookies-from-browser", "", "Browser name to load cookies from for yt-dlp")
	batchCmd.Flags().StringVar(&batchFlags.s3Profile, "s3-profile", "default", "AWS profile for S3 downloads")
	batchCmd.Flags().StringVarP(&batchFlags.extract, "extract", "e", "", "Site-specific extractor for live streams")
	batchCmd.Flags().BoolVar(&batchFlags.manual, "manual", false, "Manually select release version and asset for GitHub Releases")
}
