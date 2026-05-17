#!/usr/bin/env bash
# Manual integration test: large HTTP download (unauthenticated).
# Fetches a 500 MB test file; uses multi-chunk behavior depending on server and danzo flags.
set -euo pipefail

readonly DEST="${DANZO_TEST_DEST:-/mnt/usbdrive/danzo-tests}"
readonly DANZO_BIN="${DANZO:-danzo}"
readonly URL="https://link.testfile.org/500MB"
readonly OUT_NAME="500mb-testfile.bin"

mkdir -p "$DEST"

echo "==> http integration: 500 MB test file"
echo "    URL:  $URL"
echo "    dest: $DEST/$OUT_NAME"
echo "    (this will take a while depending on bandwidth)"
"$DANZO_BIN" http "$URL" -o "$DEST/$OUT_NAME"
echo "==> done"
