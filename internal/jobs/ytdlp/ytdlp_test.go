package ytdlpjob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/utils"
)

func TestParseProgressJSONHandlesTotalAndEstimateFallback(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantOK     bool
		wantCurr   int64
		wantTotal  int64
		descReason string
	}{
		{
			name:      "knownTotalIsPreferred",
			input:     `{"downloaded_bytes":"1024","total_bytes":"5817118","total_bytes_estimate":"9999","status":"downloading"}`,
			wantOK:    true,
			wantCurr:  1024,
			wantTotal: 5817118,
		},
		{
			name:      "fallsBackToEstimateWhenTotalIsNA",
			input:     `{"downloaded_bytes":"2048","total_bytes":"NA","total_bytes_estimate":"4096","status":"downloading"}`,
			wantOK:    true,
			wantCurr:  2048,
			wantTotal: 4096,
		},
		{
			name:      "handlesDecimalsFromYtdlp",
			input:     `{"downloaded_bytes":"235677176","total_bytes":"NA","total_bytes_estimate":"490316784.7586207","status":"downloading"}`,
			wantOK:    true,
			wantCurr:  235677176,
			wantTotal: 490316784,
		},
		{
			name:      "handlesDecimalDownloadedBytes",
			input:     `{"downloaded_bytes":"12345.67","total_bytes":"50000.99","total_bytes_estimate":"NA","status":"downloading"}`,
			wantOK:    true,
			wantCurr:  12345,
			wantTotal: 50000,
		},
		{
			name:   "rejectedWhenBothTotalsAreNA",
			input:  `{"downloaded_bytes":"1024","total_bytes":"NA","total_bytes_estimate":"NA","status":"downloading"}`,
			wantOK: false,
		},
		{
			name:   "rejectedWhenInputIsNotJSON",
			input:  `not even json`,
			wantOK: false,
		},
		{
			name:   "rejectedWhenTotalIsZero",
			input:  `{"downloaded_bytes":"0","total_bytes":"0","total_bytes_estimate":"NA","status":"downloading"}`,
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCurr, gotTotal, gotOK := parseProgressJSON(tc.input)
			if gotOK != tc.wantOK {
				t.Fatalf("ok mismatch: got %v want %v", gotOK, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if gotCurr != tc.wantCurr {
				t.Errorf("current: got %d want %d", gotCurr, tc.wantCurr)
			}
			if gotTotal != tc.wantTotal {
				t.Errorf("total: got %d want %d", gotTotal, tc.wantTotal)
			}
		})
	}
}

func TestStreamOutputEmitsProgressAndSuppressesAfterMerge(t *testing.T) {
	const jobID = "out.mp4"
	stream := strings.Join([]string{
		`[download] Destination: out.mp4`,
		`JSON_PROGRESS: {"downloaded_bytes":"1024","total_bytes":"4096","total_bytes_estimate":"NA","status":"downloading"}`,
		`JSON_PROGRESS: bogus json should be skipped`,
		`JSON_PROGRESS: {"downloaded_bytes":"NA","total_bytes":"NA","total_bytes_estimate":"NA","status":"downloading"}`,
		`JSON_PROGRESS: {"downloaded_bytes":"4096","total_bytes":"4096","total_bytes_estimate":"NA","status":"finished"}`,
		`[Merger] Merging formats into "out.mp4"`,
		`JSON_PROGRESS: {"downloaded_bytes":"512","total_bytes":"512","total_bytes_estimate":"NA","status":"downloading"}`,
	}, "\n") + "\n"

	progressCh := make(chan highway.Progress, 32)
	if err := streamOutput(jobID, strings.NewReader(stream), progressCh); err != nil {
		t.Fatalf("streamOutput returned error: %v", err)
	}
	close(progressCh)

	var (
		gotProgress []highway.Progress
		gotSub      []highway.Progress
	)
	for p := range progressCh {
		switch p.Type {
		case highway.ProgressTypeProgress:
			gotProgress = append(gotProgress, p)
		case highway.ProgressTypeSubStatus:
			gotSub = append(gotSub, p)
		}
	}

	if got, want := len(gotProgress), 2; got != want {
		t.Fatalf("progress events: got %d want %d (%+v)", got, want, gotProgress)
	}
	if gotProgress[0].Current != 1024 || gotProgress[0].Total != 4096 {
		t.Errorf("first progress: got %d/%d want 1024/4096", gotProgress[0].Current, gotProgress[0].Total)
	}
	if gotProgress[1].Current != 4096 || gotProgress[1].Total != 4096 {
		t.Errorf("second progress: got %d/%d want 4096/4096", gotProgress[1].Current, gotProgress[1].Total)
	}
	if got, want := len(gotSub), 1; got != want {
		t.Fatalf("substatus events: got %d want %d", got, want)
	}
	if gotSub[0].Message != "Merging" || gotSub[0].SubStatus == "" {
		t.Errorf("merging substatus: got %+v", gotSub[0])
	}
}

