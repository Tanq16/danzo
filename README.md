<div align="center">
  <img src=".github/assets/logo.png" alt="Danzo Logo" width="300">

  <a href="https://github.com/tanq16/danzo/actions/workflows/binary.yml"><img alt="Build" src="https://github.com/tanq16/danzo/actions/workflows/binary.yml/badge.svg"></a> <a href="https://github.com/Tanq16/danzo/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/tanq16/danzo"></a><br><br>
  <a href="#features">Features</a> &bull; <a href="#installation">Installation</a> &bull; <a href="#usage">Usage</a> &bull; <a href="#tips-and-notes">Tips & Notes</a>
</div>

---

***Danzo*** is a cross-platform and cross-architecture CLI downloader utility designed for multi-threaded downloads, progress tracking, and an intuitive command structure. Danzo maximizes download speeds by using a large number of goroutines.

*Side note - yes, the name is the same as a Naruto character with a hobby of collecting and using multiple "items", reprentative of parallel connections used in this tool.*

## Features

- Multiple connection threads for high speed downloads and assembly
  - Temporary directory for chunk downloads
  - Automatic cleanup of temporary files
  - Manual cleanup of temporary files in case of failures
- Automatic optimization of chunk size vs. threads
  - Direct single-threaded download preference for small chunk sizes
  - Fallback to single thread operation for lack of byte-range support
  - Automatic configuration of TCP dialer high-thread mode (>5 connection threads)
- Real-time rotating progress display with average speed and ETA
- Multi-worker (second threading layer) batch file downloads with a YAML config
- Customizable download parameters
  - Custom or randomized user angent strings
  - Custom timeout settings
  - Configurable worker and connection threads (capped at 64 total)
- Support for HTTP or HTTPS proxies
- Configurable (optional) output filenames
  - Automatic numbering of existing names (for single direct URL downloads only)
  - Automatic output name inference from URL path or Content Disposition headers
- Resumable downloads for both single-threaded and multi-threaded downloads
- Resumable Google Drive downloads through API keys and OAuth2.0 credential keys

## Installation

### Release Binary (Recommended)

