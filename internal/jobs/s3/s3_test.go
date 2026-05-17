package s3

import "testing"

func TestParseS3URLSupportsBucketObjectsAndPrefixes(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{name: "scheme with nested key", rawURL: "s3://my-bucket/path/to/object.zip", wantBucket: "my-bucket", wantKey: "path/to/object.zip"},
		{name: "bucket only", rawURL: "my-bucket", wantBucket: "my-bucket", wantKey: ""},
		{name: "prefix keeps trailing slash", rawURL: "s3://my-bucket/folder/", wantBucket: "my-bucket", wantKey: "folder/"},
		{name: "missing bucket", rawURL: "s3://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBucket, gotKey, err := parseS3URL(tt.rawURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse s3 url: %v", err)
			}
			if gotBucket != tt.wantBucket || gotKey != tt.wantKey {
				t.Fatalf("expected bucket/key %q/%q, got %q/%q", tt.wantBucket, tt.wantKey, gotBucket, gotKey)
			}
		})
	}
}
