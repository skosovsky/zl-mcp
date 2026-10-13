package events

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:cyclop,gocognit,gocyclo // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestConversationScopesSignedDeliveryAndCancellation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.sqlite")
	policy := domain.CollectionPolicy{All: true}
	s, err := storage.OpenWithPolicy(ctx, path, policy, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	manager, err := NewSubscriptionManager(s, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("x", 32))
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	manager.Client = &http.Client{Transport: callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		var c map[string]string
		if decodeErr := json.NewDecoder(r.Body).Decode(&c); decodeErr != nil {
			return nil, decodeErr
		}
		body, _ := json.Marshal(map[string]string{"challenge": c["challenge"]})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})}
	raw := func(name string, args map[string]any, sign bool) json.RawMessage {
		delivery := map[string]any{"mode": "webhook", "url": "https://callback.example/events"}
		if sign {
			delivery["secret"] = secret
		}
		b, _ := json.Marshal(map[string]any{"name": name, "arguments": args, "delivery": delivery})
		return b
	}
	scopes := []map[string]any{
		{"scope": "all"},
		{"scope": "direct"},
		{"scope": "group"},
		{"scope": "conversation", "conversation_type": "direct", "conversation_id": "same"},
	}
	ids := map[string]bool{}
	for _, scope := range scopes {
		result, callErr := manager.Call(
			ctx,
			"events/subscribe",
			"owner",
			raw(domain.ConversationMessageCreatedV2, scope, true),
		)
		if callErr != nil {
			t.Fatal(callErr)
		}
		id := result.(map[string]any)["id"].(string)
		if ids[id] {
			t.Fatal("scope identities collided")
		}
		ids[id] = true
	}
	if _, err = manager.Call(
		ctx,
		"events/subscribe",
		"owner",
		raw(MessageCreated, map[string]any{"group_id": "same"}, true),
	); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	text := strings.Repeat("界", 2049)
	for _, ref := range []domain.ConversationRef{{Type: "direct", ID: "same"}, {Type: "group", ID: "same"}, {Type: "direct", ID: "new-peer"}} {
		if err = s.Put(
			ctx,
			domain.Message{
				Conversation: ref,
				ID:           "m",
				SenderID:     "author",
				Text:         text,
				SentAt:       at.Add(-time.Hour),
				Source:       "replay",
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	w, err := NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	// Act: journal ingestion followed by durable fanout, then reopen before delivery.
	if _, err = w.fanout(ctx, at); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = storage.OpenWithPolicy(ctx, path, policy, 90)
	if err != nil {
		t.Fatal(err)
	}
	w, err = NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	received := map[string]int{}
	legacy := 0
	w.Client = &http.Client{Transport: callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		body, readAllErr := io.ReadAll(r.Body)
		if readAllErr != nil {
			return nil, readAllErr
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("Webhook-Id") + "." + r.Header.Get("Webhook-Timestamp") + "."))
		mac.Write(body)
		signature, readAllErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("Webhook-Signature"), "v1,"))
		if readAllErr != nil || !hmac.Equal(signature, mac.Sum(nil)) {
			t.Fatal("bad signature")
		}
		var event struct {
			Name string         `json:"name"`
			Data map[string]any `json:"data"`
		}
		if readAllErr = json.Unmarshal(body, &event); readAllErr != nil {
			t.Fatal(readAllErr)
		}
		if len([]rune(event.Data["text"].(string))) != 2048 || event.Data["text_truncated"] != true {
			t.Fatal("Unicode truncation lost")
		}
		if event.Name == MessageCreated {
			legacy++
			if _, ok := event.Data["conversation_type"]; ok {
				t.Fatal("legacy payload widened")
			}
		} else {
			kind := event.Data["conversation_type"].(string)
			received[kind]++
			if event.Data["schema_version"] != float64(2) ||
				!strings.Contains(event.Data["text_resource_uri"].(string), "/"+kind+"/") {
				t.Fatal("typed payload invalid")
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	for range 10 {
		worked, deliverOneErr := w.DeliverOne(ctx, at.Add(time.Second))
		if deliverOneErr != nil {
			t.Fatal(deliverOneErr)
		}
		if !worked {
			break
		}
	}
	// Assert: direct same matches all/direct/conversation; new matches all/direct; group matches all/group/legacy.
	if received["direct"] != 5 || received["group"] != 2 || legacy != 1 {
		t.Fatalf("scope delivery mismatch: %v legacy=%d", received, legacy)
	}
	// Act / Assert: cancellation uses the same profile/scope identity after restart.
	manager, err = NewSubscriptionManager(s, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Call(
		ctx,
		"events/unsubscribe",
		"owner",
		raw(domain.ConversationMessageCreatedV2, scopes[0], false),
	); err != nil {
		t.Fatal(err)
	}
	var active int
	if err = s.DB.QueryRow("SELECT count(*) FROM event_subscriptions WHERE active=1").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 4 {
		t.Fatal("cancellation changed other scopes")
	}
}

func TestConversationV1IsNotAvailable(t *testing.T) {
	// Arrange: a real manager; old profile must fail before callback verification.
	ctx := context.Background()
	s, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	manager, err := NewSubscriptionManager(s, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	catalog, err := manager.Call(ctx, "events/list", "owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, oldErr := manager.Call(
		ctx,
		"events/subscribe",
		"owner",
		json.RawMessage(
			`{"name":"zalo.conversation.message.created","arguments":{"scope":"all"},"delivery":{"mode":"webhook","url":"https://receiver.example","secret":"invalid"}}`,
		),
	)
	// Assert.
	if oldErr == nil {
		t.Fatal("removed v1 accepted")
	}
	for _, entry := range catalog.(map[string]any)["events"].([]any) {
		if entry.(map[string]any)["name"] == "zalo.conversation.message.created" {
			t.Fatal("removed v1 advertised")
		}
	}
}
