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

func TestConversationEventsThroughHTTPAndSignedTLSReceiver(t *testing.T) {
	// Arrange: an all-conversation service and an independent signed TLS receiver.
	ctx := context.Background()
	dir := t.TempDir()
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(dir, "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := []byte("synthetic-key-for-webhook-32bytes!")
	received := make(chan map[string]any, 4)
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
		mac.Write(body)
		sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
		if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
			t.Error("invalid callback signature")
			w.WriteHeader(400)
			return
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if value["type"] == "verification" {
			json.NewEncoder(w).Encode(map[string]any{"challenge": value["challenge"]})
			return
		}
		received <- value
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	target, _ := url.Parse(receiver.URL)
	client := &http.Client{Transport: localReceiverTransport{target: target, base: receiver.Client().Transport}}
	token := strings.Repeat("t", 32)
	service, err := NewHTTP(store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	service.Events.Client = client
	endpoint := httptest.NewServer(service)
	defer endpoint.Close()
	worker, err := events.NewWorker(store)
	if err != nil {
		t.Fatal(err)
	}
	worker.Client = client
	params := map[string]any{
		"name":      events.ConversationMessageCreated,
		"arguments": map[string]any{"scope": "all"},
		"delivery":  map[string]any{"mode": "webhook", "url": "https://receiver.example/events", "secret": "whsec_" + base64.StdEncoding.EncodeToString(key)},
		"ttlMs":     nil,
	}
	fullText := strings.Repeat("界", 2049)
	// Act: activate through MCP, insert both conversation types and deliver via TLS.
	httpRPC(t, endpoint.URL+"/mcp", token, "events/subscribe", params)
	for _, kind := range []string{domain.ConversationDirect, domain.ConversationGroup} {
		if err := store.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: "same/id"}, ID: "same/message", SenderID: "author", SentAt: time.Now().Add(-time.Hour), Text: fullText, Source: "replay"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.FanoutProfileEvents(ctx, time.Now(), worker.Policy, worker.Encoder.EncodeProfile); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if worked, err := worker.DeliverOne(ctx, time.Now()); err != nil || !worked {
			t.Fatalf("delivery worked=%v error=%v", worked, err)
		}
	}
	// Assert: typed identities and escaped full-text resources survive the MCP wire.
	seen := map[string]bool{}
	for range 2 {
		var event map[string]any
		select {
		case event = <-received:
		default:
			t.Fatal("TLS receiver did not receive callback")
		}
		data := event["data"].(map[string]any)
		kind := data["conversation_type"].(string)
		if seen[kind] || event["name"] != events.ConversationMessageCreated || data["schema_version"] != float64(1) || data["conversation_id"] != "same/id" || data["text_truncated"] != true || len([]rune(data["text"].(string))) != 2048 {
			t.Fatalf("invalid typed event: %v", event)
		}
		seen[kind] = true
		resource := httpRPC(t, endpoint.URL+"/mcp", token, "resources/read", map[string]any{"uri": data["text_resource_uri"]})
		if resource["contents"].([]any)[0].(map[string]any)["text"] != fullText {
			t.Fatal("full text resource mismatch")
		}
	}
	if !seen[domain.ConversationDirect] || !seen[domain.ConversationGroup] {
		t.Fatal("missing conversation type")
	}
	// Act: cancel through MCP and insert another record.
	params["delivery"] = map[string]any{"mode": "webhook", "url": "https://receiver.example/events"}
	delete(params, "ttlMs")
	httpRPC(t, endpoint.URL+"/mcp", token, "events/unsubscribe", params)
	if err := store.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: domain.ConversationDirect, ID: "another"}, ID: "after-cancel", SenderID: "author", SentAt: time.Now(), Text: "synthetic", Source: "live"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FanoutProfileEvents(ctx, time.Now(), worker.Policy, worker.Encoder.EncodeProfile); err != nil {
		t.Fatal(err)
	}
	// Assert: cancellation stops actual callbacks for the broad scope.
	if worked, err := worker.DeliverOne(ctx, time.Now()); err != nil || worked || len(received) != 0 {
		t.Fatalf("cancelled delivery worked=%v error=%v", worked, err)
	}
}