func TestStreamOutputEmitsMergingOnlyOnce(t *testing.T) {
	const jobID = "out.mp4"
	stream := strings.Join([]string{
		`[Merger] Merging formats into "out.mp4"`,
		`[Merger] Merging formats into "out.mp4"`,
		`Merging formats one more time`,
	}, "\n") + "\n"

	progressCh := make(chan highway.Progress, 8)
	if err := streamOutput(jobID, strings.NewReader(stream), progressCh); err != nil {
		t.Fatalf("streamOutput returned error: %v", err)
	}
	close(progressCh)

	count := 0
	for range progressCh {
		count++
	}
	if count != 1 {
		t.Fatalf("merging substatus emit count: got %d want 1", count)
	}
}

func TestLastErrorLinePrefersErrorPrefixOverTrailingNoise(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name:   "picksErrorLineEvenIfNotLast",
			stderr: "ERROR: Unsupported URL: foo\nWARNING: cookies expired\n\n",
			want:   "Unsupported URL: foo",
		},
		{
			name:   "fallsBackToLastNonEmptyLine",
			stderr: "warn one\n\nwarn two\n",
			want:   "warn two",
		},
		{
			name:   "emptyStderrYieldsEmptyString",
			stderr: "   \n\n",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastErrorLine(tc.stderr); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestIDFallsBackToURLWhenOutputPathIsEmpty(t *testing.T) {
	if id := New("https://example.com/v.mp4", "", utils.HTTPClientConfig{}).ID(); id != "https://example.com/v.mp4" {
		t.Errorf("empty OutputPath: got %q want URL", id)
	}
	if id := New("https://example.com/v.mp4", "vid.mp4", utils.HTTPClientConfig{}).ID(); id != "vid.mp4" {
		t.Errorf("OutputPath set: got %q want %q", id, "vid.mp4")
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	src := New("https://example.com/v.mp4", "out/vid.mp4", utils.HTTPClientConfig{
		ProxyURL:  "http://proxy:8080",
		UserAgent: "TestUA",
		Headers:   map[string]string{"X-Test": "value"},
	})
	data, err := src.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	job, ok := got.(*YTDLPJob)
	if !ok {
		t.Fatalf("unmarshal returned wrong type: %T", got)
	}
	if job.URL != src.URL || job.OutputPath != src.OutputPath {
		t.Errorf("round trip mismatch: got %+v want %+v", job, src)
	}
	if job.HTTPConfig.ProxyURL != src.HTTPConfig.ProxyURL || job.HTTPConfig.UserAgent != src.HTTPConfig.UserAgent || job.HTTPConfig.Headers["X-Test"] != "value" {
		t.Errorf("round trip HTTP config mismatch: got %+v want %+v", job.HTTPConfig, src.HTTPConfig)
	}
	if job.Type() != "ytdlp" {
		t.Errorf("type: got %q want ytdlp", job.Type())
	}
}

func TestRunSurfacesStderrTailWhenBinaryExitsNonZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh stub")
	}
	stub := writeShellStub(t, `#!/bin/sh
echo "JSON_PROGRESS: {\"downloaded_bytes\":\"10\",\"total_bytes\":\"100\",\"total_bytes_estimate\":\"NA\",\"status\":\"downloading\"}"
echo "ERROR: Unsupported URL: bad" 1>&2
exit 1
`)
	swap := swapBinary(t, stub)
	defer swap()

	progressCh := make(chan highway.Progress, 8)
	job := New("https://example.com/bad", filepath.Join(t.TempDir(), "out.mp4"), utils.HTTPClientConfig{})
	err := job.Run(context.Background(), progressCh)
	close(progressCh)
	if err == nil {
		t.Fatal("expected error from stub failure")
	}
	if !strings.Contains(err.Error(), "Unsupported URL: bad") {
		t.Errorf("error does not surface stderr tail: %v", err)
	}
	if !strings.Contains(err.Error(), "yt-dlp failed") {
		t.Errorf("error missing yt-dlp prefix: %v", err)
	}

	var emittedDone bool
	for p := range progressCh {
		if p.Done {
			emittedDone = true
		}
	}
	if emittedDone {
		t.Errorf("Run should NOT emit Done on failure; highway handles that")
	}
}

