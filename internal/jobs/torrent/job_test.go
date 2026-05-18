package torrentjob

import (
	"context"
	"strings"
	"testing"

	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/utils"
)

func TestTorrentJobID(t *testing.T) {
	tests := []struct {
		name       string
		uri        string
		outputPath string
		want       string
	}{
		{
			name:       "Magnet link with no output path",
			uri:        "magnet:?xt=urn:btih:12345",
			outputPath: "",
			want:       "magnet-link",
		},
		{
			name:       "Torrent file with no output path",
			uri:        "https://example.com/file.torrent",
			outputPath: "",
			want:       "file.torrent",
		},
		{
			name:       "Magnet link with explicit output path",
			uri:        "magnet:?xt=urn:btih:12345",
			outputPath: "my-download-dir",
			want:       "my-download-dir",
		},
		{
			name:       "Magnet link with dot output path",
			uri:        "magnet:?xt=urn:btih:12345",
			outputPath: ".",
			want:       "magnet-link", // dot is ignored for ID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := New(tt.uri, tt.outputPath, 0, utils.HTTPClientConfig{})
			if got := job.ID(); got != tt.want {
				t.Errorf("ID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTorrentJobInvalidURI(t *testing.T) {
	// Provide a completely invalid URI or non-existent file for a torrent, to verify it fails fast
	job := New("not-a-magnet-or-file.torrent", "", 0, utils.HTTPClientConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	progressCh := make(chan highway.Progress, 10)

	err := job.Run(ctx, progressCh)
	if err == nil {
		t.Error("expected error for invalid torrent URI, got nil")
	}

	// Ensure error string complains about file addition
	if err != nil && !strings.Contains(err.Error(), "failed to add torrent") {
		t.Errorf("expected 'failed to add torrent' in error message, got: %v", err)
	}
}
