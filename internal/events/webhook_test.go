package events

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

type callbackRoundTrip func(*http.Request) (*http.Response, error)

func (f callbackRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerificationSignatureAndChallenge(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "wrong-challenge"}[mismatch], func(t *testing.T) {
			// Arrange: independently validate the Standard Webhooks wire format.
			key := []byte("synthetic-key-for-webhook-32bytes!")
			secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
			calls := 0
			client := &http.Client{Transport: callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				mac := hmac.New(sha256.New, key)
				mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
				mac.Write(body)
				decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
				if err != nil || !hmac.Equal(decoded, mac.Sum(nil)) {
					t.Fatal("Standard Webhooks signature mismatch")
				}
				if r.Header.Get("X-MCP-Subscription-Id") != "synthetic-subscription" || r.Header.Get("Content-Type") != "application/json" || r.Method != "POST" {
					t.Fatal("missing required headers")
				}
				var payload map[string]string
				if err := json.Unmarshal(body, &payload); err != nil || payload["type"] != "verification" || len(payload["challenge"]) < 32 {
					t.Fatal("invalid verification payload")
				}
				if mismatch {
					payload["challenge"] = "not-the-challenge"
				}
				responseBody, _ := json.Marshal(map[string]string{"challenge": payload["challenge"]})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(responseBody))), Header: http.Header{}}, nil
			})}

			// Act.
			err := verifyCallback(context.Background(), client, "https://callback.example/events", secret, "synthetic-subscription")

			// Assert.
			if calls != 1 || (err != nil) != mismatch {
				t.Fatalf("calls=%d err=%v mismatch=%v", calls, err, mismatch)
			}
		})
	}
}

func TestCallbackBlocksNonPublicAndRedirect(t *testing.T) {
	// Arrange.
	unsafe := []string{"http://example.com", "https://user:secret@example.com", "https://127.0.0.1", "https://[::1]", "https://169.254.169.254", "https://192.168.1.1", "https://100.64.0.1", "https://203.0.113.1", "https://[::ffff:127.0.0.1]", "https://example.com/#fragment"}
	for _, raw := range unsafe {
		// Act.
		_, err := callbackURL(raw)
		// Assert.
		if err == nil {
			t.Fatalf("accepted unsafe callback %q", raw)
		}
	}
	// Act / Assert: the production client refuses every redirect.
	client := newCallbackClient()
	if client.CheckRedirect(&http.Request{}, nil) == nil {
		t.Fatal("redirect was allowed")
	}
}

func TestCallbackDNSPinnedAndRechecked(t *testing.T) {
	// Arrange: DNS changes from a public address to loopback on the next call.
	lookups, dials := 0, 0
	dial := validatedDial(func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		if lookups == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}, func(ctx context.Context, network, address string) (net.Conn, error) {
		dials++
		if address != "8.8.8.8:443" {
			t.Fatalf("hostname resolved again instead of pinning: %q", address)
		}
		return nil, errors.New("synthetic dial failure")
	})

	// Act.
	_, firstErr := dial(context.Background(), "tcp", "callback.example:443")
	_, secondErr := dial(context.Background(), "tcp", "callback.example:443")

	// Assert: no connection attempted to the rebound address.
	if firstErr == nil || secondErr == nil || lookups != 2 || dials != 1 {
		t.Fatalf("lookups=%d dials=%d errors=%v/%v", lookups, dials, firstErr, secondErr)
	}
}

func TestCallbackBoundsAndRedactsErrors(t *testing.T) {
	// Arrange.
	calls := 0
	client := &http.Client{Transport: callbackRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("private-response-marker")
	})}
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))

	// Act.
	_, oversizedErr := postCallback(context.Background(), client, "https://example.com", secret, "s", "e", []byte(strings.Repeat("x", 262145)))
	_, invalidKeyErr := postCallback(context.Background(), client, "https://example.com", "secret-marker", "s", "e", []byte("{}"))
	_, requestErr := postCallback(context.Background(), client, "https://example.com/private-url-marker", secret, "s", "e", []byte("{}"))

	// Assert.
	if calls != 1 || oversizedErr == nil || invalidKeyErr == nil || requestErr == nil {
		t.Fatalf("calls=%d errors=%v/%v/%v", calls, oversizedErr, invalidKeyErr, requestErr)
	}
	for _, err := range []error{invalidKeyErr, requestErr} {
		if strings.Contains(err.Error(), "marker") || strings.Contains(err.Error(), "https://") {
			t.Fatal("private callback data leaked through error")
		}
	}
}
