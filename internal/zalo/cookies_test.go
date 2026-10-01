package zalo

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestCookieScopeSurvivesSaveAndRestore(t *testing.T) {
	// Arrange: parent-domain auth and a separate host-only cookie.
	origin, _ := url.Parse("https://id.zalo.me/account/login")
	jar := newPersistentJar()
	jar.SetCookies(origin, []*http.Cookie{{Name: "auth", Value: "synthetic", Domain: ".zalo.me", Path: "/", Secure: true, HttpOnly: true, MaxAge: 3600}, {Name: "host", Value: "only-id", Secure: true}})
	// Act: serialize lossless records and restore using each original origin.
	data, err := json.Marshal(jar.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var records []savedCookie
	if err = json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	restored := newPersistentJar()
	for _, v := range records {
		u, _ := url.Parse(v.Origin)
		restored.SetCookies(u, []*http.Cookie{&v.Cookie})
	}
	wpa, _ := url.Parse("https://wpa.chat.zalo.me/api/login/getLoginInfo")
	id, _ := url.Parse("https://id.zalo.me/account/profile")
	// Assert: auth reaches the login subdomain, host-only scope is not broadened.
	cookies := restored.Cookies(wpa)
	if len(cookies) != 1 || cookies[0].Name != "auth" {
		t.Fatalf("login cookies: %+v", cookies)
	}
	if len(restored.Cookies(id)) != 2 {
		t.Fatal("host-only cookie lost")
	}
	for _, v := range records {
		if v.Cookie.Name == "auth" && (!v.Cookie.HttpOnly || !v.Cookie.Secure || v.Cookie.Expires.Before(time.Now())) {
			t.Fatal("metadata lost")
		}
	}
	// Act/Assert: expired or deleted cookies are absent from the persisted session.
	restored.SetCookies(origin, []*http.Cookie{{Name: "auth", Domain: ".zalo.me", Path: "/", MaxAge: -1}})
	for _, v := range restored.snapshot() {
		if v.Cookie.Name == "auth" {
			t.Fatal("deleted cookie retained")
		}
	}
}
