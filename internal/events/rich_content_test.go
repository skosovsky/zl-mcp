package events

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:cyclop,gocognit,gocyclo,govet // Keep this AAA cross-layer scenario together; short error scopes do not alter state ownership.
func TestRichContentSurvivesStorageRestartFanoutAndJournal(t *testing.T) {
	// Arrange: a genuine empty text with a structured image, two profile consumers.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s, err := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	native, reason := "chat.photo", "metadata_limited"
	content := &domain.MessageContent{
		TextRole:     "none",
		Kind:         "image",
		NativeType:   &native,
		Summary:      "Image message; media bytes unavailable",
		Availability: "metadata_only",
		Reason:       &reason,
		Elements:     []domain.ContentElement{},
	}
	for _, profile := range []string{domain.ConversationMessageCreatedV2, domain.ConversationMessageCreatedV3} {
		sub := storage.EventSubscription{
			ID:               profile,
			Principal:        "owner",
			Profile:          profile,
			Scope:            "direct",
			ConversationType: "direct",
			Callback:         "https://receiver.example",
			Secret:           "synthetic",
		}
		revision, err := s.SubscriptionRevision(ctx, sub.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ActivateSubscription(ctx, sub, revision, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Put(ctx, domain.Message{
		Conversation: ref, ID: "image", SenderID: "peer", SentAt: at, Text: "", Content: content,
		Direction: "incoming", Source: "live",
	}); err != nil {
		t.Fatal(err)
	}
	// Act: close/reopen before reading and fanout.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	message, err := s.ConversationMessage(ctx, ref, "image")
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.FanoutProfileEvents(ctx, at, storage.DefaultDeliveryPolicy(), encoder.EncodeProfile); err != nil ||
		n != 1 {
		t.Fatal(n, err)
	}
	// Assert: persistent rich projection; exact same source ID, unchanged old schema.
	if message.Content == nil || message.Content.Kind != "image" || message.Text != "" {
		t.Fatal("stored content lost")
	}
	var sourceID string
	for range 2 {
		d, err := s.ClaimDelivery(ctx, at, storage.DefaultDeliveryPolicy())
		if err != nil || d == nil {
			t.Fatal(d, err)
		}
		var wire map[string]any
		if err := json.Unmarshal(d.Payload, &wire); err != nil {
			t.Fatal(err)
		}
		profile := wire["name"].(string)
		schemaName := "conversation_v2_event"
		if profile == domain.ConversationMessageCreatedV3 {
			schemaName = "conversation_v3_event"
		}
		schema, err := contracts.Compile(schemaName, "delivery")
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(wire); err != nil {
			t.Fatal(err)
		}
		if sourceID != "" && sourceID != wire["eventId"] {
			t.Fatal("profile changed source identity")
		}
		sourceID = wire["eventId"].(string)
		data := wire["data"].(map[string]any)
		if (data["content"] != nil) != (profile == domain.ConversationMessageCreatedV3) {
			t.Fatal("profile compatibility lost")
		}
		outcome := storage.DeliveryOutcome{State: "delivered", Reason: "accepted"}
		if err := s.FinishDelivery(ctx, *d, outcome, at); err != nil {
			t.Fatal(err)
		}
		journal, err := s.ReadSubscriptionEvents(ctx, "owner", profile, 20)
		if err != nil {
			t.Fatal(err)
		}
		journalSchema, err := contracts.Compile("zalo_read_subscription_events", "output")
		if err != nil {
			t.Fatal(err)
		}
		journalBytes, _ := json.Marshal(journal)
		var journalValue any
		json.Unmarshal(journalBytes, &journalValue)
		if err := journalSchema.Validate(journalValue); err != nil {
			t.Fatal(err)
		}
		entry := journal["events"].([]map[string]any)[0]
		recovered, err := json.Marshal(entry["event"])
		if err != nil {
			t.Fatal(err)
		}
		// JSON object ordering differs, but the durable event must be semantically exact.
		var expected, actual any
		json.Unmarshal(d.Payload, &expected)
		json.Unmarshal(recovered, &actual)
		left, _ := json.Marshal(expected)
		right, _ := json.Marshal(actual)
		if !bytes.Equal(left, right) {
			t.Fatal("journal changed callback")
		}
		if _, err := s.AckSubscriptionEvents(ctx, "owner", profile, entry["receipt"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Browse(ctx, ref, "", "", "asc", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if page["messages"].([]map[string]any)[0]["content"] == nil {
		t.Fatal("browse lost content")
	}
}