func TestRunStreamsAllProgressAndCompletesOnStubSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh stub")
	}
	stub := writeShellStub(t, `#!/bin/sh
echo "[download] Destination: $4"
echo "JSON_PROGRESS: {\"downloaded_bytes\":\"50\",\"total_bytes\":\"100\",\"total_bytes_estimate\":\"NA\",\"status\":\"downloading\"}"
echo "JSON_PROGRESS: {\"downloaded_bytes\":\"100\",\"total_bytes\":\"100\",\"total_bytes_estimate\":\"NA\",\"status\":\"finished\"}"
exit 0
`)
	swap := swapBinary(t, stub)
	defer swap()

	progressCh := make(chan highway.Progress, 16)
	outPath := filepath.Join(t.TempDir(), "out.mp4")
	job := New("https://example.com/ok", outPath, utils.HTTPClientConfig{})
	if err := job.Run(context.Background(), progressCh); err != nil {
		t.Fatalf("Run: %v", err)
	}
	close(progressCh)

	var progressCount int
	var lastDone bool
	for p := range progressCh {
		if p.Done {
			lastDone = true
			continue
		}
		if p.Type == highway.ProgressTypeProgress {
			progressCount++
		}
	}
	if progressCount != 2 {
		t.Errorf("progress events: got %d want 2", progressCount)
	}
	if !lastDone {
		t.Errorf("expected Done emitted on success")
	}
}

func TestRunRenamesExistingNonTemplateOutputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh stub")
	}
	stub := writeShellStub(t, `#!/bin/sh
echo "JSON_PROGRESS: {\"downloaded_bytes\":\"1\",\"total_bytes\":\"1\",\"total_bytes_estimate\":\"NA\",\"status\":\"finished\"}"
exit 0
`)
	swap := swapBinary(t, stub)
	defer swap()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "vid.mp4")
	if err := os.WriteFile(outPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	job := New("https://example.com/v", outPath, utils.HTTPClientConfig{})
	progressCh := make(chan highway.Progress, 8)
	if err := job.Run(context.Background(), progressCh); err != nil {
		t.Fatalf("Run: %v", err)
	}
	close(progressCh)

	want := filepath.Join(dir, "vid-(1).mp4")
	if job.OutputPath != want {
		t.Errorf("OutputPath: got %q want %q", job.OutputPath, want)
	}
}

func TestRunReportsStartupErrorWhenBinaryMissing(t *testing.T) {
	swap := swapBinary(t, "/nonexistent/path/yt-dlp-xyz")
	defer swap()

	progressCh := make(chan highway.Progress, 4)
	job := New("https://example.com/v", filepath.Join(t.TempDir(), "x.mp4"), utils.HTTPClientConfig{})
	err := job.Run(context.Background(), progressCh)
	close(progressCh)

	if err == nil {
		t.Fatal("expected error when binary missing")
	}
	if !strings.Contains(err.Error(), "error starting yt-dlp") {
		t.Errorf("error should mention start failure: %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("error should not be context.Canceled: %v", err)
	}
}

func writeShellStub(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ytdlp-stub.sh")
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func swapBinary(t *testing.T, newPath string) func() {
	t.Helper()
	old := ytdlpBinary
	ytdlpBinary = newPath
	return func() { ytdlpBinary = old }
}
