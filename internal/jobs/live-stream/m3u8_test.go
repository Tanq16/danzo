package m3u8

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/tanq16/danzo/utils"
)

func TestParseM3U8ContentResolvesMediaPlaylistSegmentsAndInitMap(t *testing.T) {
	content := `#EXTM3U
#EXT-X-MAP:URI="init.mp4"
#EXTINF:4.0,
segment-1.ts
#EXTINF:4.0,
../shared/segment-2.ts
`

	info, err := parseM3U8Content(context.Background(), content, "https://cdn.example.com/video/playlist.m3u8", utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}))
	if err != nil {
		t.Fatalf("parse m3u8: %v", err)
	}

	wantSegments := []string{
		"https://cdn.example.com/video/segment-1.ts",
		"https://cdn.example.com/shared/segment-2.ts",
	}
	if !reflect.DeepEqual(info.VideoSegmentURLs, wantSegments) {
		t.Fatalf("expected segments %#v, got %#v", wantSegments, info.VideoSegmentURLs)
	}
	if info.VideoInitSegment != "https://cdn.example.com/video/init.mp4" {
		t.Fatalf("expected init segment URL, got %q", info.VideoInitSegment)
	}
	if info.HasSeparateAudio {
		t.Fatalf("media playlist should not report separate audio")
	}
}

func TestParseM3U8ContentSelectsBestVariantAndAudioTrack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/master.m3u8":
			_, _ = w.Write([]byte(`#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-low",NAME="Low",URI="/audio/low.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-high",NAME="High",URI="/audio/high.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=1000
/video/low.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5000
/video/high.m3u8
`))
		case "/video/high.m3u8":
			_, _ = w.Write([]byte(`#EXTM3U
#EXT-X-MAP:URI="init-video.mp4"
#EXTINF:4.0,
video-high-1.m4s
`))
		case "/audio/high.m3u8":
			_, _ = w.Write([]byte(`#EXTM3U
#EXT-X-MAP:URI="init-audio.mp4"
#EXTINF:4.0,
audio-high-1.m4s
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	master, err := getM3U8Contents(context.Background(), server.URL+"/master.m3u8", utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}))
	if err != nil {
		t.Fatalf("get master: %v", err)
	}
	info, err := parseM3U8Content(context.Background(), master, server.URL+"/master.m3u8", utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}))
	if err != nil {
		t.Fatalf("parse master: %v", err)
	}

	if !info.HasSeparateAudio {
		t.Fatalf("expected separate audio from master playlist")
	}
	if want := []string{server.URL + "/video/video-high-1.m4s"}; !reflect.DeepEqual(info.VideoSegmentURLs, want) {
		t.Fatalf("expected high bandwidth video segments %#v, got %#v", want, info.VideoSegmentURLs)
	}
	if want := []string{server.URL + "/audio/audio-high-1.m4s"}; !reflect.DeepEqual(info.AudioSegmentURLs, want) {
		t.Fatalf("expected high quality audio segments %#v, got %#v", want, info.AudioSegmentURLs)
	}
	if info.VideoInitSegment != server.URL+"/video/init-video.mp4" {
		t.Fatalf("expected video init segment, got %q", info.VideoInitSegment)
	}
	if info.AudioInitSegment != server.URL+"/audio/init-audio.mp4" {
		t.Fatalf("expected audio init segment, got %q", info.AudioInitSegment)
	}
}

func TestParseM3U8ContentHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := parseM3U8Content(ctx, "#EXTM3U\nsegment.ts\n", "https://cdn.example.com/playlist.m3u8", utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestGetDailymotionVideoIDHandlesKnownURLShapes(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "short link", rawURL: "https://dai.ly/x9abc12", want: "x9abc12"},
		{name: "video URL", rawURL: "https://www.dailymotion.com/video/x9abc12_title", want: "x9abc12_title"},
		{name: "query parameter", rawURL: "https://example.com/watch?video=x9abc12&autoplay=1", want: "x9abc12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getDailymotionVideoID(tt.rawURL)
			if err != nil {
				t.Fatalf("extract id: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestGetDailymotionVideoIDRejectsUnknownURLs(t *testing.T) {
	if got, err := getDailymotionVideoID("https://example.com/video/x9abc12"); err == nil {
		t.Fatalf("expected extraction error, got %q", got)
	}
}
