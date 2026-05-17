#!/usr/bin/env bash
# Manual integration test: GitHub release download (unauthenticated, public repo).
# Downloads the latest Terraform release asset for the current OS/arch from hashicorp/terraform.
set -euo pipefail

readonly DEST="${DANZO_TEST_DEST:-/mnt/usbdrive/danzo-tests}"
readonly DANZO_BIN="${DANZO:-danzo}"

mkdir -p "$DEST"
cd "$DEST"

echo "==> ghr integration: hashicorp/terraform -> $DEST"
echo "    (asset filename is chosen by danzo from the release, e.g. terraform_*_linux_amd64.zip)"
"$DANZO_BIN" ghr "hashicorp/terraform"
echo "==> done"
