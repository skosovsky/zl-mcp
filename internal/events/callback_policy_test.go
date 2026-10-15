package events

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestLocalCallbackPolicyDoesNotBroadenPublicTransport(t *testing.T) {
	// Arrange: one allowed URL and another local server that must never be reached.
	var hits atomic.Int32
	other := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); w.WriteHeader(http.StatusNoContent) },
		),
	)
	t.Cleanup(other.Close)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, other.URL+"/events", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(receiver.Close)
	urls := []string{receiver.URL + "/events", receiver.URL + "/redirect"}
	policy, err := NewCallbackPolicy(urls)
	if err != nil {
		t.Fatal(err)
	}
	urls[0] = other.URL + "/events" // Constructor must own its allowlist.
	secret := "whsec_" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	// Act: exercise the actual network client, not an injected routing transport.
	for _, target := range []string{receiver.URL + "/other", other.URL + "/events"} {
		response, requestErr := postCallbackWithPolicy(
			t.Context(),
			policy.Client(),
			target,
			secret,
			"sub",
			"event",
			[]byte("{}"),
			policy,
		)
		if response != nil {
			response.Body.Close()
		}
		// Assert: forbidden endpoints must not be dialed.
		if requestErr == nil {
			t.Fatal("non-allowlisted local request accepted")
		}
	}
	if hits.Load() != 0 {
		t.Fatal("forbidden endpoint received a request")
	}
	response, err := postCallbackWithPolicy(
		t.Context(),
		policy.Client(),
		receiver.URL+"/events",
		secret,
		"sub",
		"event",
		[]byte("{}"),
		policy,
	)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if hits.Load() != 1 {
		t.Fatal("allowed endpoint not reached exactly once")
	}
	response, err = postCallbackWithPolicy(
		t.Context(),
		policy.Client(),
		receiver.URL+"/redirect",
		secret,
		"sub",
		"event",
		[]byte("{}"),
		policy,
	)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || hits.Load() != 2 {
		t.Fatal("redirect was followed")
	}
	// Assert: the default mode rejects the same loopback URL before sending.
	response, err = postCallback(
		context.Background(),
		NewCallbackClient(),
		receiver.URL+"/events",
		secret,
		"sub",
		"event",
		[]byte("{}"),
	)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || hits.Load() != 2 {
		t.Fatal("default callback policy broadened")
	}
}

func TestLocalHTTPSKeepsCertificateVerification(t *testing.T) {
	// Arrange: the fixture certificate is intentionally not trusted by the default client.
	receiver := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	)
	t.Cleanup(receiver.Close)
	policy, err := NewCallbackPolicy([]string{receiver.URL + "/events"})
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	response, err := policy.Client().Post(receiver.URL+"/events", "application/json", nil)
	if response != nil {
		response.Body.Close()
	}
	// Assert.
	if err == nil {
		t.Fatal("untrusted local TLS certificate accepted")
	}
}
