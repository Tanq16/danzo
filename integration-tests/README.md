# Integration tests (manual)

These are **not** Go unit tests. They are **optional, manually run** checks against live networks and real endpoints. Run them when you want to validate downloaders end-to-end on your machine.

## Requirements

- A built `danzo` binary on `PATH`, or set `DANZO` to the full path of the binary.
- The destination directory must exist or be mountable. By default scripts use:

  **`/mnt/usbdrive/danzo-tests`**

  Override with `DANZO_TEST_DEST` if that path is wrong on your system.

- Enough free space on the destination (the large HTTP test needs **~512 MB**).

## Unauthenticated downloads

| Script | What it exercises |
|--------|-------------------|
| [`01-ghr-anbu.sh`](./01-ghr-anbu.sh) | `danzo ghr` — latest **anbu** (`tanq16/anbu`) release asset for this OS/arch (public GitHub API, no token). |
| [`02-http-512mb.sh`](./02-http-512mb.sh) | `danzo http` — static file (`thinkbroadband.com`), configurable via `DANZO_HTTP_SIZE` (default: `50MB`). `Accept-Ranges` friendly for multi-chunk downloads. |
| [`03-torrent-iso.sh`](./03-torrent-iso.sh) | `danzo torrent` — downloads metadata, resolves peers, and tests downloading a live Ubuntu ISO via BitTorrent with a 15-second cap. |

**Why `tanq16/anbu`?** Small public repo with predictable per-platform zip assets on GitHub `releases/latest` (good fit for unauthenticated `ghr` checks).

**Why not `link.testfile.org/500MB`?** That host often returns **403** with a Cloudflare challenge for non-browser clients, so it is a poor fit for automated or CLI integration checks.

### How to run

From the repo root, run the scripts with absolute paths to ensure target locations remain correct when changing directories:

```bash
chmod +x integration-tests/*.sh   # once

# Run GitHub Release test
DANZO=$(pwd)/danzo DANZO_TEST_DEST=$(pwd)/.danzo-temp/integration-tests ./integration-tests/01-ghr-anbu.sh

# Run HTTP download test (configurable size, e.g., 10MB, 50MB, 512MB)
DANZO=$(pwd)/danzo DANZO_TEST_DEST=$(pwd)/.danzo-temp/integration-tests DANZO_HTTP_SIZE=10MB ./integration-tests/02-http-512mb.sh

# Run Torrent download test (with robust 15s timeout limit)
DANZO=$(pwd)/danzo DANZO_TEST_DEST=$(pwd)/.danzo-temp/integration-tests ./integration-tests/03-torrent-iso.sh
```

## Notes

- Scripts use `set -euo pipefail` and abort on failure.
- Outputs land under `DANZO_TEST_DEST` (default `/mnt/usbdrive/danzo-tests`).
- Active validation checks are performed at the end of each script to confirm files were successfully created and are non-empty.

