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

func TestFilteredProfileCanonicalIdentitySignedPayloadAndCancel(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "state.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err := NewSubscriptionManager(s, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("x", 32))
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	m.Client = &http.Client{Transport: callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		var c map[string]string
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			return nil, err
		}
		b, _ := json.Marshal(c)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}
	raw := func(args map[string]any, sign bool) json.RawMessage {
		d := map[string]any{"mode": "webhook", "url": "https://receiver.example/events"}
		if sign {
			d["secret"] = secret
		}
		b, _ := json.Marshal(map[string]any{"name": domain.ConversationMessageCreatedV2, "arguments": args, "delivery": d})
		return b
	}
	defaults, err := m.Call(ctx, "events/subscribe", "owner", raw(map[string]any{"scope": "direct"}, true))
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := m.Call(ctx, "events/subscribe", "owner", raw(map[string]any{"scope": "direct", "direction": "all", "first_incoming_only": false}, true))
	if err != nil {
		t.Fatal(err)
	}
	if defaults.(map[string]any)["id"] != explicit.(map[string]any)["id"] {
		t.Fatal("defaults changed canonical subscription identity")
	}
	firstArgs := map[string]any{"scope": "direct", "direction": "incoming", "first_incoming_only": true}
	first, err := m.Call(ctx, "events/subscribe", "owner", raw(firstArgs, true))
	if err != nil {
		t.Fatal(err)
	}
	if first.(map[string]any)["id"] == defaults.(map[string]any)["id"] {
		t.Fatal("filter excluded from identity")
	}
	put := func(id, direction string) {
		t.Helper()
		if err := s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: id, SenderID: "peer", Direction: direction, SentAt: time.Now().Add(-time.Hour), Text: "synthetic", Source: "replay"}); err != nil {
			t.Fatal(err)
		}
	}
	w, err := NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	received, firstCount := 0, 0
	w.Client = &http.Client{Transport: callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
		mac.Write(body)
		sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
		if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
			t.Fatal("invalid independent signature")
		}
		var v map[string]any
		if err = json.Unmarshal(body, &v); err != nil {
			t.Fatal(err)
		}
		d := v["data"].(map[string]any)
		if v["name"] != domain.ConversationMessageCreatedV2 || d["schema_version"] != float64(2) {
			t.Fatal("wrong profile version")
		}
		if d["first_incoming"] == true {
			if d["direction"] != "incoming" {
				t.Fatal("first applied to outgoing")
			}
			firstCount++
		}
		received++
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	// Act.
	put("own", "outgoing")
	put("first", "incoming")
	put("next", "incoming")
	if _, err = s.FanoutProfileEvents(ctx, time.Now(), w.Policy, w.Encoder.EncodeProfile); err != nil {
		t.Fatal(err)
	}
	for {
		did, err := w.DeliverOne(ctx, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if !did {
			break
		}
	}
	if _, err = m.Call(ctx, "events/unsubscribe", "owner", raw(firstArgs, false)); err != nil {
		t.Fatal(err)
	}
	// Assert.
	if received != 4 || firstCount != 2 {
		t.Fatalf("received=%d first=%d", received, firstCount)
	}
	var active int
	if err = s.DB.QueryRow("SELECT active FROM event_subscriptions WHERE id=?", first.(map[string]any)["id"]).Scan(&active); err != nil || active != 0 {
		t.Fatal("filtered cancellation failed")
	}
}
