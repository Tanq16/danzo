<div align="center">
  <img src=".github/assets/logo.png" alt="Danzo Logo" width="250">

  <a href="https://github.com/tanq16/danzo/actions/workflows/binary.yml"><img alt="Build Binary" src="https://github.com/tanq16/danzo/actions/workflows/binary.yml/badge.svg"></a> &bull; <a href="https://github.com/Tanq16/danzo/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/tanq16/danzo"></a><br><br>
  <a href="#features">Features</a> &bull; <a href="#installation-and-usage">Install & Use</a> &bull; <a href="#tips-and-notes">Tips & Notes</a>
</div>

---

***Danzo*** is a cross-OS and cross-architecture CLI downloader utility designed for fast parallel connections, progress tracking, and an easy to use binary. The tool aims to maximize download speeds by utilizing optimized buffer sizes.

Yes, the name is the same as a Naruto character who has a hobby of collecting many things, reprentative of parallel connections used in this tool.

## Features

- Multi-connection downloads to improve speed
- Automatic chunk size optimization
- Real-time progress display with speed and ETA
- Smart connection management based on your system's resources
- Customizable user agent and timeout settings
- Resume capability for interrupted downloads (&ast;untested)
- Debug logging for troubleshooting

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
Usage:
  danzo [flags]

Flags:
  -c, --connections int        Number of connections (default: # CPU cores)
      --debug                  Enable debug logging
  -h, --help                   help for danzo
  -o, --output string          Output file path
  -t, --timeout duration       Connection timeout (default 3m0s)
  -u, --url string             URL to download
  -a, --user-agent string      User agent (default "Danzo/1.0")
```

## Tips and Notes

- For optimal download speeds, the number of connections is automatically set to match your CPU cores, but you can adjust this with the `-c` flag
- Large files benefit the most from multiple connections
- If a download fails, Danzo will retry individual chunks up to 3 times
- *Not all servers support multi-connection downloads (range requests)*
- For servers with rate limiting, reducing the number of connections might help
- Debug mode (--debug) provides detailed information about the download process
- Temporary files are stored in a `.danzo-temp` directory and automatically cleaned up after download
- To cancel a download, use Ctrl+C (the temporary files will remain for potential future resumption)
