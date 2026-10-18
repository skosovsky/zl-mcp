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
	for _, profile := range []struct {
		name    string
		version float64
	}{
		{domain.ConversationMessageCreatedV2, 2},
		{domain.ConversationMessageCreatedV3, 3},
	} {
		t.Run(
			profile.name,
			func(t *testing.T) { testConversationProfileTLS(t, profile.name, profile.version) },
		)
	}
}

//nolint:cyclop,gocognit,gocyclo // This test helper checks the complete fixture/metric shape without dropping nested boundary assertions.
func testConversationProfileTLS(t *testing.T, profileName string, profileVersion float64) {
	// Arrange: an all-conversation service and an independent signed TLS receiver.
	ctx := context.Background()
	dir := t.TempDir()
	store, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(dir, "messages.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := []byte("synthetic-key-for-webhook-32bytes!")
	received := make(chan map[string]any, 4)
	var verificationSubscription, expectedSubscription string
	receiver := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, readAllErr := io.ReadAll(r.Body)
			if readAllErr != nil {
				t.Error(readAllErr)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mac := hmac.New(sha256.New, key)
			mac.Write(
				[]byte(r.Header.Get("Webhook-Id") + "." + r.Header.Get("Webhook-Timestamp") + "."),
			)
			mac.Write(body)
			sig, readAllErr := base64.StdEncoding.DecodeString(
				strings.TrimPrefix(r.Header.Get("Webhook-Signature"), "v1,"),
			)
			if readAllErr != nil || !hmac.Equal(sig, mac.Sum(nil)) {
				t.Error("invalid callback signature")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var value map[string]any
			if unmarshalErr := json.Unmarshal(body, &value); unmarshalErr != nil {
				t.Error(unmarshalErr)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if value["type"] == "verification" {
				verificationSubscription = r.Header.Get("X-Mcp-Subscription-Id")
				json.NewEncoder(w).Encode(map[string]any{"challenge": value["challenge"]})
				return
			}
			if r.Header.Get("Content-Type") != "application/json" ||
				r.Header.Get("Webhook-Id") != value["eventId"] ||
				r.Header.Get("X-Mcp-Subscription-Id") != expectedSubscription ||
				expectedSubscription == "" {
				t.Error("callback routing headers differ from subscribed identity")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			received <- value
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	defer receiver.Close()
	target, _ := url.Parse(receiver.URL)
	client := &http.Client{
		Transport: localReceiverTransport{target: target, base: receiver.Client().Transport},
	}
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
	params := map[string]any{
		"name":      profileName,
		"arguments": map[string]any{"scope": "all"},
		"delivery": map[string]any{
			"mode":   "webhook",
			"url":    "https://receiver.example/events",
			"secret": "whsec_" + base64.StdEncoding.EncodeToString(key),
		},
		"ttlMs": nil,
	}
	fullText := strings.Repeat("界", 2049)
	// Act: activate through MCP, insert both conversation types and deliver via TLS.
	subscription := httpRPC(t, endpoint.URL+"/mcp", token, "events/subscribe", params)
	expectedSubscription = subscription["id"].(string)
	if verificationSubscription != expectedSubscription {
		t.Fatal("verification and subscription IDs differ")
	}
	for _, kind := range []string{domain.ConversationDirect, domain.ConversationGroup} {
		if err := store.Put(
			ctx,
			domain.Message{
				Conversation: domain.ConversationRef{Type: kind, ID: "same/id"},
				ID:           "same/message",
				SenderID:     "author",
				SentAt:       time.Now().Add(-time.Hour),
				Text:         fullText,
				Source:       "replay",
			},
		); err != nil {
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
		if seen[kind] || event["name"] != profileName || data["schema_version"] != profileVersion ||
			data["conversation_id"] != "same/id" ||
			data["text_truncated"] != true ||
			len([]rune(data["text"].(string))) != 2048 {
			t.Fatalf("invalid typed event: %v", event)
		}
		seen[kind] = true
		resource := httpRPC(
			t,
			endpoint.URL+"/mcp",
			token,
			"resources/read",
			map[string]any{"uri": data["text_resource_uri"]},
		)
		if resource["contents"].([]any)[0].(map[string]any)["text"] != fullText {
			t.Fatal("full text resource mismatch")
		}
		if profileName == domain.ConversationMessageCreatedV3 {
			metadata := httpRPC(
				t,
				endpoint.URL+"/mcp",
				token,
				"resources/read",
				map[string]any{"uri": data["content_resource_uri"]},
			)
			var content map[string]any
			body := metadata["contents"].([]any)[0].(map[string]any)["text"].(string)
			if err := json.Unmarshal([]byte(body), &content); err != nil {
				t.Fatal(err)
			}
			if content["reason"] != "legacy_projection" {
				t.Fatal("synthetic legacy content misclassified")
			}
		}
	}
	if !seen[domain.ConversationDirect] || !seen[domain.ConversationGroup] {
		t.Fatal("missing conversation type")
	}

	// Act: a payload-less automation recovers the exact accepted envelopes via MCP.
	catalog := httpRPC(t, endpoint.URL+"/mcp", token, "events/list", map[string]any{})
	if len(catalog["events"].([]any)) != 6 {
		t.Fatal("event profiles missing or unexpected in discovery")
	}
	tools := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result := httpRPC(
			t,
			endpoint.URL+"/mcp",
			token,
			"tools/call",
			map[string]any{"name": name, "arguments": args},
		)
		if result["isError"] == true {
			t.Fatalf("recovery failed: %v", result)
		}
		return result["structuredContent"].(map[string]any)
	}
	owned := tools("zalo_list_event_subscriptions", map[string]any{})
	if owned["subscriptions"].([]any)[0].(map[string]any)["subscription_id"] != expectedSubscription {
		t.Fatal("wrong journal owner")
	}
	recovered := tools(
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": expectedSubscription},
	)
	legacyRecords := recovered["events"].([]any)
	if _, present := legacyRecords[0].(map[string]any)["delivery_reason"]; present {
		t.Fatal("legacy diagnostics changed")
	}
	recovered = tools(
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": expectedSubscription, "include_diagnostics": true},
	)
	records := recovered["events"].([]any)
	if len(records) != 2 {
		t.Fatal("callback recovery lost events")
	}
	for _, record := range records {
		entry := record.(map[string]any)
		if entry["delivery_reason"] != "accepted" || entry["delivery_reference"] == nil {
			t.Fatal("safe diagnostics missing")
		}
		payload := entry["event"].(map[string]any)
		if payload["name"] != profileName || payload["eventId"] == nil ||
			entry["delivery_state"] != "delivered" {
			t.Fatal("recovery did not return exact envelope")
		}
		result := tools(
			"zalo_ack_subscription_events",
			map[string]any{"subscription_id": expectedSubscription, "receipt": entry["receipt"]},
		)
		if result["acknowledged"] != true {
			t.Fatal(result)
		}
	}
	if len(
		tools("zalo_read_subscription_events", map[string]any{"subscription_id": expectedSubscription})["events"].([]any),
	) != 0 {
		t.Fatal("acknowledged events replayed")
	}
	// Act: cancel through MCP and insert another record.
	params["delivery"] = map[string]any{"mode": "webhook", "url": "https://receiver.example/events"}
	delete(params, "ttlMs")
	httpRPC(t, endpoint.URL+"/mcp", token, "events/unsubscribe", params)
	if err := store.Put(
		ctx,
		domain.Message{
			Conversation: domain.ConversationRef{Type: domain.ConversationDirect, ID: "another"},
			ID:           "after-cancel",
			SenderID:     "author",
			SentAt:       time.Now(),
			Text:         "synthetic",
			Source:       "live",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FanoutProfileEvents(ctx, time.Now(), worker.Policy, worker.Encoder.EncodeProfile); err != nil {
		t.Fatal(err)
	}
	// Assert: cancellation stops actual callbacks for the broad scope.
	if worked, err := worker.DeliverOne(ctx, time.Now()); err != nil || worked ||
		len(received) != 0 {
		t.Fatalf("cancelled delivery worked=%v error=%v", worked, err)
	}
}
