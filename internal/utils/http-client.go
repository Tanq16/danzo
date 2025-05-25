package utils

import (
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

type HTTPClientConfig struct {
	Timeout        time.Duration
	KATimeout      time.Duration
	ProxyURL       string
	ProxyUsername  string
	ProxyPassword  string
	UserAgent      string
	Headers        map[string]string
	HighThreadMode bool // advanced socket options for high concurrency
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
	SetHeader(key, value string)
}

type DanzoHTTPClient struct {
	client *http.Client
	config HTTPClientConfig
}

func NewDanzoHTTPClient(cfg HTTPClientConfig) *DanzoHTTPClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.KATimeout == 0 {
		cfg.KATimeout = 60 * time.Second
	}
	transport := &http.Transport{
		IdleConnTimeout:     cfg.KATimeout,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		DisableCompression:  true,
		MaxConnsPerHost:     0,
	}
	if cfg.HighThreadMode {
		transport.DialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					setSocketOptions(fd)
				})
			},
		}).DialContext
	}
	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err == nil {
			if cfg.ProxyUsername != "" {
				if cfg.ProxyPassword != "" {
					proxyURL.User = url.UserPassword(cfg.ProxyUsername, cfg.ProxyPassword)
				} else {
					proxyURL.User = url.User(cfg.ProxyUsername)
				}
			}
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &DanzoHTTPClient{
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
		config: cfg,
	}
}

func (d *DanzoHTTPClient) SetHeader(key, value string) {
	d.config.Headers[key] = value
}

func (d *DanzoHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if d.config.UserAgent != "" {
		req.Header.Set("User-Agent", d.config.UserAgent)
	} else {
		req.Header.Set("User-Agent", "Danzo-CLI")
	}
	for k, v := range d.config.Headers {
		req.Header.Set(k, v)
	}
	return d.client.Do(req)
}