1. Download the appropriate binary for your system from the [latest release](https://github.com/tanq16/danzo/releases/latest)
2. Unzip the file and make the binary executable (Linux/macOS) with `chmod +x danzo`
3. Run the binary:

```bash
danzo "https://example.com/largefile.zip"
```

### Using Go

With `Go 1.24+` installed, run the following to download the binary to your GOBIN:

```bash
go install github.com/tanq16/danzo@latest
```

Or, you can build from source like so:

```bash
git clone https://github.com/tanq16/danzo.git && cd danzo
go build .
```

### Command Options

The command line options can be printed with `danzo -h`:

```
Danzo is a fast CLI download manager

Usage:
  danzo [flags]

Flags:
      --clean                         Clean up temporary files for provided output path
  -c, --connections int               Number of connections per download (default 8, i.e., high thread mode) (default 8)
      --debug                         Enable debug logging
  -h, --help                          help for danzo
  -k, --keep-alive-timeout duration   Keep-alive timeout for client (eg. 10s, 1m, 80s) (default 1m30s)
  -o, --output string                 Output file path (Danzo infers file name if not provided)
  -p, --proxy string                  HTTP/HTTPS proxy URL (e.g., proxy.example.com:8080)
  -t, --timeout duration              Connection timeout (eg. 5s, 10m) (default 3m0s)
  -l, --urllist string                Path to YAML file containing URLs and output paths
  -a, --user-agent string             User agent (default "danzo/1337")
  -v, --version                       version for danzo
  -w, --workers int                   Number of links to download in parallel (default 1)
```

## Usage

### Basic Usage

The simplest way to download a file is to provide a URL directly:

```bash
danzo https://example.com/largefile.zip
```

The output filename will be inferred from the URL and Danzo will use 4 connection threads by default. You can also specify an output filename manually with:

```bash
danzo https://example.com/largefile.zip -o ./path/to/file.zip
```

> [!NOTE]
> The value for `-c` can go upto `64` for a single URL. Danzo creates chunks equal to number of connections requested. Once all chunks are downloaded, they are combined into a single file. If the decided number of chunks are smaller than 20 MB, Danzo falls back to a single threaded download for that file. This number was **arbitrarily** chosen based on heuristics.

You can customize the number of connections to use like so:

```bash
danzo "https://example.com/largefile.zip" -c 16
```

> [!WARNING]
> You should be careful of the disk IO as well. Multi-connection download takes disk IO, which can add to overall time before the file is ready.
>
> For example, a 1 GB file takes 54 seconds when using 50 connections vs. 62 seconds when using 64 connections. This is because combining 64 files takes longer than combining 50 files.
>
> Therefore, you need to find a balance where the number of connections maximize your network throughput without putting extra strain on disk IO. This effect is especially observable in HDDs.

Lastly, if a URL does not use byte-range requests (i.e., server doesn't support partial content downloads), Danzo automatically switches to a simple, single-threaded, direct download.

### Batch Download

Danzo can be provided a YAML config to allow simultaneous downloads of several URLs. Each URL in turn will use multi-threaded connection mode by default to maximize throughput. The YAML file requires following format:

```yaml
- op: "./output1.zip"
  link: "https://example.com/file1.zip"
- op: "./output2.zip"
  link: "https://example.com/file2.zip"
# more entries with output path and urls...
```

Then run Danzo as:

```bash
danzo -l config.yaml
```

The number of files being downloaded in parallel can be configured as workers (default: 1) and the number of connections would be applied per worker. Define these parameters as follows:

```bash
danzo -l downloads.yaml -w 3 -c 16
```

> [!NOTE]
> Danzo caps the total number of parallel workers at 64. Specifically `#workers * #connections <= 64`. This is a generous default to prevent overwhelming the system.

### Resumable Downloads & Temporary Files

Single-connection downloads store a `OUTPUTPATH.part` file in the current working directory while multi-connection downloads store partial files named `OUTPUTPATH.part1`, `OUTPUTPATH.part2`, etc. in the `.danzo-temp` directory.

These partial downloads on disk are useful when a download event is interrupted or failed. In that case, the temporary files are used to resume the download.

> [!WARNING]
> A resume operation is triggered automatically when the same output path is encountered. However, the feature will only work correctly if the number of connections are exactly the same. Otherwise, the resulting assembled file may contain faulty bytes.

To clear the temporary (partially downloaded) files, use the command with the `clean` flag:

```bash
danzo --clean -o "./path/for/download.zip"
# or if output was in current directory -
danzo --clean
```

For batch downloads, you may need to run the clean command for each output path individually if they don't share the same parent directory.

> [!NOTE]
> The `clean` command is helpful only when your downloads have failed or were interrupted. Otherwise, Danzo automatically runs a clean for a download event once it is successful.

### Google Drive Downloads

Downloading a file from a Drive URL requires authentication, which Danzo supports in 2 ways:

- `API Key`: The API key is automatically picked up from the `GDRIVE_API_KEY` environment variable.
  - This requires the end-user to create an API key after enabling the drive API [here](https://console.cloud.google.com/apis/dashboard).
  - Users should visit the [GCP credentials console](https://console.cloud.google.com/apis/credentials), and then create an API key.
  - Then, click the key and restrict it to only the Google Drive API. Save this somewhere safe (**this is a secret**).
- `OAuth2.0 Device Code`: This requires an OAuth client credential file passed to Danzo via the `GDRIVE_CREDENTIALS` environment credentials (similar to how `rclone` does it).
  - Users should enable the necessary APIs like shown in the `API Key` section before this.
  - Then, visit the [GCP credentials console](https://console.cloud.google.com/apis/credentials) and create an "OAuth2.0 Client ID".
  - Download and save the credential JSON file in a safe location (**this is a secret**).
  - During authentication, Danzo will produce a URL to authenticate via the device code flow; users should copy that into a browser.
  - In the browser, allow access to the credential (this effectively allows the credntial you downloaded to act on your behalf and read all your GDrive files).
  - Moving forward after allowing the credential and clicking "Continue", a webpage will appear with an error like "*This site can’t be reached*". THIS IS OKAY!
  - The URL bar will have a link of the form *http ://localhost/?state=state-token&code=**4/0.....AOwVQ**&scope=https:// www.googleapis.com/auth/drive.readonly*.
  - The `code=....&`, i.e., the part after the `=` and before the next `&` sign (highlighted in bold in the previous URL) is what you need to copy and paste into the Danzo terminal waiting for input, then press return.
  - Danzo will exchange this for an authentication token and save it to `.danzo-token.json`.
  - If you re-attempt the use of `GDRIVE_CREDENTIALS`, Danzo will reuse the token from current directory if it exists, refresh it if possible, and fallback to reauthentication.

> [!TIP]
> The API Key method only works on files that are either publicly shared or shared with your user. It cannot be used to download private files that you own. So for your own files, use the OAuth device code method.

Danzo can be used in this manner to download Google Drive files:

```bash
GDRIVE_API_KEY=$(cat ~/secrets/gdrive-api.key) \
danzo "https://drive.google.com/file/d/1w.....HK/view?usp=drive_link"
```

OR

```bash
GDRIVE_CREDENTIALS=~/secrets/gdrive-oauth.key \
danzo "https://drive.google.com/file/d/1w.....HK/view?usp=drive_link"
```

> [!WARNING]
> Danzo does not perform multi-connection download for Google Drive files; instead it uses the simple download method. For Google Drive specifically, this does not present a loss in bandwidth; however, remember that Google does throttle repeated downloads after a while.

> [!NOTE]
> Users who have never logged into GCP may be required to create a new GCP Project. This is normal and doesn't cost anything.

## Tips and Notes

- Large files benefit the most from multiple connections, but also add to disk IO. Be mindful of the balance between network and disk IO.
- If a chunk download fails, Danzo will retry individual chunks up to 5 times.
- For downloading through a proxy, use the `--proxy` or `-p` flag with your proxy URL (you needn't provide the HTTP scheme, Danzo matches it to that of the URL)
- Not all servers support multi-connection downloads (range requests), in which case, Danzo auto-switches to simple downloads.
- For servers with rate limiting, reducing the number of connections might help.
- Debug mode (`--debug`) provides detailed information about the download process.
- Temporary files are automatically cleaned up after successful downloads.
- Use `-a randomize` to randomly assign a user agent for every HTTP client.
  - The full list of user agents considered are stored in the [vars.go](https://github.com/Tanq16/danzo/blob/main/utils/vars.go) file.
- The tool automatically activates "high-thread-mode" when using more than 5 connections, which optimizes socket buffer sizes for better performance.
- Maximizing throughput - Danzo supports 2 (automatically handled) modes of HTTP clients:
  - Simple: this client has a default configuration and usually helps with lower number of connections; Danzo uses this mode for upto 5 connections (including the default of 4)
  - Specialized: this client has a custom configuration and significantly benefits high-thread mode; Danzo automatically switches to this mode for high-thread mode (>=6 connections)
  - Example: A large download with the specicalized client and 2 threads will run at ~4 MB/s. The same download with a simple client would run at ~20 MB/s. Next, the same download with the simple client but 30 threads would also run at 20 MB/s. However, the exact same download with the specialized client and 30 threads will run at ~60 MB/s. This example is a simple real-world observation meant to demonstrate the need for striking a balance between number of threads and download size to obtain maximum throughput.
- Danzo is specifically design so that the item being downloaded is only represented simply as an argument. Danzo automatically adjusts methods, nuances, and strategies to ensure URLs of various types are handled in the intended way.
