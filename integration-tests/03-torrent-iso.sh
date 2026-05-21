#!/usr/bin/env bash
# Manual integration test: torrent download.
# Downloads the Ubuntu ISO torrent meta and starts downloading blocks.
set -euo pipefail

readonly DEST="${DANZO_TEST_DEST:-/mnt/usbdrive/danzo-tests}"
readonly DANZO_BIN="${DANZO:-danzo}"
echo "==> torrent integration: discovering latest Ubuntu 24.04 torrent..."
TORRENT_FILENAME=""
if command -v curl >/dev/null 2>&1; then
    TORRENT_FILENAME=$(curl -sSL "https://releases.ubuntu.com/24.04/" | grep -oE "ubuntu-24.04\.[0-9]+-live-server-amd64\.iso\.torrent" | head -n 1 || true)
fi

if [ -z "$TORRENT_FILENAME" ]; then
    echo "    (Dynamic discovery failed or curl not found, falling back to static v24.04.3)"
    TORRENT_FILENAME="ubuntu-24.04.3-live-server-amd64.iso.torrent"
fi

readonly TORRENT_URL="https://releases.ubuntu.com/24.04/$TORRENT_FILENAME"
readonly TORRENT_FILE="$DEST/$TORRENT_FILENAME"
readonly OUT_DIR="$DEST/torrent-test"

mkdir -p "$DEST"
mkdir -p "$OUT_DIR"

echo "==> torrent integration: downloading torrent metadata file..."
echo "    Source: $TORRENT_URL"
if command -v curl >/dev/null 2>&1; then
    curl -sSL -o "$TORRENT_FILE" "$TORRENT_URL"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TORRENT_FILE" "$TORRENT_URL"
else
    echo "==> Error: Neither curl nor wget is available!"
    exit 1
fi

echo "==> Running danzo torrent integration test (15s timeout)..."
echo "    Torrent: $TORRENT_FILE"
echo "    Output:  $OUT_DIR"

# Run torrent download with cross-platform timeout support
# Torrent downloads might keep running, so we cap it to 15s to check peer connection and initial blocks.
if command -v timeout >/dev/null 2>&1; then
    timeout 15 "$DANZO_BIN" torrent "$TORRENT_FILE" -o "$OUT_DIR" --for-ai > "$DEST/torrent_output.log" 2>&1 || true
elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout 15 "$DANZO_BIN" torrent "$TORRENT_FILE" -o "$OUT_DIR" --for-ai > "$DEST/torrent_output.log" 2>&1 || true
else
    # Simple background job runner with sleep and kill for cross-platform compatibility
    "$DANZO_BIN" torrent "$TORRENT_FILE" -o "$OUT_DIR" --for-ai > "$DEST/torrent_output.log" 2>&1 &
    PID=$!
    (sleep 15; kill $PID 2>/dev/null || true) &
    TIMER_PID=$!
    wait $PID 2>/dev/null || true
    kill $TIMER_PID 2>/dev/null || true
fi

echo "==> Output Log:"
if [ -f "$DEST/torrent_output.log" ]; then
    cat "$DEST/torrent_output.log"
else
    echo "    (No log file found)"
fi

echo "==> Success: Torrent integration test executed!"

# Clean up
rm -rf "$OUT_DIR" "$TORRENT_FILE" "$DEST/torrent_output.log"
echo "==> done"
