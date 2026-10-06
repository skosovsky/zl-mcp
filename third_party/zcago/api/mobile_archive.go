package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrMobileArchive = errors.New("mobile archive request failed")

// ConsumeMobileArchive preserves the caller's validated transport and uses only
// URL-applicable cookies from the current account. There is no API transport
// fallback, redirect, retry or credential getter.
func (a *api) ConsumeMobileArchive(ctx context.Context, req *http.Request, validated *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || a.sc == nil || a.sc.UID() == "" || a.sc.CookieJar() == nil || req == nil || req.URL == nil || req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Fragment != "" || req.Body != nil || req.Host != "" && req.Host != req.URL.Host || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" || req.Header.Get("Proxy-Authorization") != "" || validated == nil || validated.Transport == nil || validated.Jar != nil || consume == nil {
		return nil, ErrMobileArchive
	}
	for name := range req.Header {
		if strings.EqualFold(name, "Cookie") || strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Proxy-Authorization") {
			return nil, ErrMobileArchive
		}
	}
	owner := a.sc.UID()
	client := *validated
	client.Jar = archiveReadOnlyJar{a.sc.CookieJar()}
	if client.Timeout <= 0 || client.Timeout > 120*time.Second {
		client.Timeout = 120 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := req.Clone(ctx)
	if err := bindMobileArchiveCookie(request, a.sc.CookieJar()); err != nil {
		slog.Warn("mobile_archive_transport_failed", "stage", "AUTH_COOKIE_SCOPE")
		return nil, ErrMobileArchive
	}
	response, err := client.Do(request)
	if err != nil {
		slog.Warn("mobile_archive_transport_failed", "stage", "HTTP_REQUEST", "cancelled", ctx.Err() != nil)
		return nil, ErrMobileArchive
	}
	defer response.Body.Close()
	data, err := consume(ctx, response)
	if err != nil || ctx.Err() != nil || a.sc.UID() != owner {
		slog.Warn("mobile_archive_transport_failed", "stage", "RESPONSE_CONSUMPTION", "consumer_failed", err != nil, "cancelled", ctx.Err() != nil, "owner_changed", a.sc.UID() != owner)
		clear(data)
		return nil, ErrMobileArchive
	}
	return data, nil
}

// Download responses cannot change authentication cookies.
type archiveReadOnlyJar struct{ http.CookieJar }

func (archiveReadOnlyJar) SetCookies(*url.URL, []*http.Cookie) {}

// Native setAppCookie scopes the same token to zaloapp.com. Borrow only for the
// independently pinned archive host; never broaden the persistent cookie jar.
func bindMobileArchiveCookie(request *http.Request, jar http.CookieJar) error {
	if request.URL.Scheme != "https" || request.URL.Host != "trans-bin.zaloapp.com" || request.URL.User != nil || request.URL.Fragment != "" {
		return nil
	}
	for _, c := range jar.Cookies(request.URL) {
		if c.Name == "zpw_sek" && c.Value != "" {
			return nil
		}
	}
	source, _ := url.Parse("https://chat.zalo.me/")
	var token *http.Cookie
	for _, c := range jar.Cookies(source) {
		if c.Name != "zpw_sek" || c.Value == "" {
			continue
		}
		if token != nil {
			return ErrMobileArchive
		}
		token = c
	}
	if token == nil {
		return ErrMobileArchive
	}
	request.AddCookie(&http.Cookie{Name: "zpw_sek", Value: token.Value})
	return nil
}
