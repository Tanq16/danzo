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

# Verify at least one file matching 'anbu' was downloaded and is non-empty
if ls anbu-* 1>/dev/null 2>&1; then
    echo "==> Success: Asset downloaded successfully!"
    ls -lh anbu-*
else
    echo "==> Error: No anbu release asset was downloaded!"
    exit 1
fi

echo "==> done"

