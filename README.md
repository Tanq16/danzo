<div align="center">
  <img src=".github/assets/logo.png" alt="Danzo Logo" width="300">

  <a href="https://github.com/tanq16/danzo/actions/workflows/binary.yml"><img alt="Build" src="https://github.com/tanq16/danzo/actions/workflows/binary.yml/badge.svg"></a> <a href="https://github.com/Tanq16/danzo/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/tanq16/danzo"></a><br><br>
  <p>Danzo is a cross-platform and cross-architecture all-in-one CLI file download utility designed for multi-threaded downloads, progress tracking, and an intuitive command structure.</p><br>
  <a href="#features">Features</a> &bull; <a href="#quickstart">Quickstart</a> &bull; <a href="#installation">Installation</a> &bull; <a href="#usage">Usage</a> &bull; <a href="#tips-and-notes">Tips & Notes</a><br>
</div>

---

*Just like its namesake from the Naruto series who collected powers through multiple "sources", Danzo harnesses the strength of parallel connections to supercharge your downloads.*

Jump to [Quickstart](#quickstart) to look at the extremely simple command structure. For detailed descriptions, see [Usage](#usage). It may help to use GitHub's table of contents (hamburger menu, top right of readme) to navigate to a section of interest.

## Features

***High-Performance Downloads***

- **Multi-threaded**: Split files into chunks for parallel downloads
- **Optimization**: Auto-adjust client for ranged or full downloads based on server support
- **Adaptation**: Use single-threaded downloads for small files or servers without range support
- **High-thread mode**: TCP socket optimizations when using multiple threads
- **Resumable downloads**: Continue interrupted HTTP downloads from where they left off
- **Live progress tracking**: Monitor speed, completion percentage, and ETA in real-time
- **Comprehensive summary**: Download report with total size and average speed on completion

***Batch Processing***

- **YAML configuration**: Download multiple files simultaneously with a simple config
- **Parallel processing**: Set concurrent downloads and connections per download
- **Resource management**: Auto-caps at 64 total connections to prevent overload

***Service & Protocol Support***

- HTTP(S) downloads with range request (for chunking) support
- Google Drive file downloads with API key or OAuth2.0 authentication
- YouTube videos and audio download with simple quality selection (uses [yt-dlp](https://github.com/yt-dlp/yt-dlp))
- AWS S3 objects and folder downloads using AWS profile
- GitHub release asset downloads with automatic OS/architecture detection

***Customization***

- **Output handling**: 
  - Auto-detect filenames from URLs and headers
  - Numbered versioning for duplicate manually specified filenames
- **Connection tuning**: Configure timeouts, HTTP(S) proxy, and user agents
- **Tempfile management**: Auto and on-demand cleanup of temporary files

***Usability***

- **Simplicity**: Extremely simple command structure for ease of use
- **Alternative to wget**: Simple, faster alternative for straightforward downloads
- **One stop shop**: An entryway into downloading files or various kinds
- **Cross-platform compatibility**: Works on Linux, macOS, and Windows as self-contained binaries

## Quickstart

```bash
danzo https://example.com/internet-file.zip -o local.zip

# Multiple connections w/ large files for faster downloads
danzo https://example.com/largefile.zip -c 40

# Batch download with multiple workers and connections per worker
danzo -l downloads.yaml -w 4 -c 16

# Download YouTube video (best quality) using yt-dlp
danzo https://www.youtube.com/watch?v=dQw4w9WgXcQ

# Download YouTube audio only using yt-dlp
danzo "https://www.youtube.com/watch?v=dQw4w9WgXcQ||audio"

# Download from Google Drive (direct for public or OAuth for private)
GDRIVE_API_KEY="your_key" danzo "https://drive.google.com/file/d/abc123/view"
GDRIVE_CREDENTIALS=service-acc-key.json danzo "https://drive.google.com/file/d/abc123/view"

# Download from AWS S3 (file or folder)
AWS_PROFILE=myprofile danzo "s3://mybucket/path/to/file.zip"

# Download a GitHub release (can auto-select based on OS/arch)
danzo "github://username/repo"
```

## Installation

### Release Binary (Recommended)

- Download the appropriate binary for your system from the [latest release](https://github.com/tanq16/danzo/releases/latest)
- Unzip the file, make the binary executable (Linux/macOS) with `chmod +x danzo`, and run as

```bash
danzo "https://example.com/largefile.zip"
```

### Using Go (Development Version)

With `Go 1.24+` installed, run the following to install the binary to your GOBIN:

```bash
go install github.com/tanq16/danzo@latest
```

Or, you can build from source like so:

```bash
git clone https://github.com/tanq16/danzo.git && cd danzo
go build .
```

### CLI Options

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

> [!TIP]
> The help section will aid in building a command, but not explain the nuances. It's highly recommended to read through this document to understand edge cases and nuanced details to bring the most out of downloads with Danzo.

## Usage

### Basic Usage

The simplest way to download a file is to provide a URL directly and let Danzo do its thing:

```bash
danzo https://example.com/largefile.zip
```

Of course, this will not always yield the best result, so to optimize according to file types, read through the specific sections below.

### HTTP(S) Downloads

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

#### Batch Download Capability

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

#### Resumable Downloads & Temporary Files

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
  - Moving forward after allowing the credential and clicking "Continue", a webpage will appear with an error like "*This site can't be reached*". THIS IS OKAY!
  - The URL bar will have a link of the form `http ://localhost/?state=state-token&code=4/0.....AOwVQ&scope=https:// www.googleapis.com/auth/drive.readonly`.
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

### YouTube Downloads

Danzo supports downloading videos and audio from YouTube by using [yt-dlp](https://github.com/yt-dlp/yt-dlp) as a dependency. By default, it will download the best available quality.

To download a YouTube video:

```bash
danzo "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
```

> [!NOTE]
> In an effort to create a successful and simple integration, Danzo uses a default output name of `danzo-yt-dlp-video.mp4` or `danzo-yt-dlp-audio.m4a`. As such, the `-o` flag will have no effect on a YouTube download.

A download type can be appended to the URL to control Danzo's behavior. These defaults were chosen based on heuristics and observed popularity.

```bash
# Download best quality
danzo "https://www.youtube.com/watch?v=dQw4w9WgXcQ||best"

# Download 1080p MP4
danzo "https://www.youtube.com/watch?v=dQw4w9WgXcQ||1080p"

# Download 720p MP4
danzo "https://www.youtube.com/watch?v=dQw4w9WgXcQ||720p"

# Download decent quality (≤1080p)
danzo "https://www.youtube.com/watch?v=dQw4w9WgXcQ||decent"

# Download audio only (m4a)
danzo "https://www.youtube.com/watch?v=dQw4w9WgXcQ||audio"
```

> [!NOTE]
> YouTube downloads require `yt-dlp` to be installed on your system. If it's not found, Danzo will automatically download and use a compatible version. Additionally, since the STDOUT and STDERR are directly streamed from `yt-dlp` to `danzo`, YouTube videos are not tracked for progress the way HTTP downloads are. When downloading a single YouTube URL, the output from `yt-dlp` will be streamed to the user's STDOUT. But if the URL is part of a batch file, then the output is hidden and the progress appears stalled until finished.

### AWS S3 Downloads

There are 2 ways of downloading objects from S3:

- Public Buckets: These are often directly exposed as HTTP(S) sites or pre-signed URLs. Either of the two can be sufficiently handled by the HTTP(S) downloaders.
- Private Buckets: This is where keys or locally setup profiles are important. Danzo simplifies the process in which the user is responsible for securing access to S3, so they can use the resource. The profile can be specified to Danzo with an environment variable.

Danzo supports downloading a single object, a single directory, or the entire bucket from AWS S3. All such connections are multi-threaded for maximum efficiency. Example:

```bash
# uses the `astro` profile to authenticate
AWS_PROFILE=astro danzo "s3://mybucket/path/to/file.zip"
```

You can also download entire folders from S3:

```bash
danzo "s3://mybucket/some/directory/"
```

AWS session profiles are used to allow for flexibility and ease of access. As a result, specifying the environment variable (`AWS_PROFILE`) allows using a profile of the user's choice. Additionally, when not set inline or exported, Danzo uses the `default` profile.

```bash
AWS_PROFILE=myprofile danzo "s3://mybucket/path/to/file.zip"
```

> [!WARNING]
> For successful authentication, Danzo should use a profile that is configured for the same region as the S3 bucket.

> [!NOTE]
> For S3 downloads, the `connections` flag determines how many objects will be downloaded in parallel if downloading a folder.

### GitHub Release Downloads

It is often a task to download GitHub project releases because it requires figuring out the exact name of the asset file based on the OS and architecture of the machine. Danzo simplifies this process and only requires you to provide the owner and the project name. It uses that to automatically identify the correct latest release for its host's architecture and OS.

```bash
# default: latest release for your platform
danzo "github://owner/repo"
```

Danzo also automatically falls back to a user selection process where the user is displayed the release versions and the assets, requiring the user to confirm each to trigger the correct download.

If the user selection process needs to be manually kicked off, use Danzo like so:

```bash
danzo "github://owner/repo||version"
```

> [!WARNING]
> If you provide a `||version` subcommand or if the automatic download triggers a selection process when using a YAML config for batch downloads, the continuous progress display may interfere with the selection display. This may become especially harder if multiple `github://` URLs are used. Therefore, it is recommended to use the GitHub release download feature only for one-off downloads.

## Tips and Notes

- For troubleshooting, use `--debug` to see detailed operation logs
- Use `-a randomize` to assign a random user agent for each HTTP client
- When using a proxy, you only need to provide the hostname and port with `-p` - the scheme is matched to the download URL
- Failed chunk downloads are automatically retried up to 5 times before failing
- Reduce connection count for servers with rate limiting to avoid being blocked
- Balance connections and file size for optimal performance - more connections aren't always better due to disk I/O overhead
- Throughput optimization tips:
  - For small files (<20MB), single-threaded downloads are often faster
  - For large files on high-bandwidth connections, try 12-16 connections
  - For very large files (>1GB), find your optimal connection count (usually 16-32)
  - Example: A 1GB file took 54 seconds with 50 connections vs 62 seconds with 64 connections due to assembly overhead
- YouTube downloads require `yt-dlp`, which Danzo will automatically download if not found in your PATH
- AWS S3 downloads uses configured AWS CLI profiles - if a specific profile is not set with `AWS_PROFILE=name`, the `default` profile is used
- For Google Drive downloads, rate limits and throttling will be enforced by Google; Danzo only uses a simple client
- If a download is interrupted, Danzo will automatically resume from temporary files when you run the same command (requires matching connections count)
- To change Google Drive auth methods, use environment variables:
  - API Key: `GDRIVE_API_KEY=your_key` (can only download shared URLs)
  - OAuth: `GDRIVE_CREDENTIALS=path/to/credentials.json` (can download private URLs also)
