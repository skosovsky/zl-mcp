package mobilebackup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/netpolicy"
)

var ErrDownload = errors.New("mobile backup download failed")

type Downloader struct {
	session domain.MobileArchiveTransport
	hosts   map[string]bool
	client  *http.Client
}

func (d *Downloader) Ready() bool { return d != nil && d.client != nil && len(d.hosts) > 0 }

// NewDownloader requires hosts independently authorized by trusted configuration.
// An offer's URL is never used to populate this allowlist.
func NewDownloader(hosts []string) (*Downloader, error) {
	if len(hosts) == 0 || len(hosts) > 100 {
		return nil, ErrDownload
	}
	allowed := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		if !downloadHost(host) {
			return nil, ErrDownload
		}
		allowed[host] = true
	}
	return &Downloader{hosts: allowed, client: netpolicy.NewClient(120 * time.Second)}, nil
}

func downloadHost(host string) bool {
	if len(host) == 0 || len(host) > 253 || host != strings.ToLower(host) || strings.HasSuffix(host, ".") || !strings.Contains(host, ".") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func (d *Downloader) Fetch(ctx context.Context, rawURL string, expected, limit uint64) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || d == nil || d.client == nil || expected == 0 || expected > limit || limit > MaxTotalBytes || len(rawURL) > 4096 {
		return nil, ErrDownload
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || !downloadHost(u.Hostname()) || !d.hosts[u.Hostname()] || u.Port() != "" && u.Port() != "443" {
		return nil, ErrDownload
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ErrDownload
	}
	req.Header.Set("Accept-Encoding", "identity")
	if d.session != nil {
		data, e := d.session.ConsumeMobileArchive(ctx, req, d.client, func(active context.Context, response *http.Response) ([]byte, error) {
			return readDownload(active, response, expected)
		})
		if e != nil || ctx.Err() != nil || uint64(len(data)) != expected {
			clear(data)
			return nil, ErrDownload
		}
		return data, nil
	}
	response, err := d.client.Do(req)
	if err != nil {
		return nil, ErrDownload
	}
	defer response.Body.Close()
	return readDownload(ctx, response, expected)
}

func readDownload(ctx context.Context, response *http.Response, expected uint64) ([]byte, error) {
	if ctx == nil || response == nil || response.Body == nil || ctx.Err() != nil {
		return nil, ErrDownload
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" || response.ContentLength >= 0 && uint64(response.ContentLength) != expected {
		return nil, ErrDownload
	}
	data, err := io.ReadAll(io.LimitReader(cancelReader{ctx, response.Body}, int64(expected)+1))
	if err != nil || ctx.Err() != nil || uint64(len(data)) != expected {
		clear(data)
		return nil, ErrDownload
	}
	return data, nil
}

// WithSession binds a copy; the original remains an anonymous codec test candidate.
func (d *Downloader) WithSession(source domain.MobileArchiveTransport) (*Downloader, error) {
	if !d.Ready() || source == nil {
		return nil, ErrDownload
	}
	clone := *d
	clone.session = source
	return &clone, nil
}
