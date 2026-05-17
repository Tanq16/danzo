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
| [`02-http-512mb.sh`](./02-http-512mb.sh) | `danzo http` — **~512 MB** static file (`thinkbroadband.com`), `Accept-Ranges` friendly for multi-chunk downloads. |

**Why `tanq16/anbu`?** Small public repo with predictable per-platform zip assets on GitHub `releases/latest` (good fit for unauthenticated `ghr` checks).

**Why not `link.testfile.org/500MB`?** That host often returns **403** with a Cloudflare challenge for non-browser clients, so it is a poor fit for automated or CLI integration checks.

### How to run

From the repo root:

```bash
chmod +x integration-tests/*.sh   # once
./integration-tests/01-ghr-anbu.sh
./integration-tests/02-http-512mb.sh
```

Or with a custom binary or destination:

```bash
DANZO=/path/to/danzo DANZO_TEST_DEST=/mnt/other/volume/tests ./integration-tests/01-ghr-anbu.sh
```

## Notes

- Scripts use `set -euo pipefail` and abort on failure.
- Outputs land under `DANZO_TEST_DEST` (default `/mnt/usbdrive/danzo-tests`). Clean old artifacts there yourself when done.
