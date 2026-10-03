package events

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			return nil, err
		}
		body, _ := json.Marshal(map[string]string{"challenge": c["challenge"]})
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	raw := func(name string, args map[string]any, sign bool) json.RawMessage {
		delivery := map[string]any{"mode": "webhook", "url": "https://callback.example/events"}
		if sign {
			delivery["secret"] = secret
		}
		b, _ := json.Marshal(map[string]any{"name": name, "arguments": args, "delivery": delivery})
		return b
	}
	scopes := []map[string]any{{"scope": "all"}, {"scope": "direct"}, {"scope": "group"}, {"scope": "conversation", "conversation_type": "direct", "conversation_id": "same"}}
	ids := map[string]bool{}
	for _, scope := range scopes {
		result, err := manager.Call(ctx, "events/subscribe", "owner", raw(ConversationMessageCreated, scope, true))
		if err != nil {
			t.Fatal(err)
		}
		id := result.(map[string]any)["id"].(string)
		if ids[id] {
			t.Fatal("scope identities collided")
		}
		ids[id] = true
	}
	if _, err = manager.Call(ctx, "events/subscribe", "owner", raw(MessageCreated, map[string]any{"group_id": "same"}, true)); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	text := strings.Repeat("界", 2049)
	for _, ref := range []domain.ConversationRef{{Type: "direct", ID: "same"}, {Type: "group", ID: "same"}, {Type: "direct", ID: "new-peer"}} {
		if err = s.Put(ctx, domain.Message{Conversation: ref, ID: "m", SenderID: "author", Text: text, SentAt: at.Add(-time.Hour), Source: "replay"}); err != nil {
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
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
		mac.Write(body)
		signature, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
		if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
			t.Fatal("bad signature")
		}
		var event struct {
			Name string         `json:"name"`
			Data map[string]any `json:"data"`
		}
		if err = json.Unmarshal(body, &event); err != nil {
			t.Fatal(err)
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
			if event.Data["schema_version"] != float64(1) || !strings.Contains(event.Data["text_resource_uri"].(string), "/"+kind+"/") {
				t.Fatal("typed payload invalid")
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	for i := 0; i < 10; i++ {
		worked, err := w.DeliverOne(ctx, at.Add(time.Second))
		if err != nil {
			t.Fatal(err)
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
	if _, err = manager.Call(ctx, "events/unsubscribe", "owner", raw(ConversationMessageCreated, scopes[0], false)); err != nil {
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
