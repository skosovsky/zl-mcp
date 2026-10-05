package api

import (
	"context"
	"errors"
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
	response, err := client.Do(request)
	if err != nil {
		return nil, ErrMobileArchive
	}
	defer response.Body.Close()
	data, err := consume(ctx, response)
	if err != nil || ctx.Err() != nil || a.sc.UID() != owner {
		clear(data)
		return nil, ErrMobileArchive
	}
	return data, nil
}

// Download responses cannot change authentication cookies.
type archiveReadOnlyJar struct{ http.CookieJar }

func (archiveReadOnlyJar) SetCookies(*url.URL, []*http.Cookie) {}
