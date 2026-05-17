package gdrive

import "testing"

func TestExtractFileIDAcceptsCommonDriveURLShapes(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "file URL", rawURL: "https://drive.google.com/file/d/file-123/view?usp=sharing", want: "file-123"},
		{name: "open id URL", rawURL: "https://drive.google.com/open?id=open-456&authuser=0", want: "open-456"},
		{name: "folder URL", rawURL: "https://drive.google.com/drive/folders/folder-789", want: "folder-789"},
		{name: "generic id query", rawURL: "https://example.com/download?id=query-abc", want: "query-abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFileID(tt.rawURL)
			if err != nil {
				t.Fatalf("extract file id: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestExtractFileIDRejectsURLsWithoutIDs(t *testing.T) {
	if got, err := extractFileID("https://drive.google.com/drive/my-drive"); err == nil {
		t.Fatalf("expected missing id error, got id %q", got)
	}
}
