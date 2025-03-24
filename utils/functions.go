package utils

import (
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	u "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

func GetRandomUserAgent() string {
	return userAgents[time.Now().UnixNano()%int64(len(userAgents))]
}

func DetermineDownloadType(url string) string {
	if strings.HasPrefix(url, "https://drive.google.com") {
		return "gdrive"
	} else if strings.HasPrefix(url, "s3://") {
		return "s3"
		// } else if strings.HasPrefix(url, "https://youtu.be") || strings.HasPrefix(url, "https://www.youtube.com") {
		// 	return "youtube"
	} else if strings.HasPrefix(url, "ftp://") || strings.HasPrefix(url, "ftps://") {
		return "ftp"
	} else if strings.HasPrefix(url, "sftp://") {
		return "sftp"
	}
	return "http"
}

// includes logger
func ReadDownloadList(filePath string) ([]DownloadEntry, error) {
	log := GetLogger("config")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading YAML file: %v", err)
	}
	var entries []DownloadEntry
	err = yaml.Unmarshal(data, &entries)
	if err != nil {
		return nil, fmt.Errorf("error parsing YAML file: %v", err)
	}
	for i, entry := range entries {
		if entry.URL == "" {
			return nil, fmt.Errorf("missing URL for entry %d", i+1)
		}
		if entry.OutputPath == "" {
			return nil, fmt.Errorf("missing output path for entry %d", i+1)
		}
	}
	log.Debug().Int("count", len(entries)).Msg("Entries loaded from YAML")
	return entries, nil
}

func RenewOutputPath(outputPath string) string {
	dir := filepath.Dir(outputPath)
	base := filepath.Base(outputPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	index := 1
	for {
		outputPath = filepath.Join(dir, fmt.Sprintf("%s-(%d)%s", name, index, ext))
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			return outputPath
		}
		index++
	}
}

// includes logger
func CreateHTTPClient(timeout time.Duration, keepAliveTO time.Duration, proxyURL string, highThreadMode bool) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // for connection reuse
		IdleConnTimeout:     keepAliveTO,
		DisableCompression:  true,
		MaxConnsPerHost:     0,
		// These two seem to reduce performance drastically with custom dial context
		// DisableKeepAlives:   false,
		// ForceAttemptHTTP2:   true,
	}
	if highThreadMode {
		transport.DialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
			// Increased socket buffer size for better speed
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					setSocketOptions(fd)
				})
			},
		}).DialContext
	}
	if proxyURL != "" {
		proxyURLParsed, err := u.Parse(proxyURL)
		if err != nil {
			log.Error().Err(err).Str("proxy", proxyURL).Msg("Invalid proxy URL, proceeding without proxy")
		} else {
			transport.Proxy = http.ProxyURL(proxyURLParsed)
			log.Debug().Str("proxy", proxyURL).Msg("Using proxy for connections")
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func GetFileInfo(url string, userAgent string, client *http.Client) (int64, string, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	filename := ""
	filenameRegex := regexp.MustCompile(`[^a-zA-Z0-9_\-\. ]+`)
	if contentDisposition := resp.Header.Get("Content-Disposition"); contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if fn, ok := params["filename"]; ok && fn != "" {
				filename = filenameRegex.ReplaceAllString(fn, "_")
			} else if fn, ok := params["filename*"]; ok && fn != "" {
				if strings.HasPrefix(fn, "UTF-8''") {
					unescaped, _ := u.PathUnescape(strings.TrimPrefix(fn, "UTF-8''"))
					filename = filenameRegex.ReplaceAllString(unescaped, "_")
				}
			}
		}
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		return 0, filename, ErrRangeRequestsNotSupported
	}
	contentLength := resp.Header.Get("Content-Length")
	if contentLength == "" {
		return 0, filename, errors.New("server didn't provide Content-Length header")
	}
	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return 0, filename, err
	}
	if size <= 0 {
		return 0, filename, errors.New("invalid file size reported by server")
	}
	return size, filename, nil
}

func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func Clean(outputPath string) error {
	tempDir := filepath.Join(filepath.Dir(outputPath), ".danzo-temp")
	if err := os.RemoveAll(tempDir); err != nil {
		return err
	}
	return nil
}
