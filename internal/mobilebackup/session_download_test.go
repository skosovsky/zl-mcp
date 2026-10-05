package mobilebackup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type sessionArchiveFunc func(context.Context, *http.Request, *http.Client, func(context.Context, *http.Response) ([]byte, error)) ([]byte, error)

func (f sessionArchiveFunc) ConsumeMobileArchive(c context.Context, r *http.Request, h *http.Client, k func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	return f(c, r, h, k)
}
func TestSessionDownloaderPreservesValidationAndBodyBounds(t *testing.T) {
	for _, mode := range []string{"exact", "overflow", "encoding", "unauthorized", "host"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: authenticated port uses the exact validated transport supplied by the downloader.
			d, _ := NewDownloader([]string{"archive.example.com"})
			calls := 0
			d.client.Transport = downloadTransport(func(r *http.Request) (*http.Response, error) {
				payload, status, encoding := "ciphertext", 200, ""
				switch mode {
				case "overflow":
					payload += "x"
				case "encoding":
					encoding = "gzip"
				case "unauthorized":
					status = 401
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Encoding": []string{encoding}}, Body: io.NopCloser(strings.NewReader(payload)), ContentLength: -1, Request: r}, nil
			})
			source := sessionArchiveFunc(func(ctx context.Context, req *http.Request, client *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
				calls++
				if client != d.client {
					t.Fatal("validated client replaced")
				}
				response, e := client.Do(req)
				if e != nil {
					return nil, e
				}
				defer response.Body.Close()
				return consume(ctx, response)
			})
			bound, e := d.WithSession(source)
			if e != nil {
				t.Fatal(e)
			}
			url := "https://archive.example.com/file"
			if mode == "host" {
				url = "https://other.example.com/file"
			}
			// Act.
			data, e := bound.Fetch(context.Background(), url, 10, 100)
			// Assert: anonymous original is unchanged; auth response is a fixed archive failure.
			if d.session != nil {
				t.Fatal("original downloader mutated")
			}
			if mode == "exact" {
				if e != nil || string(data) != "ciphertext" || calls != 1 {
					t.Fatal("session fetch failed", e)
				}
			} else if !errors.Is(e, ErrDownload) || data != nil {
				t.Fatal("invalid body accepted")
			}
			if mode == "host" && calls != 0 {
				t.Fatal("unverified URL reached session")
			}
		})
	}
}
