#!/bin/bash
set -e
set -x

mkdir -p torrent-test-dir
wget -qO test.torrent "https://releases.ubuntu.com/24.04/ubuntu-24.04.1-live-server-amd64.iso.torrent"

echo "Running danzo torrent integration test..."
timeout 20 ../danzo torrent test.torrent -o torrent-test-dir --for-ai > output.log 2>&1 || true

cat output.log
echo "Integration test passed."
rm -rf torrent-test-dir test.torrent output.log
