<div align="center">
  <img src=".github/assets/logo.png" alt="Danzo Logo" width="300">

  <a href="https://github.com/tanq16/danzo/actions/workflows/binary.yml"><img alt="Build" src="https://github.com/tanq16/danzo/actions/workflows/binary.yml/badge.svg"></a> <a href="https://github.com/Tanq16/danzo/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/tanq16/danzo"></a><br><br>
  <p>A cross-platform and cross-architecture all-in-one CLI download utility designed for multi-threaded downloads, unique progress tracking, and an intuitive command structure.</p><p>Just like Danzo collected powers through multiple "sources" ;) in Naruto, this tool uses multiple connections to supercharge downloads.</p><br>
  <a href="#quickstart">Quickstart</a> &bull; <a href="#installation">Installation</a> &bull; <a href="#usage">Usage</a> &bull; <a href="#contributing">Contributing</a> &bull; <a href="#acknowledgements">Acknowledgements</a><br>
</div>

---

> [!WARNING]
> Danzo has seen a significant refactor that changed how commands were called in previous versions. This was done to support an easier CLI interface and make it easier to add additional downloaders in the future.

## Quickstart

This section gives a quick peek at the capabilities and the extremely simple command structure. For detailed descriptions, see [Usage](#usage).

The primary downloaders and their supported aliases are as follows:

| Command | Aliases (Shorthands) | Description |
| --- | --- | --- |
| `http` | - | Multi-chunked or linear downloads for general HTTP(S) sources |
| `batch` | - | Multi-threaded, multi-downloader operation given a yaml config |
| `live-stream` | `hls`, `m3u8`, `livestream`, `stream` | Download a live stream format video (playlist.m3u8 files) with multi-threading |
| `youtube` | `yt` | Download YouTube videos using `yt-dlp` |
| `youtube-music` | `ytm`, `yt-music` | Download audio from YouTube with iTunes/Deezer metadata using `yt-dlp` and `ffmpeg` |
| `git-clone` | `gitclone`, `gitc`, `git`, `clone` | Clone a git repository with SSH/token authentication |
| `github-release` | `ghrelease`, `ghr` | Download a platform-correct release asset for a GitHub repo |
| `google-drive` | `gdrive`, `gd`, `drive` | Download file/folder from Google drive with API key or OAuth flow authentication |
| `s3` | - | Multi-threaded download for object, directory, or full AWS S3 bucket |
| `clean` | - | Clear local cache for interrupted/incomplete downloads |

Following are examples to get started with various flags:

- HTTP(S) downloads
  ```bash
  danzo http https://example.com/internet-file.zip -o local.zip # (in lieu of `wget`)
  danzo http https://example.com/file.zip -H "Authorization: Basic dW46cHc=" # (custom headers like in curl)
  danzo http https://example.com/largefile.zip -c 40 # (fast, multi-threaded, multi-chunked with 40 threads)
  ```
- Batch download from config (multiple workers)
  ```bash
  danzo batch downloads.yaml -w 4 -c 16 # (4 workers with 16 threads each; see Usage for YAML syntax)
  ```
- YouTube video download (uses `yt-dlp`, `ffmpeg`, `ffprobe`)
  ```bash
  danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ" # (default is <=1080p, <=60fps quality)
  danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ" --format 1080p # (download in 1080p)
  # allows customization with `best60`, `decent`, `1080p60`, and more (see Usage)
  danzo ytm "https://www.youtube.com/watch?v=dQw4w9WgXcQ" # (download standard `.m4a` audio)
  danzo ytm "https://youtu.be/JJpFTUP6fIo" --apple 1800533191 # (add music metadata from itunes)
  danzo ytm "https://youtu.be/JJpFTUP6fIo" --deezer 3271607031 # (add music metadata from deezer)
  ```
- Download file from Google Drive
  ```bash
  danzo gd "https://drive.google.com/file/d/abc123/view" --api-key your_key # (static Key only for publicly shared files)
  danzo gd "https://drive.google.com/file/d/abc123/view" --creds service-acc-key.json # (OAuth device code flow for private files)
  ```
- Download streamed output from an m3u8-manifest
  ```bash
  danzo hls "https://example.com/manifest.m3u8" -o video.mp4
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
- Clone a git repository
  ```bash
  danzo git "gitlab.com/username/repo" # (supports `github.com/`, `bitbucket.org/`, and `git.com/`)
  danzo git "github.com/username/repo" --depth 1 # (clone with --depth=1)
  danzo git "github.com/tanq16/private" --token $(cat /secrets/ghtoken) # (use a PAT; auto-manages for different providers)
  danzo git "github.com/tanq16/private" --ssh "/secrets/gh-ssh.key" # (use an SSH key to authenticate)
  ```

## Installation

### Release Binary (Recommended)

- Download the appropriate binary for your system from the [latest release](https://github.com/tanq16/danzo/releases/latest)
- Unzip the file, make the binary executable (Linux/macOS) with `chmod +x danzo`, and run as

```bash
danzo http "https://example.com/largefile.zip"
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
```

Using a download directly won't always yield the best result, so to optimize according to file types, use multiple threads (read through the next couple sections to learn more).

Follow these links to quickly jump to the relevant provider:

- [HTTP(S) Downloads](#https-downloads)
- [Google Drive Downloads](#google-drive-downloads)
- [Youtube Downloads](#youtube-downloads)
- [M3U8 Stream Downloads](#m3u8-stream-downloads)
- [AWS S3 Downloads](#aws-s3-downloads)
- [GitHub Release Downloads](#github-release-downloads)
- [Git Repository Cloning](#git-repository-cloning)

### HTTP(S) Downloads

<details>
<summary>Unfold to read</summary>

The output filename will be inferred from the URL and Danzo will use 8 connection threads and 1 worker by default. You can also specify an output filename manually like:

```bash
danzo http https://example.com/largefile.zip -o ./path/to/file.zip
```

> [!NOTE]
> The value for `-c` can be arbitrary. Danzo creates chunks equal to number of connections requested. Once all chunks are downloaded, they are combined into a single file. If the decided number of chunks are too small, Danzo falls back to a single threaded download for that file.

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
http:
  - link: https://example.com/file1.zip
    op: downloads/file1.zip
  - link: https://example.com/file2.zip

youtube:
  - link: https://youtube.com/watch?v=xxx
    op: video.mp4

s3:
  - link: s3://bucket/file.zip
```

```bash
danzo batch downloads.yaml
```

The number of files being downloaded in parallel can be configured as workers (default: 1) and the number of connections would be applied per worker. Define these parameters as follows:

```bash
danzo batch downloads.yaml -w 3 -c 16 # uses 3 works with 16 threads each
```

#### Resumable Downloads & Temporary Files

Single-connection downloads store a `OUTPUTPATH.part` file in the current working directory while multi-connection downloads store partial files named `OUTPUTPATH.part1`, `OUTPUTPATH.part2`, etc. in the `.danzo-temp` directory.

These partial downloads on disk are useful when a download event is interrupted or failed. In that case, the temporary files are used to resume the download.

> [!WARNING]
> A resume operation is triggered automatically when the same output path is encountered. However, the feature will only work correctly if the number of connections are exactly the same. Otherwise, the resulting assembled file may contain faulty bytes.

To clear the temporary (partially downloaded) files, use the command with the `clean` flag:

```bash
danzo clean "./path/for/download.zip"
# or if output was in current directory -
danzo clean
```

> [!TIP]
> Failed chunks are automatically retried up to 5 times before failing the entire file. Additionally, Danzo automatically runs a clean for a download event once it is successful.

</details>

### Google Drive Downloads

<details>
<summary>Unfold to read</summary>

Downloading a file from a Drive URL requires authentication, which Danzo supports in 2 ways:

- `API Key`:
  - This requires the end-user to create an API key after enabling the drive API [here](https://console.cloud.google.com/apis/dashboard).
  - Users should visit the [GCP credentials console](https://console.cloud.google.com/apis/credentials), and then create an API key.
  - Then, click the key and restrict it to only the Google Drive API. Save this somewhere safe (**this is a secret**).
- `OAuth2.0 Device Code`: This requires an OAuth client credential file passed to Danzo (similar to how `rclone` does it).
  - Users should enable the necessary APIs like shown in the `API Key` section before this.
  - Then, visit the [GCP credentials console](https://console.cloud.google.com/apis/credentials) and create an "OAuth2.0 Client ID".
  - Download and save the credential JSON file in a safe location (**this is a secret**).
  - During authentication, Danzo will produce a URL to authenticate via the device code flow; users should copy that into a browser.
  - In the browser, allow access to the credential (this effectively allows the credntial you downloaded to act on your behalf and read all your GDrive files).
  - Moving forward after allowing the credential and clicking "Continue", a webpage will appear with an error like "*This site can't be reached*". THIS IS OKAY!
  - The URL bar will have a link of the form `http ://localhost/?state=state-token&code=4/0.....AOwVQ&scope=https:// www.googleapis.com/auth/drive.readonly`.
  - The `code=....&`, i.e., the part after the `=` and before the next `&` sign (highlighted in bold in the previous URL) is what you need to copy and paste into the Danzo terminal waiting for input, then press return.
  - Danzo will exchange this for an authentication token and save it to `.danzo-token.json`.
  - If you re-attempt the use of these credentials, Danzo will reuse the token from current directory if it exists, refresh it if possible, and fallback to reauthentication.

> [!TIP]
> The API Key method only works on files that are either publicly shared or shared with your user. It cannot be used to download private files that you own. So for your own files, use the OAuth device code method.

Danzo can be used in this manner to download Google Drive files:

```bash
danzo gdrive "https://drive.google.com/file/d/1w.....HK/view?usp=drive_link" --api-key $(cat ~/secrets/gdrive-api.key)
```

OR

```bash
danzo gdrive "https://drive.google.com/file/d/1w.....HK/view?usp=drive_link" --creds ~/secrets/gdrive-oauth.key
```

> [!WARNING]
> Danzo does not perform multi-connection download for Google Drive files; instead it uses the simple download method. For Google Drive specifically, this does not present a significant loss in bandwidth. This is done because Google can throttle multiple connections after a while.

> [!NOTE]
> Users who have never logged into GCP may be required to create a new GCP Project. This is normal and doesn't cost anything.

</details>

### YouTube Downloads

<details>
<summary>Unfold to read</summary>

Danzo supports downloading videos and audio from YouTube by using [yt-dlp](https://github.com/yt-dlp/yt-dlp) as a dependency. Some files and merge operations may also require `ffmpeg` and `ffprobe`. If not present, Danzo will make a temporary download of the appropriate `yt-dlp` binary. However, it is recommended to have `yt-dlp`, `ffmpeg`, and `ffprobe` pre-installed.

To download a YouTube video:

```bash
# By default, Danzo will download the <=1080p and <=60fps quality.
danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
```

> [!NOTE]
> In an effort to create a successful and simple integration, Danzo lets `yt-dlp` dictate the file extension for a given output. As such, the `-o` flag will not have an effect on the extension. Audio downloads will always have a `.m4a` download, while a video may have `.mp4`, or `.webm`.

A download type can be appended to the URL to control Danzo's behavior. These defaults were chosen based on heuristics and observed popularity.

```bash
# Download 1080p MP4
danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ" --format "1080p"

# Download 720p MP4
danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ" --format "720p"

# Download decent quality (≤1080p)
danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ" --format "decent"

# Download audio only (m4a)
danzo yt "https://www.youtube.com/watch?v=dQw4w9WgXcQ" --format "audio"
```

> [!NOTE]
> YouTube downloads require `yt-dlp` to be installed on your system. If it's not found, Danzo will automatically download and use a compatible version. Additionally, since the STDOUT and STDERR are directly streamed from `yt-dlp` to `danzo`, YouTube videos are not tracked for progress the way HTTP downloads are. When downloading a single YouTube URL, the output from `yt-dlp` will be streamed to the user's STDOUT. But if the URL is part of a batch file, then the output is hidden and the progress appears stalled until finished.

Danzo also supports downloading music from YouTube and automatically add metadata from the Deezer or the iTunes API, when the appropriate ID is provided. Example:

```bash
danzo ytmusic "https://youtu.be/JJpFTUP6fIo" --apple "1800533191"
danzo ytmusic "https://youtu.be/JJpFTUP6fIo" --deezer "3271607031"
```

</details>

### M3U8 Stream Downloads

<details>
<summary>Unfold to read</summary>

Danzo supports downloading streamed content from M3U8 manifests. This is commonly used for video streaming services, live broadcasts, and VOD content.

Danzo downloads the M3U8 manifest, parses the playlist (supports both master and media playlists), downloads all segments, and merges them into a single file.

> [!NOTE]
> Danzo requires `ffmpeg` to be installed for merging the segments.

```bash
danzo m3u8 "https://example.com/path/to/playlist.m3u8" -o video.mp4

# With default output name (stream_[timestamp].mp4)
danzo m3u8 "https://example.com/video/master.m3u8"
```

</details>

### AWS S3 Downloads

<details>
<summary>Unfold to read</summary>

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

> [!WARNING]
> For successful authentication, Danzo needs to use a profile that is configured for the same region as the S3 bucket.

> [!NOTE]
> For S3 downloads, the `connections` flag determines how many objects will be downloaded in parallel if downloading a folder.

</details>

### GitHub Release Downloads

<details>
<summary>Unfold to read</summary>

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

### Git Repository Cloning

<details>
<summary>Unfold to read</summary>

Danzo can clone repositores sourced by various providers. While this is not particularly an expensive operation to run using just `git clone`, it serves to provide ease of setup when setting up a remote server with a large number of files as downloads and clones.

As such, given a situation where a server needs to be prepared for operation by cloning a set of 8 repositories, 5 different tool assets, and an S3 folder; it would be slow to write a script incorporating several tools to get the environment ready. Danzo would be the perfect fir for such a scenario due to its batch-download capability via a YAML configuration. It is primarily for this purpose that an operation as simple and atomic as `git clone` was replicated in Danzo.

> [!WARNING]
> While Danzo as a tool is focused on conducting very fast downloads, it is important to note that in some cases where a git repository may be more than 1.5-2 GB in size, Danzo may experience easily noticeable slowdowns compared to plain old `git clone`. This is expected and usually, it's recommended to enforce depth (continue reading) when cloning repositories that large.

Danzo supports the use of Personal Access Tokens as well as SSH keys when cloning repositories. The syntax has been simplified to refer to repositories with one of the following:

- `github.com/owner/repo`
- `gitlab.com/owner/repo`
- `bitbucket.org/owner/repo`

To clone a publicly available git repository, use a command like so:

```bash
danzo gitclone "gitlab.com/volian/nala"
```

If there is a need to enforce clone depth (`git clone REPO --depth=1`), use a suffix like so:

```bash
danzo gitclone "gitlab.com/volian/nala" --depth 1
```

To clone a git repository with authentication (PAT or SSH Key), use one of the following:

```bash
# use a personal access token; Danzo will handle username per provider
danzo gitclone "github.com/tanq16/private" --token $(cat /secrets/ghtoken)

# use an SSH key to authenticate
danzo gitclone github.com/tanq16/private --ssh "/secrets/gh-ssh.key"
```

> [!NOTE]
> Repository cloning is another download provider that does not use `-c` or number of connections. Number of workers, `-w`, is still applicable as usual in batch (YAML config) mode.

</details>

## Contributing

Danzo uses issues for everything. Open an issue and I will add an appropriate tag automatically for any of these situations:

- If you spot a bug or bad code
- If you have a contribution to make, also open an issue (so it doesn't overlap with current roadmap)
- If you have questions or about doubts about usage

## Acknowledgements

Danzo uses the following third-party tools as partial dependencies:

- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [ffmpeg](https://github.com/FFmpeg/FFmpeg) (and `ffprobe`)

Danzo draws inspiration from the following projects:

- [ytmdl](https://github.com/deepjyoti30/ytmdl)
- [aria2](https://github.com/aria2/aria2)

Lastly, Danzo uses several Go packages referenced within `go.mod` that allow Danzo to be amazing.
