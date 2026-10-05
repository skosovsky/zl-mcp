package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

type archiveTransport func(*http.Request) (*http.Response, error)

func (f archiveTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type archiveBody struct {
	*bytes.Reader
	closed bool
}

func (b *archiveBody) Close() error { b.closed = true; return nil }
func TestMobileArchiveCookieScopeTransportAndJar(t *testing.T) {
	// Arrange: synthetic host/path cookies, distinct ordinary and validated transports.
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://archive.example.com/allowed/file")
	chat, _ := url.Parse("https://chat.zalo.me/")
	jar.SetCookies(u, []*http.Cookie{{Name: "archive", Value: "synthetic", Path: "/allowed", Secure: true}})
	jar.SetCookies(chat, []*http.Cookie{{Name: "chat", Value: "synthetic", Path: "/", Secure: true}})
	sc := mobileRequestSession(&http.Client{Transport: archiveTransport(func(*http.Request) (*http.Response, error) { t.Fatal("SDK transport fallback"); return nil, nil })}, chat.String())
	sc.SetCookieJar(jar)
	body := &archiveBody{Reader: bytes.NewReader([]byte("ciphertext"))}
	calls := 0
	client := &http.Client{Transport: archiveTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Cookie") != "archive=synthetic" {
			t.Error("cookie scope changed")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"archive=changed; Path=/allowed; Secure"}}, Body: body, Request: r}, nil
	})}
	req, _ := http.NewRequest("GET", u.String(), nil)
	// Act.
	data, e := (&api{sc: sc}).ConsumeMobileArchive(context.Background(), req, client, func(_ context.Context, r *http.Response) ([]byte, error) { return io.ReadAll(r.Body) })
	// Assert: only matching cookies, owned body closed, original client/request/jar unchanged.
	if e != nil || calls != 1 || string(data) != "ciphertext" || !body.closed || client.Jar != nil || req.Header.Get("Cookie") != "" || jar.Cookies(u)[0].Value != "synthetic" {
		t.Fatal("session archive failed", e)
	}
	clear(data)
	client.Transport = archiveTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Error("cookie escaped path scope")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("x")), Request: r}, nil
	})
	req, _ = http.NewRequest("GET", "https://archive.example.com/other", nil)
	if _, e = (&api{sc: sc}).ConsumeMobileArchive(context.Background(), req, client, func(context.Context, *http.Response) ([]byte, error) { return []byte("x"), nil }); e != nil {
		t.Fatal(e)
	}
}
func TestMobileArchiveRedirectCredentialsAndFailedBuffer(t *testing.T) {
	// Arrange.
	sc := mobileRequestSession(&http.Client{}, "https://chat.zalo.me")
	jar, _ := cookiejar.New(nil)
	sc.SetCookieJar(jar)
	calls := 0
	client := &http.Client{Transport: archiveTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.example.com/private"}}, Body: io.NopCloser(strings.NewReader("x")), Request: r}, nil
	})}
	req, _ := http.NewRequest("GET", "https://archive.example.com/file", nil)
	owned := []byte("synthetic private result")
	// Act.
	data, e := (&api{sc: sc}).ConsumeMobileArchive(context.Background(), req, client, func(_ context.Context, r *http.Response) ([]byte, error) {
		if r.StatusCode != 302 {
			t.Error("redirect followed")
		}
		return owned, errors.New("private marker")
	})
	// Assert.
	if !errors.Is(e, ErrMobileArchive) || data != nil || calls != 1 || !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("redirect or failed result escaped")
	}
	for _, header := range []string{"Cookie", "cookie", "Authorization", "authorization", "Proxy-Authorization"} {
		// Arrange / Act / Assert: injected credentials fail before any network call.
		r := req.Clone(context.Background())
		r.Header[header] = []string{"synthetic"}
		before := calls
		if _, e = (&api{sc: sc}).ConsumeMobileArchive(context.Background(), r, client, func(context.Context, *http.Response) ([]byte, error) { return nil, nil }); !errors.Is(e, ErrMobileArchive) || calls != before {
			t.Fatal("injected credentials accepted")
		}
	}
}

func TestMobileArchiveDiscardsCancelledConsumerResult(t *testing.T) {
	// Arrange: cancellation arrives as the bounded body consumer returns success.
	sc := mobileRequestSession(&http.Client{}, "https://chat.zalo.me")
	jar, _ := cookiejar.New(nil)
	sc.SetCookieJar(jar)
	body := &archiveBody{Reader: bytes.NewReader([]byte("x"))}
	client := &http.Client{Transport: archiveTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body, Request: r}, nil
	})}
	request, _ := http.NewRequest("GET", "https://archive.example.com/file", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owned := []byte("synthetic ciphertext")
	// Act.
	data, e := (&api{sc: sc}).ConsumeMobileArchive(ctx, request, client, func(context.Context, *http.Response) ([]byte, error) { cancel(); return owned, nil })
	// Assert: cancellation wins and no successful prefix survives.
	if !errors.Is(e, ErrMobileArchive) || data != nil || !body.closed || !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("late cancelled ciphertext escaped")
	}
}
