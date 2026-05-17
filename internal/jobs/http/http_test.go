package danzohttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tanq16/danzo/utils"
)

func TestGetFileInfoExtractsSafeFilenameAndSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("expected HEAD request, got %s", r.Method)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "12345")
		w.Header().Set("Content-Disposition", `attachment; filename="bad:name?.zip"`)
	}))
	defer server.Close()

	size, filename, err := getFileInfo(context.Background(), server.URL, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}))
	if err != nil {
		t.Fatalf("get file info: %v", err)
	}
	if size != 12345 {
		t.Fatalf("expected size 12345, got %d", size)
	}
	if filename != "bad_name_.zip" {
		t.Fatalf("expected sanitized filename, got %q", filename)
	}
}

func TestGetFileInfoSeparatesRangeSupportFromFilenameDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="single.bin"`)
		w.Header().Set("Content-Length", "99")
	}))
	defer server.Close()

	size, filename, err := getFileInfo(context.Background(), server.URL, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}))
	if !errors.Is(err, utils.ErrRangeRequestsNotSupported) {
		t.Fatalf("expected range support error, got %v", err)
	}
	if size != 0 || filename != "single.bin" {
		t.Fatalf("expected filename with zero size on no range support, got size=%d filename=%q", size, filename)
	}
}

func TestGetFileInfoRejectsInvalidContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "not-a-number")
	}))
	defer server.Close()

	_, _, err := getFileInfo(context.Background(), server.URL, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}))
	if err == nil {
		t.Fatalf("expected invalid content length error")
	}
}

func TestDownloadSingleChunkWritesExpectedRangeAndReportsProgress(t *testing.T) {
	const body = "hello"
	var gotRange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 0-4/5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	dir := t.TempDir()
	tempFile := filepath.Join(dir, "chunk.part0")
	job := &HTTPDownloadJob{Config: HTTPDownloadConfig{URL: server.URL}}
	chunk := &HTTPDownloadChunk{ID: 0, StartByte: 0, EndByte: 4}
	progressCh := make(chan int64, 10)

	err := downloadSingleChunk(context.Background(), job, chunk, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}), tempFile, progressCh, 0)
	if err != nil {
		t.Fatalf("download chunk: %v", err)
	}
	if gotRange != "bytes=0-4" {
		t.Fatalf("expected range header bytes=0-4, got %q", gotRange)
	}
	data, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("expected written body %q, got %q", body, string(data))
	}
	if chunk.Downloaded != int64(len(body)) {
		t.Fatalf("expected chunk downloaded %d, got %d", len(body), chunk.Downloaded)
	}
	close(progressCh)
	var total int64
	for n := range progressCh {
		total += n
	}
	if total != int64(len(body)) {
		t.Fatalf("expected progress total %d, got %d", len(body), total)
	}
}

func TestDownloadSingleChunkDetectsTruncatedBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-4/5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("abc"))
	}))
	defer server.Close()

	job := &HTTPDownloadJob{Config: HTTPDownloadConfig{URL: server.URL}}
	chunk := &HTTPDownloadChunk{ID: 0, StartByte: 0, EndByte: 4}
	progressCh := make(chan int64, 10)

	err := downloadSingleChunk(context.Background(), job, chunk, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}), filepath.Join(t.TempDir(), "chunk.part0"), progressCh, 0)
	if err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("expected size mismatch error, got %v", err)
	}
}

func TestAssembleFileOrdersChunksAndValidatesTotalSize(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "joined.txt")
	part0 := filepath.Join(dir, "joined.txt.part0")
	part1 := filepath.Join(dir, "joined.txt.part1")
	if err := os.WriteFile(part0, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part1, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	job := HTTPDownloadJob{
		Config:    HTTPDownloadConfig{OutputPath: outputPath},
		FileSize:  10,
		TempFiles: []string{part1, part0},
		Chunks: []HTTPDownloadChunk{
			{ID: 0, Completed: true},
			{ID: 1, Completed: true},
		},
	}

	if err := assembleFile(job); err != nil {
		t.Fatalf("assemble file: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "helloworld" {
		t.Fatalf("expected ordered chunks, got %q", string(data))
	}
}

func TestAssembleFileRejectsIncompleteChunks(t *testing.T) {
	job := HTTPDownloadJob{
		Config: HTTPDownloadConfig{OutputPath: filepath.Join(t.TempDir(), "joined.txt")},
		Chunks: []HTTPDownloadChunk{
			{ID: 0, Completed: true},
			{ID: 1, Completed: false},
		},
	}

	if err := assembleFile(job); err == nil {
		t.Fatalf("expected incomplete chunks error")
	}
}

func TestChunkedDownloadResumesPartialTempFileWithoutDoubleCounting(t *testing.T) {
	var gotRange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 3-4/5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("lo"))
	}))
	defer server.Close()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "asset.bin")
	tempDir := filepath.Join(dir, ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatal(err)
	}
	tempFile := filepath.Join(tempDir, "asset.bin.part0")
	if err := os.WriteFile(tempFile, []byte("hel"), 0644); err != nil {
		t.Fatal(err)
	}

	job := &HTTPDownloadJob{Config: HTTPDownloadConfig{URL: server.URL, OutputPath: outputPath}}
	chunk := &HTTPDownloadChunk{ID: 0, StartByte: 0, EndByte: 4}
	progressCh := make(chan int64, 16)

	if err := chunkedDownload(context.Background(), job, chunk, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}), progressCh, &sync.Mutex{}); err != nil {
		t.Fatalf("chunked download: %v", err)
	}
	close(progressCh)

	if gotRange != "bytes=3-4" {
		t.Fatalf("expected resume range header bytes=3-4, got %q", gotRange)
	}
	if !chunk.Completed || chunk.Downloaded != 5 {
		t.Fatalf("expected resumed chunk to complete with 5 bytes, got completed=%v downloaded=%d", chunk.Completed, chunk.Downloaded)
	}

	var total int64
	for n := range progressCh {
		total += n
	}
	if total != 5 {
		t.Fatalf("expected progress deltas to sum to chunk size 5 (resume offset + new bytes), got %d", total)
	}

	data, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected resumed temp file to contain full chunk %q, got %q", "hello", string(data))
	}
}

func TestChunkedDownloadProgressNeverOvershootsAcrossRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("relies on retry backoff sleeps")
	}

	const body = "hello"
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "missing range", http.StatusBadRequest)
			return
		}
		requested := body[start : end+1]
		// First two attempts drop one byte (or the whole response when the
		// range is a single byte) to force retries that must resume from disk.
		if n < 3 {
			if len(requested) <= 1 {
				requested = ""
			} else {
				requested = requested[:len(requested)-1]
			}
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(requested))
	}))
	defer server.Close()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".danzo-temp"), 0755); err != nil {
		t.Fatal(err)
	}
	job := &HTTPDownloadJob{Config: HTTPDownloadConfig{URL: server.URL, OutputPath: filepath.Join(dir, "asset.bin")}}
	chunk := &HTTPDownloadChunk{ID: 0, StartByte: 0, EndByte: int64(len(body) - 1)}
	progressCh := make(chan int64, 64)

	var total, maxObserved int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for n := range progressCh {
			total += n
			if total > maxObserved {
				maxObserved = total
			}
		}
	}()

	err := chunkedDownload(context.Background(), job, chunk, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}), progressCh, &sync.Mutex{})
	close(progressCh)
	<-done

	if err != nil {
		t.Fatalf("chunked download after retries: %v", err)
	}
	if !chunk.Completed {
		t.Fatalf("chunk should be marked completed after a successful retry")
	}
	if total != int64(len(body)) {
		t.Fatalf("expected progress sum to equal chunk size %d, got %d", len(body), total)
	}
	if maxObserved > int64(len(body)) {
		t.Fatalf("running progress total should never exceed chunk size %d, peaked at %d", len(body), maxObserved)
	}
	if attempts.Load() < 3 {
		t.Fatalf("expected at least 3 server attempts to exercise retry path, got %d", attempts.Load())
	}
}

func TestSimpleDownloadProgressNeverOvershootsAcrossRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("relies on retry backoff sleeps")
	}

	const body = "hello"
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if rangeHdr := r.Header.Get("Range"); rangeHdr != "" {
			var start int64
			if _, err := fmt.Sscanf(rangeHdr, "bytes=%d-", &start); err != nil {
				http.Error(w, "bad range", http.StatusBadRequest)
				return
			}
			payload := body[start:]
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(payload))
			return
		}
		if n < 2 {
			// First attempt advertises the full Content-Length but drops the
			// connection mid-body, forcing the client to error and the next
			// attempt to resume from the partial temp file.
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijack unavailable", http.StatusInternalServerError)
				return
			}
			conn, buf, err := hj.Hijack()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(buf, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(body))
			_, _ = buf.WriteString(body[:3])
			_ = buf.Flush()
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "asset.bin")
	progressCh := make(chan int64, 64)

	var total, maxObserved int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for n := range progressCh {
			total += n
			if total > maxObserved {
				maxObserved = total
			}
		}
	}()

	if err := PerformSimpleDownload(context.Background(), server.URL, outputPath, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}), progressCh); err != nil {
		t.Fatalf("simple download after retry: %v", err)
	}
	<-done

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("expected assembled body %q, got %q", body, string(data))
	}
	if total != int64(len(body)) {
		t.Fatalf("expected progress sum to equal body size %d, got %d", len(body), total)
	}
	if maxObserved > int64(len(body)) {
		t.Fatalf("running progress total should never exceed body size %d, peaked at %d", len(body), maxObserved)
	}
	if attempts.Load() < 2 {
		t.Fatalf("expected at least 2 server attempts to exercise resume path, got %d", attempts.Load())
	}
}

func TestChunkedDownloadTreatsCompletedPartialFileAsProgress(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "asset.bin")
	tempDir := filepath.Join(dir, ".danzo-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatal(err)
	}
	tempFile := filepath.Join(tempDir, "asset.bin.part0")
	if err := os.WriteFile(tempFile, []byte("ready"), 0644); err != nil {
		t.Fatal(err)
	}

	job := &HTTPDownloadJob{Config: HTTPDownloadConfig{OutputPath: outputPath}}
	chunk := &HTTPDownloadChunk{ID: 0, StartByte: 0, EndByte: 4}
	progressCh := make(chan int64, 1)

	if err := chunkedDownload(context.Background(), job, chunk, utils.NewDanzoHTTPClient(utils.HTTPClientConfig{}), progressCh, &sync.Mutex{}); err != nil {
		t.Fatalf("chunked download: %v", err)
	}
	if !chunk.Completed || chunk.Downloaded != 5 {
		t.Fatalf("expected completed resumed chunk, got completed=%v downloaded=%d", chunk.Completed, chunk.Downloaded)
	}
	if len(job.TempFiles) != 1 || job.TempFiles[0] != tempFile {
		t.Fatalf("expected temp file to be registered, got %#v", job.TempFiles)
	}
	if got := <-progressCh; got != 5 {
		t.Fatalf("expected resumed progress 5, got %d", got)
	}
}
