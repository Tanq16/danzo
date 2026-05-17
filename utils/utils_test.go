package utils

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseHeaderArgsKeepsOnlyUsableHeaders(t *testing.T) {
	headers := ParseHeaderArgs([]string{
		"Authorization: Bearer token:with:colons",
		" X-Trace-ID : abc123 ",
		"not-a-header",
	})

	if headers["Authorization"] != "Bearer token:with:colons" {
		t.Fatalf("expected authorization value to preserve extra colons, got %q", headers["Authorization"])
	}
	if headers["X-Trace-ID"] != "abc123" {
		t.Fatalf("expected trimmed trace header, got %q", headers["X-Trace-ID"])
	}
	if _, ok := headers["not-a-header"]; ok {
		t.Fatalf("malformed header should not be included: %#v", headers)
	}
}

func TestRenewOutputPathSkipsExistingSequentialNames(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "archive.tar.gz")
	first := filepath.Join(dir, "archive.tar-(1).gz")
	for _, path := range []string{original, first} {
		if err := os.WriteFile(path, []byte("exists"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got := RenewOutputPath(original)
	want := filepath.Join(dir, "archive.tar-(2).gz")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormatSpeedHandlesZeroElapsedAndReadableUnits(t *testing.T) {
	if got := FormatSpeed(2048, 2); got != "1.00 KB/s" {
		t.Fatalf("expected formatted speed, got %q", got)
	}
	if got := FormatSpeed(2048, 0); got != "0 B/s" {
		t.Fatalf("expected zero elapsed speed guard, got %q", got)
	}
}

func TestDanzoHTTPClientAppliesDefaultAndCustomHeaders(t *testing.T) {
	var gotUserAgent, gotTrace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotTrace = r.Header.Get("X-Trace-ID")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewDanzoHTTPClient(HTTPClientConfig{
		UserAgent: "Danzo-Test",
		Headers: map[string]string{
			"X-Trace-ID": "abc123",
		},
	})
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotUserAgent != "Danzo-Test" {
		t.Fatalf("expected configured user agent, got %q", gotUserAgent)
	}
	if gotTrace != "abc123" {
		t.Fatalf("expected configured trace header, got %q", gotTrace)
	}
}
