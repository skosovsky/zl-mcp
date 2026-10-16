package mcpserver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestActivityHTTPJournalSignedTLSAndCancellation(t *testing.T) {
	for _, family := range []string{"reaction", "call"} {
		t.Run(family, func(t *testing.T) { testActivityHTTPJournalSignedTLSAndCancellation(t, family) })
	}
}

//nolint:gocognit,govet // Keep callback, recovery and ack evidence in one AAA scenario.
func testActivityHTTPJournalSignedTLSAndCancellation(t *testing.T, family string) {
	t.Helper()
	profile := domain.ReactionObservedV1
	if family == "call" {
		profile = domain.CallObservedV1
	}
	// Arrange: no message/target row; independent receiver verifies the real bytes.
	ctx := context.Background()
	dir := t.TempDir()
	store, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(dir, "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := []byte(strings.Repeat("k", 32))
	received := make(chan map[string]any, 2)
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Error(readErr)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("Webhook-Id") + "." + r.Header.Get("Webhook-Timestamp") + "."))
		mac.Write(body)
		signature, decodeErr := base64.StdEncoding.DecodeString(
			strings.TrimPrefix(r.Header.Get("Webhook-Signature"), "v1,"),
		)
		if decodeErr != nil || !hmac.Equal(signature, mac.Sum(nil)) {
			t.Error("invalid activity signature")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if value["type"] == "verification" {
			json.NewEncoder(w).Encode(map[string]any{"challenge": value["challenge"]})
			return
		}
		if value["eventId"] != r.Header.Get("Webhook-Id") {
			t.Error("wrong activity header")
		}
		received <- value
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	target, _ := url.Parse(receiver.URL)
	client := &http.Client{Transport: localReceiverTransport{target: target, base: receiver.Client().Transport}}
	token := strings.Repeat("t", 32)
	service, err := migrationHTTP(t, store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	service.Events.(*events.SubscriptionManager).Client = client
	endpoint := httptest.NewServer(service)
	defer endpoint.Close()
	worker, err := events.NewWorker(store)
	if err != nil {
		t.Fatal(err)
	}
	worker.Client = client
	args := map[string]any{"scope": "direct", "direction": "incoming"}
	delivery := map[string]any{
		"mode":   "webhook",
		"url":    "https://receiver.example/events",
		"secret": "whsec_" + base64.StdEncoding.EncodeToString(key),
	}
	sub := httpRPC(
		t,
		endpoint.URL+"/mcp",
		token,
		"events/subscribe",
		map[string]any{"name": profile, "arguments": args, "delivery": delivery, "ttlMs": nil},
	)
	// Act: observe both directions; only incoming becomes this subscription's job.
	at := time.Now().UTC()
	for _, direction := range []string{"incoming", "outgoing"} {
		a := domain.Activity{
			Family:           family,
			SchemaVersion:    1,
			ConversationType: "direct",
			ConversationID:   "peer",
			ActorID:          "actor",
			OccurredAt:       at,
			Direction:        direction,
			SourceID:         direction,
			IdentityBasis:    "upstream_action_id",
			Source:           "live",
			Reaction:         &domain.ReactionObservation{TargetMessageIDs: []string{}, Action: "unknown"},
		}
		if family == "call" {
			a.Reaction = nil
			a.Call = &domain.CallObservation{Status: "unknown", Mode: "unknown"}
		}
		if err := store.PutActivity(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.FanoutActivityEvents(ctx, at, worker.Policy, worker.Encoder.EncodeActivity); err != nil {
		t.Fatal(err)
	}
	if ok, err := worker.DeliverOne(ctx, at); err != nil || !ok {
		t.Fatal(ok, err)
	}
	// Assert: callback and payload-less HTTP recovery contain the same activity union.
	var callback map[string]any
	select {
	case callback = <-received:
	default:
		t.Fatal("signed activity not received")
	}
	result := httpRPC(
		t,
		endpoint.URL+"/mcp",
		token,
		"tools/call",
		map[string]any{
			"name":      "zalo_read_subscription_events",
			"arguments": map[string]any{"subscription_id": sub["id"]},
		},
	)
	if result["isError"] == true {
		t.Fatal("typed activity projection failed", result)
	}
	page := result["structuredContent"].(map[string]any)
	entries := page["events"].([]any)
	if len(entries) != 1 {
		t.Fatal("direction filter or recovery wrong")
	}
	entry := entries[0].(map[string]any)
	recovered := entry["event"].(map[string]any)
	left, _ := json.Marshal(callback)
	right, _ := json.Marshal(recovered)
	if string(left) != string(right) {
		t.Fatal("activity changed in HTTP recovery")
	}
	data := recovered["data"].(map[string]any)
	if data["family"] != family || data["message_id"] != nil || data["first_incoming"] != nil {
		t.Fatal("activity fabricated message facts")
	}
	ackArgs := map[string]any{"subscription_id": sub["id"], "receipt": entry["receipt"]}
	for range 2 {
		httpRPC(
			t,
			endpoint.URL+"/mcp",
			token,
			"tools/call",
			map[string]any{"name": "zalo_ack_subscription_events", "arguments": ackArgs},
		)
	}
	if ok, err := worker.DeliverOne(ctx, at); err != nil || ok {
		t.Fatal("outgoing reaction leaked", ok, err)
	}
	// Act / Assert: cancellation uses the exact same typed selector identity.
	delete(delivery, "secret")
	httpRPC(
		t,
		endpoint.URL+"/mcp",
		token,
		"events/unsubscribe",
		map[string]any{"name": profile, "arguments": args, "delivery": delivery},
	)
	var active int
	if err := store.DB.QueryRow(`SELECT count(*) FROM event_subscriptions WHERE active=1`).
		Scan(&active); err != nil ||
		active != 0 {
		t.Fatal("activity subscription still active", active, err)
	}
}
