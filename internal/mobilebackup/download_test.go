package mobilebackup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

type downloadTransport func(*http.Request) (*http.Response, error)

func (f downloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedDownloadBody struct {
	data   string
	read   int
	closed bool
}

func (r *trackedDownloadBody) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	r.read += n
	return n, nil
}
func (r *trackedDownloadBody) Close() error { r.closed = true; return nil }

func TestDownloadExactLengthAndFailureBounds(t *testing.T) {
	for _, tc := range []struct {
		name, body, encoding string
		status               int
		length               int64
		success              bool
	}{
		{"exact", "ciphertext", "", 200, 10, true},
		{"chunked_exact", "ciphertext", "identity", 200, -1, true},
		{"short", "short", "", 200, -1, false},
		{"overflow", strings.Repeat("x", 100), "", 200, -1, false},
		{"length_mismatch", "ciphertext", "", 200, 11, false},
		{"gzip", "ciphertext", "gzip", 200, 10, false},
		{"redirect", "ciphertext", "", 302, 10, false},
		{"error_status", "ciphertext", "", 500, 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: synthetic transport supplies ciphertext, never a real archive.
			d, err := NewDownloader([]string{"archive.example.com"})
			if err != nil {
				t.Fatal(err)
			}
			body := &trackedDownloadBody{data: tc.body}
			calls := 0
			d.client.Transport = downloadTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.Header.Get("Accept-Encoding") != "identity" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Fatal("unexpected credentials/encoding")
				}
				return &http.Response{StatusCode: tc.status, ContentLength: tc.length, Header: http.Header{"Content-Encoding": []string{tc.encoding}}, Body: body}, nil
			})
			// Act.
			data, err := d.Fetch(context.Background(), "https://archive.example.com/backup?signature=synthetic-private", 10, 20)
			// Assert: bounds, close and fixed errors apply even to a valid prefix.
			if calls != 1 || !body.closed || body.read > 11 {
				t.Fatal("unbounded or repeated read")
			}
			if tc.success {
				if err != nil || string(data) != "ciphertext" {
					t.Fatal("valid ciphertext rejected")
				}
			} else if !errors.Is(err, ErrDownload) || data != nil || strings.Contains(err.Error(), "signature") {
				t.Fatal("invalid ciphertext exposed")
			}
		})
	}
}

func TestDownloadPolicyRejectsBeforeNetwork(t *testing.T) {
	// Arrange: a trusted exact DNS host, with a transport that must never run.
	d, err := NewDownloader([]string{"archive.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	d.client.Transport = downloadTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected transport") })
	// Act / Assert.
	for _, raw := range []string{"http://archive.example.com/file", "https://other.example.com/file", "https://archive.example.com.evil.invalid/file", "https://archive.example.com:444/file", "https://user:password@archive.example.com/file", "https://archive.example.com/file#fragment", "https://archive.example.com./file", "https://127.0.0.1/file"} {
		if data, err := d.Fetch(context.Background(), raw, 10, 20); !errors.Is(err, ErrDownload) || data != nil {
			t.Fatal("URL policy bypass")
		}
	}
	for _, limits := range [][2]uint64{{0, 10}, {11, 10}, {1, MaxTotalBytes + 1}} {
		if _, err := d.Fetch(context.Background(), "https://archive.example.com/file", limits[0], limits[1]); !errors.Is(err, ErrDownload) {
			t.Fatal("budget bypass")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Fetch(ctx, "https://archive.example.com/file", 10, 20); !errors.Is(err, ErrDownload) {
		t.Fatal("cancel bypass")
	}
	if calls != 0 {
		t.Fatal("rejected request reached network")
	}
	for _, host := range []string{"", "*.example.com", "127.0.0.1", "localhost", "Example.com", "example.com.", "https://example.com", "a..example.com", "-a.example.com"} {
		if _, err := NewDownloader([]string{host}); !errors.Is(err, ErrDownload) {
			t.Fatal("invalid allowlist")
		}
	}
	if _, err := NewDownloader(nil); !errors.Is(err, ErrDownload) {
		t.Fatal("empty allowlist")
	}
}

func TestDownloadProductionTransportPolicy(t *testing.T) {
	// Arrange / Act.
	d, err := NewDownloader([]string{"archive.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := d.client.Transport.(*http.Transport)
	// Assert: no proxy/cookie state and redirects are rejected without a request.
	if !ok || transport.Proxy != nil || d.client.Jar != nil || !transport.DisableKeepAlives || transport.DialContext == nil || d.client.Timeout.Seconds() != 120 || d.client.CheckRedirect(&http.Request{}, nil) == nil {
		t.Fatal("unsafe production client")
	}
}

func TestDownloadFailureDiagnosticsExcludeURLAndBody(t *testing.T) {
	// Arrange: a rejected response with private payload and URL markers.
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	d, _ := NewDownloader([]string{"archive.example.com"})
	d.client.Transport = downloadTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: make(http.Header), ContentLength: 12, Body: io.NopCloser(strings.NewReader("private-body"))}, nil
	})
	// Act.
	_, err := d.Fetch(context.Background(), "https://archive.example.com/private-path?signature=private-token", 12, 20)
	// Assert: diagnosis exposes only fixed stages and numeric status.
	if err == nil || !strings.Contains(logs.String(), "HTTP_STATUS") || !strings.Contains(logs.String(), "403") {
		t.Fatal("missing rejection diagnostic")
	}
	for _, secret := range []string{"private-body", "private-path", "private-token", "archive.example.com"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("private download data leaked")
		}
	}
}
