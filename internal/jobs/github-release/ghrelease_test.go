package ghrelease

import (
	"runtime"
	"testing"

	"github.com/tanq16/danzo/utils"
)

func TestParseGitHubURLAcceptsSupportedRepositoryForms(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
	}{
		{name: "owner repo", input: "tanq16/danzo", wantOwner: "tanq16", wantRepo: "danzo"},
		{name: "github host", input: "github.com/tanq16/danzo", wantOwner: "tanq16", wantRepo: "danzo"},
		{name: "https URL with suffix", input: "https://github.com/tanq16/danzo/releases/latest", wantOwner: "tanq16", wantRepo: "danzo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseGitHubURL(tt.input)
			if err != nil {
				t.Fatalf("parse github url: %v", err)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Fatalf("expected %s/%s, got %s/%s", tt.wantOwner, tt.wantRepo, owner, repo)
			}
		})
	}
}

func TestParseGitHubURLRejectsMalformedInput(t *testing.T) {
	if owner, repo, err := parseGitHubURL("https://example.com/tanq16/danzo"); err == nil {
		t.Fatalf("expected malformed repo error, got %s/%s", owner, repo)
	}
}

func TestSelectGitHubLatestAssetIgnoresChecksumsAndMatchesRuntimePlatform(t *testing.T) {
	platformKey := runtime.GOOS + runtime.GOARCH
	selectors := assetSelectMap[platformKey]
	if len(selectors) == 0 {
		t.Skipf("no asset selectors configured for %s", platformKey)
	}

	assets := []map[string]any{
		{
			"name":                 "danzo-checksums.txt",
			"browser_download_url": "https://example.com/checksums",
			"size":                 float64(10),
		},
		{
			"name":                 "danzo-" + selectors[0] + ".tar.gz",
			"browser_download_url": "https://example.com/danzo",
			"size":                 float64(42),
		},
	}

	url, size, err := selectGitHubLatestAsset(assets)
	if err != nil {
		t.Fatalf("select asset: %v", err)
	}
	if url != "https://example.com/danzo" || size != 42 {
		t.Fatalf("expected platform asset, got url=%q size=%d", url, size)
	}
}

func TestGHReleaseJobIDIsStable(t *testing.T) {
	job := New("tanq16/danzo", "", false, utils.HTTPClientConfig{})
	initialID := job.ID()
	
	// Simulate what happens in Run() when output path is resolved
	job.OutputPath = "danzo_resolved.tar.gz"
	
	if job.ID() != initialID {
		t.Fatalf("expected ID to be stable (%q), but got %q", initialID, job.ID())
	}
}
