package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple split",
			input:    "https://example.com/file.zip output.zip",
			expected: []string{"https://example.com/file.zip", "output.zip"},
		},
		{
			name:     "split with multiple spaces",
			input:    "https://example.com/file.zip     output.zip",
			expected: []string{"https://example.com/file.zip", "output.zip"},
		},
		{
			name:     "quoted path with spaces",
			input:    `https://example.com/file.zip "my output file name.zip"`,
			expected: []string{"https://example.com/file.zip", "my output file name.zip"},
		},
		{
			name:     "no output path",
			input:    "https://example.com/file.zip",
			expected: []string{"https://example.com/file.zip"},
		},
		{
			name:     "empty line",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLine(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("splitLine() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParsePrefix(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPrefix string
		wantURL    string
	}{
		{
			name:       "with http prefix",
			input:      "http::https://example.com/file.zip",
			wantPrefix: "http",
			wantURL:    "https://example.com/file.zip",
		},
		{
			name:       "with ytdlp prefix",
			input:      "ytdlp::https://youtube.com/watch?v=123",
			wantPrefix: "ytdlp",
			wantURL:    "https://youtube.com/watch?v=123",
		},
		{
			name:       "no prefix",
			input:      "https://example.com/file.zip",
			wantPrefix: "",
			wantURL:    "https://example.com/file.zip",
		},
		{
			name:       "empty input",
			input:      "",
			wantPrefix: "",
			wantURL:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, actualURL := parsePrefix(tt.input)
			if prefix != tt.wantPrefix {
				t.Errorf("parsePrefix() prefix = %q, want %q", prefix, tt.wantPrefix)
			}
			if actualURL != tt.wantURL {
				t.Errorf("parsePrefix() actualURL = %q, want %q", actualURL, tt.wantURL)
			}
		})
	}
}

func TestGetJobType(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		rawURL       string
		overrideType string
		want         string
	}{
		{
			name:         "prefix matches type",
			prefix:       "ytdlp",
			rawURL:       "https://youtube.com/watch?v=123",
			overrideType: "",
			want:         "ytdlp",
		},
		{
			name:         "alias mapping",
			prefix:       "ghr",
			rawURL:       "tanq16/danzo",
			overrideType: "",
			want:         "github-release",
		},
		{
			name:         "override type",
			prefix:       "",
			rawURL:       "https://example.com/file.zip",
			overrideType: "torrent",
			want:         "torrent",
		},
		{
			name:         "s3 auto-detection",
			prefix:       "",
			rawURL:       "s3://mybucket/file.zip",
			overrideType: "",
			want:         "s3",
		},
		{
			name:         "magnet auto-detection",
			prefix:       "",
			rawURL:       "magnet:?xt=urn:btih:123",
			overrideType: "",
			want:         "torrent",
		},
		{
			name:         "m3u8 auto-detection",
			prefix:       "",
			rawURL:       "https://example.com/playlist.m3u8",
			overrideType: "",
			want:         "live-stream",
		},
		{
			name:         "default http fallback",
			prefix:       "",
			rawURL:       "https://example.com/file.zip",
			overrideType: "",
			want:         "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getJobType(tt.prefix, tt.rawURL, tt.overrideType)
			if got != tt.want {
				t.Errorf("getJobType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseBatchInput(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "danzo-batch-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Plain Text input
	txtPath := filepath.Join(tempDir, "jobs.txt")
	txtContent := `# comment
ytdlp::https://youtube.com/watch?v=123 "my video.mp4"
https://example.com/file.zip
`
	if err := os.WriteFile(txtPath, []byte(txtContent), 0644); err != nil {
		t.Fatalf("failed to write txt test file: %v", err)
	}

	jobs, err := parseBatchInput(txtPath)
	if err != nil {
		t.Fatalf("parseBatchInput(txt) failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("parseBatchInput(txt) got %d jobs, want 2", len(jobs))
	}
	if jobs[0].URL != "ytdlp::https://youtube.com/watch?v=123" || jobs[0].Output != "my video.mp4" {
		t.Errorf("parseBatchInput(txt) job 0: %+v", jobs[0])
	}
	if jobs[1].URL != "https://example.com/file.zip" || jobs[1].Output != "" {
		t.Errorf("parseBatchInput(txt) job 1: %+v", jobs[1])
	}

	// 2. YAML input
	yamlPath := filepath.Join(tempDir, "jobs.yaml")
	yamlContent := `- url: "s3::s3://mybucket/file.zip"
  output: "s3file.zip"
  connections: 16
- url: "https://example.com/direct.zip"
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml test file: %v", err)
	}

	jobs, err = parseBatchInput(yamlPath)
	if err != nil {
		t.Fatalf("parseBatchInput(yaml) failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("parseBatchInput(yaml) got %d jobs, want 2", len(jobs))
	}
	if jobs[0].URL != "s3::s3://mybucket/file.zip" || jobs[0].Output != "s3file.zip" || jobs[0].Connections != 16 {
		t.Errorf("parseBatchInput(yaml) job 0: %+v", jobs[0])
	}

	// 3. JSON input
	jsonPath := filepath.Join(tempDir, "jobs.json")
	jsonJobs := []YAMLJob{
		{URL: "ghr::username/repo", Output: "my-asset"},
	}
	jsonData, _ := json.Marshal(jsonJobs)
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		t.Fatalf("failed to write json test file: %v", err)
	}

	jobs, err = parseBatchInput(jsonPath)
	if err != nil {
		t.Fatalf("parseBatchInput(json) failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("parseBatchInput(json) got %d jobs, want 1", len(jobs))
	}
	if jobs[0].URL != "ghr::username/repo" || jobs[0].Output != "my-asset" {
		t.Errorf("parseBatchInput(json) job 0: %+v", jobs[0])
	}
}

func TestBuildJob(t *testing.T) {
	// Test creating various job types
	cfg := YAMLJob{
		URL:         "http::https://example.com/file.zip",
		Output:      "downloaded.zip",
		Connections: 4,
	}

	job, err := buildJob(cfg)
	if err != nil {
		t.Fatalf("buildJob failed: %v", err)
	}
	if job.Type() != "http" {
		t.Errorf("job.Type() = %q, want %q", job.Type(), "http")
	}
	if job.ID() != "downloaded.zip" {
		t.Errorf("job.ID() = %q, want %q", job.ID(), "downloaded.zip")
	}

	cfgS3 := YAMLJob{
		URL:     "s3::mybucket/key",
		Output:  "s3out.zip",
		Profile: "prod",
	}
	jobS3, err := buildJob(cfgS3)
	if err != nil {
		t.Fatalf("buildJob failed: %v", err)
	}
	if jobS3.Type() != "s3" {
		t.Errorf("job.Type() = %q, want %q", jobS3.Type(), "s3")
	}
}

func TestParseBatchInputStdin(t *testing.T) {
	// Test stdin parsing by mocking stdin
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdin = r

	go func() {
		defer w.Close()
		io.WriteString(w, "https://example.com/file.zip output.zip\n")
	}()

	jobs, err := parseBatchInput("")
	if err != nil {
		t.Fatalf("parseBatchInput(stdin) failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("parseBatchInput(stdin) got %d jobs, want 1", len(jobs))
	}
	if jobs[0].URL != "https://example.com/file.zip" || jobs[0].Output != "output.zip" {
		t.Errorf("parseBatchInput(stdin) job 0: %+v", jobs[0])
	}
}
