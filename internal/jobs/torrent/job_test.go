package torrentjob

import (
	"testing"

	"github.com/tanq16/danzo/utils"
)

func TestTorrentJobCompile(t *testing.T) {
	job := New("magnet:?xt=urn:btih:123", "", 0, utils.HTTPClientConfig{})
	_ = job
}
