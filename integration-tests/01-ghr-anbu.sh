#!/usr/bin/env bash
# Manual integration test: GitHub release download (unauthenticated, public repo).
# Downloads the latest tanq16/anbu release asset for the current OS/arch.
set -euo pipefail

readonly DEST="${DANZO_TEST_DEST:-/mnt/usbdrive/danzo-tests}"
readonly DANZO_BIN="${DANZO:-danzo}"

mkdir -p "$DEST"
cd "$DEST"

echo "==> ghr integration: tanq16/anbu -> $DEST"
echo "    (asset filename is chosen by danzo from the release, e.g. anbu-linux-amd64.zip)"
"$DANZO_BIN" ghr "tanq16/anbu"
echo "==> done"
