# Integration tests (manual)

These are **not** Go unit tests. They are **optional, manually run** checks against live networks and real endpoints. Run them when you want to validate downloaders end-to-end on your machine.

## Requirements

- A built `danzo` binary on `PATH`, or set `DANZO` to the full path of the binary.
- The destination directory must exist or be mountable. By default scripts use:

  **`/mnt/usbdrive/danzo-tests`**

  Override with `DANZO_TEST_DEST` if that path is wrong on your system.

- Enough free space on the destination (the 500 MB HTTP test alone needs ~500 MB).

## Unauthenticated downloads

| Script | What it exercises |
|--------|-------------------|
| [`01-ghr-terraform.sh`](./01-ghr-terraform.sh) | `danzo ghr` — latest **Hashicorp Terraform** release asset for this OS/arch (public GitHub API, no token). |
| [`02-http-500mb.sh`](./02-http-500mb.sh) | `danzo http` — **500 MB** file from `link.testfile.org`. |

### How to run

From the repo root:

```bash
chmod +x integration-tests/*.sh   # once
./integration-tests/01-ghr-terraform.sh
./integration-tests/02-http-500mb.sh
```

Or with a custom binary or destination:

```bash
DANZO=/path/to/danzo DANZO_TEST_DEST=/mnt/other/volume/tests ./integration-tests/01-ghr-terraform.sh
```

## Notes

- Scripts use `set -euo pipefail` and abort on failure.
- Outputs land under `DANZO_TEST_DEST` (default `/mnt/usbdrive/danzo-tests`). Clean old artifacts there yourself when done.
