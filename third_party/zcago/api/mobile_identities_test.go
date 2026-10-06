package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"github.com/amrakk/zcago/session"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/internal/cryptox"
)

func identityResponse(t *testing.T, plain string) string {
	t.Helper()
	encrypted, err := cryptox.EncodeAESCBC([]byte(strings.Repeat("k", 32)), plain, cryptox.EncryptTypeBase64)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"error_code": 0, "data": encrypted})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func TestMobileIdentitiesEncryptedWire(t *testing.T) {
	// Arrange: current synthetic session, no new listener/login.
	calls := 0
	reply := identityResponse(t, `{"error_code":0,"data":{"fids":["9007199254740995"],"gids":["g9007199254740997"]}}`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/znoise" || r.URL.Query().Get("nretry") != "0" || r.URL.Query().Get("zpw_ver") != "691" || r.URL.Query().Get("zpw_type") != "30" {
			t.Error("identity route mismatch")
		}
		if r.ParseForm() != nil {
			t.Error("invalid form")
			return
		}
		plain, err := cryptox.DecodeAESCBC([]byte(strings.Repeat("k", 32)), r.PostForm.Get("params"))
		if err != nil {
			t.Error("request not encrypted")
			return
		}
		defer clear(plain)
		if string(plain) != `{"fids":[9007199254740993],"gids":[18446744073709551615]}` {
			t.Error("integer precision lost")
		}
		w.Write([]byte(reply))
	}))
	defer server.Close()
	sc := mobileRequestSession(server.Client(), server.URL)
	fn, err := mobileIdentitiesFactory(server.URL+"/api/znoise")(sc, &api{sc: sc})
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	data, err := fn(context.Background(), []string{"9007199254740993"}, []string{"18446744073709551615"})
	// Assert.
	if err != nil || calls != 1 || string(data) != `{"fids":["9007199254740995"],"gids":["g9007199254740997"]}` {
		t.Fatal("mapping failed", err)
	}
	clear(data)
}
func TestMobileIdentitiesRejectsResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing_code", `{"data":"private-marker"}`, 200},
		{"missing_data", `{"error_code":0}`, 200},
		{"duplicate_code", `{"error_code":114,"error_code":0,"data":"private-marker"}`, 200},
		{"duplicate_inner_data", identityResponse(t, `{"error_code":0,"data":{},"data":{}}`), 200},
		{"inner_missing_code", identityResponse(t, `{"data":{}}`), 200},
		{"inner_missing_data", identityResponse(t, `{"error_code":0}`), 200},
		{"inner_error", identityResponse(t, `{"error_code":114,"error_message":"private-marker"}`), 200},
		{"decrypted_limit", identityResponse(t, `{"error_code":0,"data":"`+strings.Repeat("x", 256<<10)+`"}`), 200},
		{"wire_limit", `{"error_code":0}` + strings.Repeat(" ", 512<<10), 200},
		{"auth", "private-marker", 401}, {"redirect", "private-marker", 302}, {"server_error", "private-marker", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			sc := mobileRequestSession(server.Client(), server.URL)
			fn, err := mobileIdentitiesFactory(server.URL+"/api/znoise")(sc, &api{sc: sc})
			if err != nil {
				t.Fatal(err)
			}
			// Act.
			data, err := fn(context.Background(), []string{"1"}, nil)
			// Assert.
			if err == nil || data != nil || calls != 1 || strings.Contains(err.Error(), "private-marker") {
				t.Fatal("failure leaked or retried")
			}
			if tc.status == 401 && !errors.Is(err, errs.ErrAuthenticationRequired) {
				t.Fatal("missing auth sentinel")
			}
		})
	}
}
func TestMobileIdentitiesRejectsInputBeforeNetwork(t *testing.T) {
	// Arrange.
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	sc := mobileRequestSession(server.Client(), server.URL)
	fn, err := mobileIdentitiesFactory(server.URL+"/api/znoise")(sc, &api{sc: sc})
	if err != nil {
		t.Fatal(err)
	}
	// Act / Assert.
	for _, ids := range [][]string{nil, {"0"}, {"01"}, {"1e0"}, {"1", "1"}, {"18446744073709551616"}, make([]string, 1001)} {
		if data, err := fn(context.Background(), ids, nil); !errors.Is(err, ErrMobileIdentities) || data != nil {
			t.Fatal("invalid request accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fn(ctx, []string{"1"}, nil); !errors.Is(err, ErrMobileIdentities) {
		t.Fatal("cancel bypass")
	}
	if calls != 0 {
		t.Fatal("invalid input reached network")
	}
}

func TestMobileIdentitiesRejectsExpandedValidPrefix(t *testing.T) {
	// Arrange: compressed HTTP body has a valid encrypted response prefix followed by excessive whitespace.
	reply := identityResponse(t, `{"error_code":0,"data":{"fids":["2"]}}`) + strings.Repeat(" ", 512<<10)
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Encoding", "gzip")
		writer := gzip.NewWriter(w)
		writer.Write([]byte(reply))
		writer.Close()
	}))
	defer server.Close()
	sc := mobileRequestSession(server.Client(), server.URL)
	fn, err := mobileIdentitiesFactory(server.URL+"/api/znoise")(sc, &api{sc: sc})
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	data, err := fn(context.Background(), []string{"1"}, nil)
	// Assert: limit is on complete expanded data, not just a decodable JSON prefix.
	if !errors.Is(err, ErrMobileIdentities) || data != nil || calls != 1 {
		t.Fatal("expanded response limit bypass")
	}
}

