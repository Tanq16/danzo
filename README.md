<div align="center">
  <img src=".github/assets/logo.svg" alt="Danzo Logo" width="300">
  <h1>Danzo</h1>

  <a href="https://github.com/tanq16/danzo/actions/workflows/release.yaml"><img alt="Release" src="https://github.com/tanq16/danzo/actions/workflows/release.yaml/badge.svg"></a> <a href="https://github.com/Tanq16/danzo/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/tanq16/danzo"></a><br><br>
  <p>A cross-platform and cross-architecture all-in-one CLI download utility designed for multi-threaded downloads, unique progress tracking, and an intuitive command structure.</p><p>Just like Danzo collected powers through multiple "sources" ;) in Naruto, this tool uses multiple connections to supercharge downloads.</p><br>
  <a href="#capabilities">Capabilities</a> &bull; <a href="#installation">Installation</a> &bull; <a href="#usage">Usage</a> &bull; <a href="#tips-and-notes">Tips & Notes</a><br>
</div>

---

## Capabilities

This section gives a quick peek at the capabilities and the extremely simple command structure. For detailed descriptions, see [Usage](#usage).

The primary downloaders and their supported aliases are as follows:


| Command          | Aliases (Shorthands)                  | Description                                                                                                             |
| ---------------- | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `http`           | -                                     | Multi-chunked or linear downloads for general HTTP(S) sources                                                           |
| `live-stream`    | `hls`, `m3u8`, `livestream`, `stream` | Download a live stream format video (playlist.m3u8 files) with multi-threading and extractor support for multiple sites |
| `github-release` | `ghrelease`, `ghr`                    | Download a platform-correct release asset for a GitHub repo                                                             |
| `s3`             | -                                     | Multi-threaded download for object, directory, or full AWS S3 bucket                                                    |
| `ytdlp`          | `yt-dlp`, `youtube-dl`, `ytdl`        | Wraps the `yt-dlp` binary for sites Danzo doesn't natively support (YouTube, etc.)                                      |
| `torrent`        | -                                     | Download file or directory via BitTorrent or Magnet link                                                                |
| `resume`         | -                                     | Resume downloads from saved interrupted job state                                                                       |
| `clean`          | -                                     | Clear local cache for interrupted/incomplete downloads                                                                  |
| `batch`          | -                                     | Download multiple resources of different types in batch from a file or stdin                                            |


Following are examples to get started with various flags:

- HTTP(S) downloads
  ```bash
  danzo http https://example.com/internet-file.zip -o local.zip # (in lieu of `wget`)
  danzo http https://example.com/file.zip -H "Authorization: Basic dW46cHc=" # (custom headers like in curl)
  danzo http https://example.com/largefile.zip -c 40 # (fast, multi-threaded, multi-chunked with 40 threads)
  ```
- Download streamed output from an m3u8-manifest
  ```bash
  danzo hls "https://example.com/manifest.m3u8" -o video.mp4
  danzo hls "https://rumble.com/v893ud-something.html" -e rumble # rumble extractor
  danzo hls "https://www.dailymotion.com/video/a999aas" -e dailymotion # dailymotion extractor
  # extractors (-e) automatically extract m3u8 URLs from service URLs
  ```
- Download an S3 object or folder
  ```bash
  danzo s3 "s3://mybucket/path/to/file.zip" --profile myprofile
  # also supports directories, it will use download multiple objects in parallel
  ```
- Download GitHub release asset
  ```bash
  danzo ghr "username/repo" # (auto-selects release according to OS and arch)
  danzo ghr "username/repo" --manual # (choose asset interactively)
  ```
- Download via `yt-dlp` (for sites Danzo doesn't natively support, like YouTube)
  ```bash
  danzo ytdlp "https://www.youtube.com/watch?v=VizjMEe0agI" -o marigold.mp4
  danzo ytdlp "https://vimeo.com/173855964" # (default yt-dlp output template)
  ```
- Download via BitTorrent or Magnet link
  ```bash
  danzo torrent "magnet:?xt=urn:btih:..." -o ./downloads/
  danzo torrent "./ubuntu.torrent" -o ./iso/
  ```
- Download multiple files in batch (mixed types in parallel)
  ```bash
  danzo batch downloads.txt # (plaintext batch file)
  danzo batch jobs.yaml --workers 4 # (YAML batch file with 4 workers)
  cat urls.txt | danzo batch # (pipe urls directly from stdin)
  ```

## Installation

### Release Binary (Recommended)

- Download the appropriate binary for your system from the [latest release](https://github.com/tanq16/danzo/releases/latest)
- Rename it to `danzo` if desired, make the binary executable (Linux/macOS) with `chmod +x danzo`, and run as

```bash
danzo http "https://example.com/largefile.zip"
```

Interrupted multi-job runs can be resumed from saved state:

```bash
danzo resume
```

### Using Go (Development Version)

With `Go 1.25+` installed, run the following to install the binary to your GOBIN:

```bash
go install github.com/tanq16/danzo@latest
```

Or, you can build from source like so:

```bash
git clone https://github.com/tanq16/danzo.git && cd danzo
go build .
```

## Usage

### Basic Usage

The simplest download is that of a file over HTTP(S). Just provide a URL and let Danzo do its thing:

```bash
danzo http https://example.com/largefile.zip
```

Danzo supports these global options:

```
--proxy, -p          HTTP/HTTPS proxy URL
--proxy-username     Proxy authentication username  
--proxy-password     Proxy authentication password
--user-agent, -a     Custom user agent string
--header, -H         Custom headers (repeatable)
--workers, -w        Number of parallel workers (default: 1)
--connections, -c    Connections per download (default: 8)
--debug              Enable debug logging at info or debug level (default: disabled, i.e., uses TUI)
--for-ai             Enable plain AI-agent-friendly output and piped input
```

Using a download directly won't always yield the best result, so to optimize according to file types, use multiple threads (read through the next couple sections to learn more).

Follow these links to quickly jump to the relevant provider:

- [HTTP(S) Downloads](#https-downloads)
- [M3U8 Stream Downloads](#m3u8-stream-downloads)
- [AWS S3 Downloads](#aws-s3-downloads)
- [GitHub Release Downloads](#github-release-downloads)
- [yt-dlp Downloads](#yt-dlp-downloads)
- [Torrent Downloads](#torrent-downloads)
- [Batch Downloads](#batch-downloads)

### HTTP(S) Downloads

<details><summary>Unfold to read</summary>

The output filename will be inferred from the URL and Danzo will use 8 connection threads and 1 worker by default. You can also specify an output filename manually like:

```bash
danzo http https://example.com/largefile.zip -o ./path/to/file.zip
```

> ✎ The value for `-c` can be arbitrary. Danzo creates chunks equal to number of connections requested. Once all chunks are downloaded, they are combined into a single file. If the decided number of chunks are too small, Danzo falls back to a single threaded download for that file.

You can customize the number of connections to use like so:

```bash
danzo "https://example.com/largefile.zip" -c 16
```

> ⚠ You should be careful of the disk IO as well. Multi-connection download takes disk IO, which can add to overall time before the file is ready.
>
> For example, a 1 GB file takes 54 seconds when using 50 connections vs. 62 seconds when using 64 connections. This is because combining 64 files takes longer than combining 50 files.
>
> Therefore, you need to find a balance where the number of connections maximize your network throughput without putting extra strain on disk IO. This effect is especially observable in HDDs.

Lastly, if a URL does not use byte-range requests (i.e., server doesn't support partial content downloads), Danzo automatically switches to a simple, single-threaded, direct download.

#### Resumable Downloads & Temporary Files

Single-connection downloads store a `OUTPUTPATH.part` file in the current working directory while multi-connection downloads store partial files named `OUTPUTPATH.part1`, `OUTPUTPATH.part2`, etc. in the `.danzo-temp` directory.

These partial downloads on disk are useful when a download event is interrupted or failed. In that case, the temporary files are used to resume the download.

> ⚠ A resume operation is triggered automatically when the same output path is encountered. However, the feature will only work correctly if the number of connections are exactly the same. Otherwise, the resulting assembled file may contain faulty bytes.

To clear the temporary (partially downloaded) files, use the command with the `clean` flag:

```bash
danzo clean "./path/for/download.zip"
# or if output was in current directory -
danzo clean
```

> ✦ Failed chunks are automatically retried up to 5 times before failing the entire file. Additionally, Danzo automatically runs a clean for a download event once it is successful.
</details>



### M3U8 Stream Downloads

<details><summary>Unfold to read</summary>

Danzo supports downloading streamed content from M3U8 manifests. This is commonly used for video streaming services, live broadcasts, and VOD content.

Danzo downloads the M3U8 manifest, parses the playlist (supports both master and media playlists), downloads all segments, and merges them into a single file.

> ✎ Danzo requires `ffmpeg` to be installed for merging the segments.

```bash
danzo m3u8 "https://example.com/path/to/playlist.m3u8" -o video.mp4

# With default output name (stream_[timestamp].mp4)
danzo m3u8 "https://example.com/video/master.m3u8"
```

#### Extractors

Danzo includes site-specific extractors that automatically extract M3U8 URLs from popular video hosting services. Extractors are automatically detected based on the URL, or can be explicitly specified using the `-e` or `--extract` flag. Examples:

```bash
danzo hls "https://rumble.com/v893ud-something.html" -e rumble
danzo hls "https://www.dailymotion.com/video/a999aas" -e dailymotion
danzo hls "https://dai.ly/a999aas" -e dailymotion
```
</details>



### AWS S3 Downloads

<details><summary>Unfold to read</summary>

There are 2 ways of downloading objects from S3:

- Public Buckets: These are often directly exposed as HTTP(S) sites or pre-signed URLs. Either of the two can be sufficiently handled by the HTTP(S) downloaders.
- Private Buckets: This is where keys or locally setup profiles are important. Danzo simplifies the process in which the user is responsible for securing access to S3, so they can use the resource. The profile can be specified to Danzo with an environment variable.

Danzo supports downloading a single object, a single directory, or the entire bucket from AWS S3. All such connections are multi-threaded for maximum efficiency. Example:

```bash
# uses the `astro` profile to authenticate
danzo s3 "s3://mybucket/path/to/file.zip" --profile astro
```

You can also download entire folders from S3 (specifying just the key):

```bash
danzo s3 "mybucket/some/directory/"
```

AWS session profiles are used to allow for flexibility and ease of access. As a result, specifying the flag (`--profile`) allows using a profile of the user's choice. Additionally, when not set, Danzo uses the `default` profile.

> ⚠︎ For successful authentication, Danzo needs to use a profile that is configured for the same region as the S3 bucket.

> ✎ For S3 downloads, the `connections` flag determines how many objects will be downloaded in parallel if downloading a folder.
</details>



### GitHub Release Downloads

<details><summary>Unfold to read</summary>

It is often a task to download GitHub project releases because it requires figuring out the exact name of the asset file based on the OS and architecture of the machine. Danzo simplifies this process and only requires you to provide the owner and the project name. It uses that to automatically identify the correct latest release for its host's architecture and OS.

```bash
# default: latest release for your platform
danzo ghrelease "github.com/owner/repo"
```

Danzo also automatically falls back to a user selection process where the user is displayed the release versions and the assets, requiring the user to confirm each to trigger the correct download.

If the user selection process needs to be manually kicked off, use Danzo like so:

```bash
danzo ghrelease "owner/repo" --manual
```
</details>



### yt-dlp Downloads

<details><summary>Unfold to read</summary>

For sites Danzo doesn't natively support (YouTube, Vimeo with audio, etc.), the `ytdlp` command wraps the `yt-dlp` binary and streams its progress into the Danzo TUI so it looks and behaves like every other Danzo job.

> ✎ Requires `yt-dlp` to be installed and available on `PATH`. Some downloads (e.g., separate video + audio streams) additionally require `ffmpeg` for the final merge step.

```bash
danzo ytdlp "https://www.youtube.com/watch?v=jNQXAC9IVRw" -o me-at-the-zoo.mp4

# Without -o, yt-dlp picks its own filename via its default output template.
danzo ytdlp "https://vimeo.com/22439234"

# Use browser cookies to download authenticated content (e.g., private Google Drive files)
danzo ytdlp "https://drive.google.com/file/d/..." --cookies-from-browser chrome
```

The wrapper:

- Streams `yt-dlp`'s structured `JSON_PROGRESS:` lines into the highway display so percent/byte counters move in real time.
- Switches the bar into a "Merging" sub-status when `yt-dlp` reaches the `[Merger]`/`Merging formats` phase (the bar would otherwise be misleading during the ffmpeg merge).
- Surfaces the failing `ERROR:` line from `yt-dlp`'s stderr in the failure message, so things like a bad URL produce the actual reason rather than a bare `exit status 1`.
- If the chosen output path already exists, falls back to `name-(1).ext` (same behavior as `http` / `git-clone`).

> ✎ This is intentionally a thin wrapper - any flags beyond `--output/-o` should be configured on the `yt-dlp` side (e.g., via its `--config-location`).
</details>

### Torrent Downloads

<details><summary>Unfold to read</summary>

Danzo supports downloading files and folders from the BitTorrent network using either a `.torrent` file path, a magnet link, or a public torrent URL. It utilizes multi-peer downloading and displays detailed real-time statistics in the highway TUI display.

```bash
# Download using a magnet link
danzo torrent "magnet:?xt=urn:btih:..." -o ./downloads/

# Download using a local .torrent file
danzo torrent "./ubuntu-24.04-desktop-amd64.torrent" -o ./iso/
```

> ✎ For Torrent downloads, the `-o` or `--output` flag specifies the target output directory where the files within the torrent will be saved.
</details>

### Batch Downloads

<details><summary>Unfold to read</summary>

Danzo supports batch downloading multiple files from a plaintext file, a YAML/JSON configuration, or directly piped from standard input (`stdin`). This allows running mixed job types (HTTP, HLS stream, GitHub Releases, S3, yt-dlp, Torrents) in parallel.

#### Prefix Mapping
URLs can be prefixed with `prefix::` to explicitly set the download provider:
- `http::` -> HTTP download
- `hls::` / `livestream::` / `live-stream::` / `m3u8::` -> HLS live stream video download
- `ghr::` / `github-release::` / `ghrelease::` -> GitHub release download
- `s3::` -> AWS S3 download
- `ytdlp::` / `yt-dlp::` / `youtube-dl::` -> yt-dlp download
- `torrent::` -> BitTorrent / Magnet link download

If no prefix is present, standard HTTP is used as a fallback (with auto-detection for `s3://`, `magnet:`, `.m3u8`, etc.).

#### Plain-Text Format
Each line represents a job with the format `[PREFIX::]URL [OUTPUT_PATH]`. Whitespace splits the URL and optional output path. Output paths with spaces can be wrapped in double quotes.

Example plaintext file `downloads.txt`:
```text
# Mixed batch download list
ytdlp::https://www.youtube.com/watch?v=VizjMEe0agI "youtube video.mp4"
https://example.com/file.zip ./downloads/my-file.zip
s3::s3://mybucket/dataset/
```

Execute via:
```bash
danzo batch downloads.txt --workers 3
```

#### YAML Configuration
Allows setting specific connection counts or custom options per job:
```yaml
- url: "ytdlp::https://www.youtube.com/watch?v=VizjMEe0agI"
  output: "marigold.mp4"
- url: "https://example.com/largefile.zip"
  output: "archive.zip"
  connections: 32
- url: "s3::s3://mybucket/dataset/"
  profile: "prod-profile"
```

Execute via:
```bash
danzo batch jobs.yaml --workers 4
```

#### Stdin Piping
Piping links from other commands directly:
```bash
cat list.txt | danzo batch --workers 2
```
</details>



## Tips and Notes

- Use `--for-ai` when invoking Danzo from scripts or AI agents that need stable plain-text output.
- Use `--debug` when you need structured logs with underlying error details.
- Use `danzo clean` to clear temporary partial download files and saved resume state.

## Contributing

Danzo uses issues for everything. Open an issue and I will add an appropriate tag automatically for any of these situations:

- If you spot a bug or bad code
- If you have a contribution to make, also open an issue (so it doesn't overlap with current roadmap)
- If you have questions or about doubts about usage

## Acknowledgements

Danzo uses `ffmpeg` for merging M3U8 stream segments.

Danzo wraps `yt-dlp` for sites it doesn't natively support.

Danzo draws inspiration from [aria2](https://github.com/aria2/aria2).

Lastly, Danzo uses several Go packages referenced within `go.mod` that allow Danzo to be amazing.