#!/usr/bin/env bash
# Manual integration test: large HTTP download (unauthenticated).
# ~512 MB static file with Accept-Ranges (good for multi-chunk danzo http).
#
# link.testfile.org/500MB is often behind Cloudflare (403 for CLI/curl); this URL is a
# long-standing public test file that responds with plain 200 + Content-Length.
set -euo pipefail

readonly DEST="${DANZO_TEST_DEST:-/mnt/usbdrive/danzo-tests}"
readonly DANZO_BIN="${DANZO:-danzo}"
readonly URL="https://download.thinkbroadband.com/512MB.zip"
readonly OUT_NAME="512mb-thinkbroadband.zip"

mkdir -p "$DEST"

echo "==> http integration: large binary (~512 MB)"
echo "    URL:  $URL"
echo "    dest: $DEST/$OUT_NAME"
echo "    (this will take a while depending on bandwidth)"
"$DANZO_BIN" http "$URL" -o "$DEST/$OUT_NAME"
echo "==> done"
