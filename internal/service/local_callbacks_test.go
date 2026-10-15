package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:cyclop,gocognit,gocyclo // One AAA scenario proves configured transport, signed delivery and queued permission revocation without injected network routing.
func TestConfiguredLocalCallbacksVerifyDeliverAndRevoke(t *testing.T) {
	// Arrange: actual configured endpoints and SQLite; only Zalo ingestion is synthetic.
	ctx := t.Context()
	store, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "messages.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	key := []byte(strings.Repeat("s", 32))
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	var verified, delivered atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			t.Error("invalid signature")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Mcp-Subscription-Id") == "" {
			t.Error("missing subscription binding")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var envelope map[string]any
		if decodeErr = json.Unmarshal(body, &envelope); decodeErr != nil {
			t.Error(decodeErr)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if envelope["type"] == "verification" {
			verified.Add(1)
			if encodeErr := json.NewEncoder(w).
				Encode(map[string]any{"challenge": envelope["challenge"]}); encodeErr != nil {
				t.Error(encodeErr)
			}
			return
		}
		data, ok := envelope["data"].(map[string]any)
		if !ok || data["message_id"] != "local-message" || data["text"] != "synthetic local message" ||
			envelope["eventId"] != r.Header.Get("Webhook-Id") {
			t.Error("event identity or payload mismatch")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		delivered.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(receiver.Close)
	var c config.Config
	c.StateDir = t.TempDir()
	c.Events.LocalCallbackURLs = []string{receiver.URL + "/events"}
	token := strings.Repeat("t", 64)
	handler, worker, trace, err := configuredEndpoints(c, store, nil, token)
	if err != nil {
		t.Fatal(err)
	}
	if trace != nil {
		t.Fatal("unexpected trace logger")
	}
	endpoint := httptest.NewServer(handler)
	t.Cleanup(endpoint.Close)
	args := map[string]any{
		"scope":               "conversation",
		"conversation_type":   "direct",
		"conversation_id":     "peer",
		"direction":           "incoming",
		"first_incoming_only": false,
	}
	params := func(sign bool) map[string]any {
		delivery := map[string]any{"mode": "webhook", "url": receiver.URL + "/events"}
		if sign {
			delivery["secret"] = secret
		}
		request := map[string]any{"name": domain.ConversationMessageCreatedV2, "arguments": args, "delivery": delivery}
		if sign {
			request["ttlMs"] = 60000
		}
		return request
	}
	// Act: no injected callback transport, DNS override or public tunnel.
	sub := serviceRPC(t, endpoint.URL+"/mcp", token, "events/subscribe", params(true))
	if err = store.Put(
		ctx,
		domain.Message{
			Conversation: domain.ConversationRef{Type: "direct", ID: "peer"},
			ID:           "local-message",
			SenderID:     "peer",
			Direction:    "incoming",
			Text:         "synthetic local message",
			SentAt:       time.Now(),
			Source:       "live",
		},
	); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if _, err = store.FanoutProfileEvents(ctx, at, worker.Policy, worker.Encoder.EncodeProfile); err != nil {
		t.Fatal(err)
	}
	if handled, deliveryErr := worker.DeliverOne(ctx, at); deliveryErr != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, deliveryErr)
	}
	// Assert: independent signature verification and a genuine stored delivered receipt.
	if verified.Load() != 1 || delivered.Load() != 1 {
		t.Fatalf("verification=%d delivery=%d", verified.Load(), delivered.Load())
	}
	var state string
	if err = store.DB.QueryRow("SELECT state FROM event_deliveries WHERE subscription_id=?", sub["id"]).
		Scan(&state); err != nil ||
		state != "delivered" {
		t.Fatalf("state=%s err=%v", state, err)
	}
	// Arrange: another queued message, then reload endpoints with the local permission removed.
	if err = store.Put(
		ctx,
		domain.Message{
			Conversation: domain.ConversationRef{Type: "direct", ID: "peer"},
			ID:           "revoked-message",
			SenderID:     "peer",
			Direction:    "incoming",
			Text:         "must not be delivered",
			SentAt:       time.Now(),
			Source:       "live",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = store.FanoutProfileEvents(ctx, at, worker.Policy, worker.Encoder.EncodeProfile); err != nil {
		t.Fatal(err)
	}
	c.Events.LocalCallbackURLs = nil
	revoked, newWorker, _, err := configuredEndpoints(c, store, nil, token)
	if err != nil {
		t.Fatal(err)
	}
	revokedEndpoint := httptest.NewServer(revoked)
	t.Cleanup(revokedEndpoint.Close)
	// Act: policy is checked for queued delivery; cancellation must still be possible.
	if handled, deliveryErr := newWorker.DeliverOne(ctx, at); deliveryErr != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, deliveryErr)
	}
	serviceRPC(t, revokedEndpoint.URL+"/mcp", token, "events/unsubscribe", params(false))
	// Assert: no additional callback, and the subscription can be cancelled after revocation.
	if delivered.Load() != 1 || verified.Load() != 1 {
		t.Fatal("revoked receiver was contacted")
	}
	var active int
	if err = store.DB.QueryRow("SELECT active FROM event_subscriptions WHERE id=?", sub["id"]).
		Scan(&active); err != nil ||
		active != 0 {
		t.Fatalf("active=%d err=%v", active, err)
	}
}
