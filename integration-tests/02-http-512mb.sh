#!/usr/bin/env bash
# Manual integration test: large HTTP download (unauthenticated).
# ~512 MB static file with Accept-Ranges (good for multi-chunk danzo http).
#
# link.testfile.org/500MB is often behind Cloudflare (403 for CLI/curl); this URL is a
# long-standing public test file that responds with plain 200 + Content-Length.
set -euo pipefail

readonly DEST="${DANZO_TEST_DEST:-/mnt/usbdrive/danzo-tests}"
readonly DANZO_BIN="${DANZO:-danzo}"
readonly TEST_SIZE="${DANZO_HTTP_SIZE:-50MB}" # e.g. 5MB, 10MB, 50MB, 100MB, 512MB
readonly URL="https://download.thinkbroadband.com/${TEST_SIZE}.zip"
readonly OUT_NAME="${TEST_SIZE}-thinkbroadband.zip"

mkdir -p "$DEST"

echo "==> http integration: static file (${TEST_SIZE})"
echo "    URL:  $URL"
echo "    dest: $DEST/$OUT_NAME"
echo "    (size is configurable via DANZO_HTTP_SIZE, default 50MB)"
"$DANZO_BIN" http "$URL" -o "$DEST/$OUT_NAME"

# Verify the file was downloaded successfully and is not empty
if [ -f "$DEST/$OUT_NAME" ] && [ -s "$DEST/$OUT_NAME" ]; then
    echo "==> Success: File downloaded successfully to $DEST/$OUT_NAME"
    ls -lh "$DEST/$OUT_NAME"
else
    echo "==> Error: Download failed or file is empty!"
    exit 1
fi

echo "==> done"

