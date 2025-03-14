<div align="center">
  <img src=".github/assets/logo.png" alt="Danzo Logo" width="250">

  <a href="https://github.com/tanq16/danzo/actions/workflows/binary.yml"><img alt="Build Binary" src="https://github.com/tanq16/danzo/actions/workflows/binary.yml/badge.svg"></a> <a href="https://github.com/Tanq16/danzo/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/tanq16/danzo"></a><br><br>
  <a href="#features">Features</a> &bull; <a href="#installation-and-usage">Install & Use</a> &bull; <a href="#tips-and-notes">Tips & Notes</a>
</div>

---

***Danzo*** is a cross-platform and cross-architecture CLI downloader utility designed for fast parallel connections, progress tracking, and an easy to use binary. The tool aims to maximize download speeds by utilizing optimized buffer sizes and parallel processing.

Yes, the name is the same as a Naruto character who has a hobby of collecting many things, reprentative of parallel connections used in this tool.

## Features

- Multi-connection downloads to improve speed
- Automatic chunk size optimization
- Real-time progress display with speed and ETA
- Batch downloading with YAML configuration
- Parallel downloading of multiple files
- Customizable user agent and timeout settings
- Support for HTTP or HTTPS proxies

## Install and Use

### Using Binary

1. Download the appropriate binary for your system from the [latest release](https://github.com/tanq16/danzo/releases/latest)
2. Make the binary executable (Linux/macOS) with `chmod +x danzo-*`
3. Run the binary with:

```bash
./danzo --url "https://example.com/largefile.zip" --output "./downloaded-file.zip"
```

### Using Go

With `Go 1.24+` installed, run the following to download the binary to your GOBIN:

```bash
go install github.com/tanq16/danzo@latest
```

Or, you can build from source like so:

```bash
git clone https://github.com/tanq16/danzo.git
cd danzo
go build .
./danzo --url "https://example.com/largefile.zip" --output "./downloaded-file.zip"
```

### Command Options

```
Danzo is a fast CLI download manager

Usage:
  danzo [flags]
  danzo [command]

Available Commands:
  clean       Clean up temporary files
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command

Flags:
  -c, --connections int               Number of connections per download (default: CPU cores) (default 16)
      --debug                         Enable debug logging
  -h, --help                          help for danzo
  -k, --keep-alive-timeout duration   Keep-alive timeout for client (eg./ 10s, 1m, 80s; default: 90s) (default 1m30s)
  -o, --output string                 Output file path (required with --url/-u)
  -p, --proxy string                  HTTP/HTTPS proxy URL (e.g., proxy.example.com:8080)
  -t, --timeout duration              Connection timeout (eg., 5s, 10m; default: 3m) (default 3m0s)
  -u, --url string                    URL to download
  -l, --urllist string                Path to YAML file containing URLs and output paths
  -a, --user-agent string             User agent (default "Danzo/1337")
  -w, --workers int                   Number of links to download in parallel (default: 1) (default 1)

Use "danzo [command] --help" for more information about a command.
```

### Batch Download

For downloading multiple files, create a YAML file with the following format:

```yaml
- op: "./output1.zip"
  link: "https://example.com/file1.zip"
- op: "./output2.zip"
  link: "https://example.com/file2.zip"
```

Then run as:

```bash
./danzo --urllist "./downloads.yaml"
```

Number of workers and connections per worker can be specified as follows:

```bash
./danzo -l downloads.yaml -w 3 -c 16
```

> [!NOTE]
> Danzo caps the total number of parallel workers at 64. Specifically `# workers * # connections <= 64`. This is a sensible default to prevent overwhelming the system.

### Cleaning Temporary Files

Danzo stores partial downloads on disk in the `.danzo-temp` directory (situated in the same path as the associated output path). If a download event is interrupted or failed, the temporary files can be cleared by specifying the output path like so:

```bash
./danzo clean --output "./downloaded-file.zip"
```

## Tips and Notes

- For optimal download speeds, the number of connections is automatically set to match your CPU cores, but you can adjust this with the -c flag
- Large files benefit the most from multiple connections
- If a download fails, Danzo will retry individual chunks up to 5 times
- For downloading through a proxy, use the `--proxy` or `-p` flag with your proxy URL (you needn't provide the HTTP scheme, Danzo matches it to that of the URL)
- *Not all servers support multi-connection downloads (range requests)*
- For servers with rate limiting, reducing the number of connections might help
- Debug mode (`--debug`) provides detailed information about the download process
- Temporary files are stored in a .danzo-temp directory and automatically cleaned up after download
- Use `-a randomize` to randomize the user agent for every HTTP client
