package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/internal/cryptox"
	"github.com/amrakk/zcago/session"
)

func syntheticMobilePublicKey(t *testing.T) string {
	t.Helper()
	// Invented odd modulus, no private key exists or is needed for request validation.
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, err := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func mobileRequestSession(client *http.Client, base string) session.MutableContext {
	sc := session.NewContext(session.WithHTTPClient(client), session.WithLogging(false))
	sc.SealLogin(session.Seal{UID: "owner", IMEI: "synthetic-imei", UserAgent: "synthetic", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{File: []string{base}}}})
	return sc
}

func TestMobileBackupInitialAndCancelEncryptedWire(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "cancel"}[cancel], func(t *testing.T) {
			// Arrange: TLS source bound to a synthetic authenticated session.
			public := syntheticMobilePublicKey(t)
			calls := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				path := "/api/message/pull_mobile_msg"
				if cancel {
					path = "/api/message/cancel_pull_mobile_msg"
				}
				if r.Method != "GET" || r.URL.Path != path || r.URL.Query().Get("nretry") != "0" {
					t.Error("incorrect mobile request route")
				}
				plain, err := cryptox.DecodeAESCBC([]byte(strings.Repeat("k", 32)), r.URL.Query().Get("params"))
				if err != nil {
					t.Error("request was not encrypted")
					return
				}
				d := json.NewDecoder(bytes.NewReader(plain))
				d.UseNumber()
				var p map[string]any
				if d.Decode(&p) != nil {
					t.Error("invalid request JSON")
					return
				}
				if p["public_key"] != public || p["pc_name"] != "Web" || p["imei"] != "synthetic-imei" {
					t.Error("session/correlation fields lost")
				}
				if cancel {
					if len(p) != 3 {
						t.Error("cancel has unexpected fields")
					}
				} else if len(p) != 7 || p["from_seq_id"] != json.Number("0") || p["min_seq_id"] != json.Number("0") || p["is_retry"] != json.Number("0") || p["temp_key"] != "" {
					t.Error("initial request was not bounded to zero/retry disabled")
				}
				cipher, err := cryptox.EncodeAESCBC([]byte(strings.Repeat("k", 32)), `{"error_code":0,"data":""}`, cryptox.EncryptTypeBase64)
				if err != nil {
					t.Error("cannot create synthetic acknowledgement")
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": cipher})
			}))
			defer srv.Close()
			sc := mobileRequestSession(srv.Client(), srv.URL)
			a := &api{sc: sc}
			// Act.
			var err error
			if cancel {
				err = a.CancelMobileBackup(context.Background(), public)
			} else {
				err = a.RequestMobileBackup(context.Background(), public)
			}
			// Assert: one acknowledgement, no implicit polling or repeated request.
			if err != nil || calls != 1 {
				t.Fatalf("request acknowledgement: %v, calls %d", err, calls)
			}
		})
	}
}

func TestMobileBackupNoRedirectAndSafeFailures(t *testing.T) {
	for _, status := range []int{302, 401, 403, 500} {
		// Arrange: redirect target must never be reached; response content is untrusted.
		calls := 0
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Location", "/unexpected-repeat")
			w.WriteHeader(status)
			w.Write([]byte("synthetic-private-detail"))
		}))
		sc := mobileRequestSession(srv.Client(), srv.URL)
		a := &api{sc: sc}
		// Act.
		err := a.RequestMobileBackup(context.Background(), syntheticMobilePublicKey(t))
		srv.Close()
		// Assert.
		if err == nil || calls != 1 || strings.Contains(err.Error(), "synthetic-private-detail") {
			t.Fatal("unsafe response or redirect retry")
		}
		if status == 401 && !errors.Is(err, errs.ErrAuthenticationRequired) {
			t.Fatal("authentication state lost")
		}
		if (status == 302 || status == 500) && !errors.Is(err, ErrMobileBackupUnknown) {
			t.Fatal("unknown dispatch claimed rejected")
		}
	}
}

func TestMobileBackupRejectsInputBeforeDispatch(t *testing.T) {
	// Arrange.
	calls := 0
	client := &http.Client{Transport: preloadTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("synthetic-private-detail")
	})}
	sc := mobileRequestSession(client, "https://synthetic.invalid")
	a := &api{sc: sc}
	for _, public := range []string{"", "invalid", syntheticMobilePublicKey(t) + "\n"} {
		// Act / Assert.
		if err := a.RequestMobileBackup(context.Background(), public); !errors.Is(err, ErrMobileBackupInput) {
			t.Fatal("bad public key accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.RequestMobileBackup(ctx, syntheticMobilePublicKey(t)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled request dispatched")
	}
	if calls != 0 {
		t.Fatal("invalid request contacted network")
	}
	// Act / Assert: a post-dispatch network error is closed and never resent.
	err := a.RequestMobileBackup(context.Background(), syntheticMobilePublicKey(t))
	if err != ErrMobileBackupUnknown || calls != 1 {
		t.Fatal("network error leaked or retried")
	}
	missing := session.NewContext(session.WithLogging(false))
	if err := (&api{sc: missing}).RequestMobileBackup(context.Background(), syntheticMobilePublicKey(t)); err != ErrMobileBackupUnavailable {
		t.Fatal("missing file service guessed")
	}
}

func TestMobileBackupAcknowledgementBoundsAndShape(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		// Arrange: valid acknowledgement prefix followed by excessive whitespace.
		raw := []byte(`{"error_code":0,"data":""}` + strings.Repeat(" ", 65<<10))
		header := http.Header{}
		if compressed {
			var b bytes.Buffer
			z := gzip.NewWriter(&b)
			z.Write(raw)
			z.Close()
			raw = b.Bytes()
			header.Set("Content-Encoding", "gzip")
		}
		body := &preloadCountedBody{Reader: bytes.NewReader(raw)}
		client := &http.Client{Transport: preloadTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: header, Body: body, Request: r}, nil
		})}
		a := &api{sc: mobileRequestSession(client, "https://synthetic.invalid")}
		// Act.
		err := a.RequestMobileBackup(context.Background(), syntheticMobilePublicKey(t))
		// Assert.
		if err != ErrMobileBackupUnknown || body.read > (64<<10)+1 {
			t.Fatal("acknowledgement budget bypassed")
		}
	}
	for _, body := range []string{`{}`, `{"error_code":null}`, `{"error_code":0,"data":"not ciphertext"}`} {
		if err := mobileBackupAcknowledgement([]byte(body), []byte(strings.Repeat("k", 32))); err != ErrMobileBackupUnknown {
			t.Fatal("malformed acknowledgement accepted")
		}
	}
	// Arrange / Act: upstream text is never retained in errors.
	err := mobileBackupAcknowledgement([]byte(`{"error_code":114,"error_message":"synthetic-private-detail"}`), nil)
	var apiErr errs.ZaloAPIError
	// Assert.
	if !errors.As(err, &apiErr) || apiErr.Code == nil || *apiErr.Code != 114 || strings.Contains(err.Error(), "synthetic-private-detail") {
		t.Fatal("code lost or detail leaked")
	}
}
