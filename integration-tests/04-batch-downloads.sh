#!/usr/bin/env bash
# Manual integration test: Batch mode downloading multiple files in parallel.
set -euo pipefail

readonly DEST="${DANZO_TEST_DEST:-.danzo-temp/integration-tests}"
readonly DANZO_BIN="${DANZO:-danzo}"

mkdir -p "$DEST"

echo "==> Preparing batch inputs..."

# 1. Plain-text batch file
readonly TXT_FILE="$DEST/batch_jobs.txt"
cat <<EOF > "$TXT_FILE"
# Text batch download test
https://download.thinkbroadband.com/5MB.zip $DEST/batch-txt-5MB.zip
http::https://download.thinkbroadband.com/10MB.zip $DEST/batch-txt-10MB.zip
EOF

# 2. YAML batch file
readonly YAML_FILE="$DEST/batch_jobs.yaml"
cat <<EOF > "$YAML_FILE"
- url: "https://download.thinkbroadband.com/5MB.zip"
  output: "$DEST/batch-yaml-5MB.zip"
- url: "http::https://download.thinkbroadband.com/10MB.zip"
  output: "$DEST/batch-yaml-10MB.zip"
EOF

echo "==> Running plain-text batch integration test..."
"$DANZO_BIN" batch "$TXT_FILE" --workers 2

# Verify plaintext results
if [ -f "$DEST/batch-txt-5MB.zip" ] && [ -s "$DEST/batch-txt-5MB.zip" ] && \
   [ -f "$DEST/batch-txt-10MB.zip" ] && [ -s "$DEST/batch-txt-10MB.zip" ]; then
    echo "==> Plain-text batch success!"
    ls -lh "$DEST"/batch-txt-*.zip
else
    echo "==> Error: Plain-text batch downloads failed or are empty!"
    exit 1
fi

echo "==> Running YAML batch integration test..."
"$DANZO_BIN" batch "$YAML_FILE" --workers 2

# Verify YAML results
if [ -f "$DEST/batch-yaml-5MB.zip" ] && [ -s "$DEST/batch-yaml-5MB.zip" ] && \
   [ -f "$DEST/batch-yaml-10MB.zip" ] && [ -s "$DEST/batch-yaml-10MB.zip" ]; then
    echo "==> YAML batch success!"
    ls -lh "$DEST"/batch-yaml-*.zip
else
    echo "==> Error: YAML batch downloads failed or are empty!"
    exit 1
fi

echo "==> Running Stdin piped batch integration test..."
echo "https://download.thinkbroadband.com/5MB.zip $DEST/batch-stdin-5MB.zip" | "$DANZO_BIN" batch --workers 1

# Verify Stdin results
if [ -f "$DEST/batch-stdin-5MB.zip" ] && [ -s "$DEST/batch-stdin-5MB.zip" ]; then
    echo "==> Stdin batch success!"
    ls -lh "$DEST"/batch-stdin-*.zip
else
    echo "==> Error: Stdin batch download failed or is empty!"
    exit 1
fi

echo "==> All batch integration tests completed successfully!"
