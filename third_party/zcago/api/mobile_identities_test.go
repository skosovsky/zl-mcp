package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
		if r.Method != "POST" || r.URL.Path != "/api/znoise" || r.URL.Query().Get("nretry") != "0" {
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
