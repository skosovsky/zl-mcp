package events

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func recoveryRequest(t *testing.T, business, generation, callback, secret string) json.RawMessage {
	t.Helper()
	delivery := map[string]any{"mode": "webhook", "url": callback}
	if secret != "" {
		delivery["secret"] = secret
	}
	wire, err := json.Marshal(
		map[string]any{
			"name":      domain.JournalReadyV1,
			"arguments": map[string]any{"subscription_id": business, "generation_reference": generation},
			"delivery":  delivery,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func recoveryChallenge(r *http.Request) (*http.Response, error) {
	var challenge map[string]string
	if err := json.NewDecoder(r.Body).Decode(&challenge); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(map[string]string{"challenge": challenge["challenge"]})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(string(wire))),
	}, nil
}

//nolint:gocognit // This AAA scenario verifies ownership, canonical identity and explicit companion replacement together.
func TestRecoveryManagerOwnedGenerationIdentityAndControlIsolation(t *testing.T) {
	// Arrange: a real owned business subscription and its exact generation.
	ctx := context.Background()
	_, s, _ := readyWorker(t, callbackRoundTrip(recoveryChallenge))
	m, err := NewSubscriptionManager(s, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	m.Client = &http.Client{Transport: callbackRoundTrip(recoveryChallenge)}
	var gen, secret string
	if err = s.DB.QueryRow("SELECT generation,secret FROM event_subscriptions WHERE id='subscription'").
		Scan(&gen, &secret); err != nil {
		t.Fatal(err)
	}
	request := recoveryRequest(t, "subscription", gen, "https://callback.example/control", secret)
	// Act: identical calls renew one identity without entering business storage.
	first, err := m.Call(ctx, methodEventsSubscribe, "owner", request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Call(ctx, methodEventsSubscribe, "owner", request)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: control stays out of typed business classification and journal fanout.
	if first.(map[string]any)["id"] != second.(map[string]any)["id"] ||
		domain.IsTypedEventProfile(domain.JournalReadyV1) {
		t.Fatal("control identity/classification")
	}
	var business, controls int
	if err = s.DB.QueryRow("SELECT count(*) FROM event_subscriptions").Scan(&business); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM recovery_subscriptions WHERE active=1").Scan(&controls); err != nil {
		t.Fatal(err)
	}
	if business != 1 || controls != 1 {
		t.Fatalf("business=%d controls=%d", business, controls)
	}
	for _, tc := range []struct{ owner, generation, business string }{{"stranger", gen, "subscription"}, {"owner", "stale-generation", "subscription"}, {"owner", gen, "missing"}} {
		_, err = m.Call(
			ctx,
			methodEventsSubscribe,
			tc.owner,
			recoveryRequest(t, tc.business, tc.generation, "https://callback.example/control", secret),
		)
		var rpc *RPCError
		if !errors.As(err, &rpc) || rpc.Data["reason"] != "recovery_target_unavailable" {
			t.Fatalf("unexpected access result: %v", err)
		}
	}
	// Act / Assert: only one companion may own this business generation.
	otherRequest := recoveryRequest(t, "subscription", gen, "https://callback.example/control-2", secret)
	if _, conflictErr := m.Call(ctx, methodEventsSubscribe, "owner", otherRequest); conflictErr == nil {
		t.Fatal("second active companion accepted")
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM recovery_subscriptions WHERE active=1").Scan(&controls); err != nil {
		t.Fatal(err)
	}
	if controls != 1 {
		t.Fatalf("conflict changed active companions=%d", controls)
	}
	if _, err = m.Call(
		ctx,
		methodEventsUnsubscribe,
		"owner",
		recoveryRequest(t, "subscription", gen, "https://callback.example/control", ""),
	); err != nil {
		t.Fatal(err)
	}
	// Explicit cancellation allows replacement; callback remains part of identity.
	other, err := m.Call(ctx, methodEventsSubscribe, "owner", otherRequest)
	if err != nil {
		t.Fatal(err)
	}
	if first.(map[string]any)["id"] == other.(map[string]any)["id"] {
		t.Fatal("callback identity collided")
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM event_subscriptions WHERE active=1").Scan(&business); err != nil {
		t.Fatal(err)
	}
	if business != 1 {
		t.Fatal("control cancellation changed business subscription")
	}
}

func TestRecoveryCancellationDuringVerificationWins(t *testing.T) {
	// Arrange: durable business target with challenge paused in flight.
	ctx := context.Background()
	_, s, _ := readyWorker(t, callbackRoundTrip(recoveryChallenge))
	m, err := NewSubscriptionManager(s, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	var gen, secret string
	if err = s.DB.QueryRow("SELECT generation,secret FROM event_subscriptions WHERE id='subscription'").
		Scan(&gen, &secret); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	m.Client = &http.Client{Transport: callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return recoveryChallenge(r)
	})}
	result := make(chan error, 1)
	go func() {
		_, err := m.Call(
			ctx,
			methodEventsSubscribe,
			"owner",
			recoveryRequest(t, "subscription", gen, "https://callback.example/control", secret),
		)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("challenge did not start")
	}
	// Act: same identity is cancelled before challenge acceptance.
	_, cancelErr := m.Call(
		ctx,
		methodEventsUnsubscribe,
		"owner",
		recoveryRequest(t, "subscription", gen, "https://callback.example/control", ""),
	)
	close(release)
	// Assert: revision check prevents resurrection.
	if cancelErr != nil {
		t.Fatal(cancelErr)
	}
	var rpc *RPCError
	if err := <-result; !errors.As(err, &rpc) || rpc.Data["reason"] != "cancelled" {
		t.Fatalf("activation error=%v", err)
	}
}

func TestRecoveryManagerUnsupportedAndStrictArguments(t *testing.T) {
	// Arrange: a legacy-only storage port deliberately lacks the optional outbox.
	ctx := context.Background()
	_, s, _ := readyWorker(t, callbackRoundTrip(recoveryChallenge))
	m, err := NewSubscriptionManager(struct{ Subscriptions }{s}, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	var gen, secret string
	if err = s.DB.QueryRow("SELECT generation,secret FROM event_subscriptions WHERE id='subscription'").
		Scan(&gen, &secret); err != nil {
		t.Fatal(err)
	}
	raw := recoveryRequest(t, "subscription", gen, "https://callback.example/control", secret)
	// Act / Assert: explicit unsupported, strict unknown fields, and no authentication bypass.
	cases := []struct {
		principal string
		raw       json.RawMessage
		reason    string
	}{{"owner", raw, "recovery_unsupported"}, {"", raw, "unauthenticated"}}
	var invalid map[string]any
	if err = json.Unmarshal(raw, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid["arguments"].(map[string]any)["text"] = "untrusted extra"
	invalidRaw, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	cases = append(cases, struct {
		principal string
		raw       json.RawMessage
		reason    string
	}{"owner", invalidRaw, "invalid_params"})
	for _, tc := range cases {
		_, err = m.Call(ctx, methodEventsSubscribe, tc.principal, tc.raw)
		var rpc *RPCError
		if !errors.As(err, &rpc) || rpc.Data["reason"] != tc.reason {
			t.Fatalf("expected %s got %v", tc.reason, err)
		}
	}
}