func TestMobileIdentityDiagnosticsRedactResponse(t *testing.T) {
	// Arrange: upstream private error text must never become a diagnostic value.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error_code":114,"error_message":"private-response-marker","data":"private-cipher"}`))
	}))
	defer server.Close()
	sc := mobileRequestSession(server.Client(), server.URL)
	fn, err := mobileIdentitiesFactory(server.URL+"/api/znoise")(sc, &api{sc: sc})
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	_, err = fn(context.Background(), []string{"1"}, nil)
	// Assert: fixed failure stage and numeric code, no raw reply or ID values.
	logs := output.String()
	if !errors.Is(err, ErrMobileIdentities) || !strings.Contains(logs, `"stage":"OUTER_ENVELOPE"`) || !strings.Contains(logs, `"upstream_error_code":114`) {
		t.Fatal("failure stage missing")
	}
	if strings.Contains(logs, "private-response-marker") || strings.Contains(logs, "private-cipher") {
		t.Fatal("private reply logged")
	}
}

func TestMobileIdentityAuthCookieIsEndpointScoped(t *testing.T) {
	// Arrange: authenticated web token has only chat scope; unrelated cookies exist.
	jar, _ := cookiejar.New(nil)
	chat, _ := url.Parse("https://chat.zalo.me/")
	target, _ := url.Parse("https://zwid.api.zalo.me/api/znoise")
	jar.SetCookies(chat, []*http.Cookie{{Name: "zpw_sek", Value: "synthetic-auth", Path: "/", Secure: true}, {Name: "private-other", Value: "synthetic-other", Path: "/"}})
	sc := session.NewContext(session.WithHTTPClient(&http.Client{Jar: jar}))
	// Act.
	headers, err := mobileIdentityHeaders(sc, target.String())
	// Assert: only the one request receives the token; jar domains remain unchanged.
	if err != nil || headers.Get("Cookie") != "zpw_sek=synthetic-auth" || len(jar.Cookies(target)) != 0 || len(jar.Cookies(chat)) != 2 {
		t.Fatal("cookie scope expanded or auth lost")
	}
	for _, base := range []string{"https://other.invalid/api/znoise", "http://zwid.api.zalo.me/api/znoise", "https://zwid.api.zalo.me/other", "https://zwid.api.zalo.me:443/api/znoise", "https://zwid.api.zalo.me/api/znoise?next=1", "https://zwid.api.zalo.me/api/znoise#fragment", "https://user@zwid.api.zalo.me/api/znoise"} {
		h, e := mobileIdentityHeaders(sc, base)
		if e != nil || len(h) != 0 {
			t.Fatal("token forwarded to a different target")
		}
	}
}

func TestMobileIdentityAuthCookieUsesDestinationJarAndRejectsMissing(t *testing.T) {
	// Arrange: no eligible auth cookie at either scope.
	jar, _ := cookiejar.New(nil)
	sc := session.NewContext(session.WithHTTPClient(&http.Client{Jar: jar}))
	target, _ := url.Parse("https://zwid.api.zalo.me/api/znoise")
	// Act / Assert: no unauthenticated fallback is sent.
	if _, err := mobileIdentityHeaders(sc, target.String()); !errors.Is(err, ErrMobileIdentities) {
		t.Fatal("missing auth accepted")
	}
	// Arrange: an existing destination cookie requires no override.
	jar.SetCookies(target, []*http.Cookie{{Name: "zpw_sek", Value: "synthetic-destination", Path: "/", Secure: true}})
	// Act.
	h, err := mobileIdentityHeaders(sc, target.String())
	// Assert: retain normal destination jar handling without duplicate tokens.
	if err != nil || len(h) != 0 {
		t.Fatal("destination cookie overridden")
	}
}

func TestMobileIdentityBorrowedCookieWireDoesNotRedirect(t *testing.T) {
	for _, status := range []int{200, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			// Arrange: production route with synthetic in-memory transport and tokens.
			jar, _ := cookiejar.New(nil)
			chat, _ := url.Parse("https://chat.zalo.me/")
			jar.SetCookies(chat, []*http.Cookie{{Name: "zpw_sek", Value: "synthetic-auth", Path: "/", Secure: true}, {Name: "other", Value: "synthetic-other", Path: "/"}})
			calls := 0
			reply := identityResponse(t, `{"error_code":0,"data":{"fids":["2"]}}`)
			client := &http.Client{Jar: jar, Transport: archiveTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "zwid.api.zalo.me" || r.URL.Path != "/api/znoise" || r.Method != "POST" || r.Header.Get("Cookie") != "zpw_sek=synthetic-auth" {
					t.Error("wrong auth scope on wire")
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.invalid/"}}, Body: io.NopCloser(strings.NewReader(reply)), Request: r}, nil
			})}
			sc := mobileRequestSession(client, "https://zwid.api.zalo.me")
			fn, err := mobileIdentitiesFactory("https://zwid.api.zalo.me/api/znoise")(sc, &api{sc: sc})
			if err != nil {
				t.Fatal(err)
			}
			// Act.
			body, err := fn(context.Background(), []string{"1"}, nil)
			// Assert: exact one request, no redirected token or persisted scope expansion.
			target, _ := url.Parse("https://zwid.api.zalo.me/api/znoise")
			if calls != 1 || len(jar.Cookies(target)) != 0 {
				t.Fatal("redirect or domain mutation")
			}
			if status == 200 && (err != nil || string(body) != `{"fids":["2"]}`) {
				t.Fatal("valid authenticated reply rejected")
			}
			if status == 302 && !errors.Is(err, ErrMobileIdentities) {
				t.Fatal("redirect accepted")
			}
		})
	}
}
