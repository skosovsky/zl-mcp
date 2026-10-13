package zalo

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// savedCookie retains Set-Cookie scope; CookieJar.Cookies discards this metadata.
type savedCookie struct {
	Origin string      `json:"origin"`
	Cookie http.Cookie `json:"cookie"`
}
type persistentJar struct {
	http.CookieJar

	mu    sync.Mutex
	saved map[string]savedCookie
}

func newPersistentJar() *persistentJar {
	jar, _ := cookiejar.New(nil)
	//nolint:exhaustruct_v5 // persistentJar initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
	return &persistentJar{CookieJar: jar, saved: map[string]savedCookie{}}
}
func (j *persistentJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.CookieJar.SetCookies(u, cookies)
	host := strings.ToLower(u.Hostname())
	if host != "zalo.me" && !strings.HasSuffix(host, ".zalo.me") {
		return
	}
	for _, c := range cookies {
		//nolint:gosec // G124: persist the upstream cookie attributes unchanged; this is not issuing a server cookie.
		saved := *c
		if saved.Path == "" || saved.Path[0] != '/' {
			path := u.Path
			cut := strings.LastIndex(path, "/")
			saved.Path = "/"
			if cut > 0 {
				saved.Path = path[:cut]
			}
		}
		domain := strings.TrimPrefix(strings.ToLower(saved.Domain), ".")
		if domain == "" {
			domain = host
		}
		if host != domain && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		key := domain + "\n" + saved.Path + "\n" + saved.Name
		if saved.MaxAge < 0 || (!saved.Expires.IsZero() && saved.Expires.Before(time.Now())) {
			delete(j.saved, key)
			continue
		}
		if saved.MaxAge > 0 {
			saved.Expires = time.Now().Add(time.Duration(saved.MaxAge) * time.Second)
		}
		saved.MaxAge = 0
		j.saved[key] = savedCookie{Origin: (&url.URL{Scheme: u.Scheme, Host: u.Host}).String(), Cookie: saved}
	}
}
func (j *persistentJar) snapshot() []savedCookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := make([]string, 0, len(j.saved))
	for key := range j.saved {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []savedCookie{}
	for _, key := range keys {
		v := j.saved[key]
		if !v.Cookie.Expires.IsZero() && v.Cookie.Expires.Before(time.Now()) {
			continue
		}
		u, _ := url.Parse(v.Origin)
		u.Path = v.Cookie.Path
		// Export only cookies actually accepted by the jar and still present.
		for _, live := range j.CookieJar.Cookies(u) {
			if live.Name == v.Cookie.Name && live.Value == v.Cookie.Value {
				out = append(out, v)
				break
			}
		}
	}
	return out
}
