package events

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:gocognit // Keep the maximum-valid-content AAA recovery scenario and its source-preservation assertions together.
func TestMaximumV3ContentRemainsRecoverableWithoutMutatingSource(t *testing.T) {
	// Arrange: every string/array at its schema bound, with maximal JSON escaping.
	ctx := t.Context()
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
	at := time.Now().UTC()
	escaped := strings.Repeat("\x00", 256)
	content := maximalProjectionContent()
	original, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	sub := storage.EventSubscription{
		ID:        "maximum-v3",
		Profile:   domain.ConversationMessageCreatedV3,
		Principal: "owner",
		Scope:     "all",
		Callback:  "https://receiver.example",
		Secret:    "synthetic",
	}
	revision, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateSubscription(ctx, sub, revision, at); err != nil {
		t.Fatal(err)
	}
	message := domain.Message{
		Conversation:     domain.ConversationRef{Type: "direct", ID: escaped},
		ConversationName: &escaped, ID: escaped, SenderID: escaped, SenderName: &escaped,
		SentAt: at, Text: strings.Repeat("\x00", 2049), Content: content, Direction: "incoming", Source: "live",
	}
	if err = s.Put(ctx, message); err != nil {
		t.Fatal(err)
	}
	encoder, err := NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	// Act: encode the maximum event, persist its delivery and read the journal.
	if _, err = s.FanoutProfileEvents(ctx, at, storage.DefaultDeliveryPolicy(), encoder.EncodeProfile); err != nil {
		t.Fatal(err)
	}
	delivery, err := s.ClaimDelivery(ctx, at, storage.DefaultDeliveryPolicy())
	if err != nil || delivery == nil {
		t.Fatal("maximum content delivery unavailable", err)
	}
	var event eventaccess.Event
	if err = json.Unmarshal(delivery.Payload, &event); err != nil {
		t.Fatal(err)
	}
	// Assert: explicit projection, bounded body, full metadata still retained.
	if len(delivery.Payload) > maximumRecoveryEnvelopeBytes || event.Data.ContentTruncated == nil ||
		!*event.Data.ContentTruncated || !event.Data.TextTruncated || event.Data.ContentResourceURI == "" ||
		len(event.Data.Content.Elements) >= 16 {
		t.Fatal("maximum event did not declare its bounded projection")
	}
	if err = s.FinishDelivery(
		ctx,
		*delivery,
		storage.DeliveryOutcome{State: "delivered", Reason: "accepted"},
		at,
	); err != nil {
		t.Fatal(err)
	}
	page, err := s.ReadSubscriptionEvents(ctx, "owner", sub.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	entries := page["events"].([]map[string]any)
	if len(entries) != 1 || entries[0]["event"] == nil {
		t.Fatal("maximum event cannot be recovered")
	}
	recovered, err := json.Marshal(entries[0]["event"])
	if err != nil {
		t.Fatal(err)
	}
	var callbackValue, journalValue any
	if json.Unmarshal(delivery.Payload, &callbackValue) != nil || json.Unmarshal(recovered, &journalValue) != nil {
		t.Fatal("invalid callback/journal")
	}
	callbackJSON, _ := json.Marshal(callbackValue)
	journalJSON, _ := json.Marshal(journalValue)
	if !bytes.Equal(callbackJSON, journalJSON) {
		t.Fatal("journal differs from callback")
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", sub.ID, entries[0]["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ConversationMessage(ctx, message.Ref(), message.ID)
	if err != nil {
		t.Fatal(err)
	}
	retained, _ := json.Marshal(stored.Content)
	unmutated, _ := json.Marshal(content)
	if !bytes.Equal(original, retained) || !bytes.Equal(original, unmutated) {
		t.Fatal("projection mutated full metadata")
	}
}

func maximalProjectionContent() *domain.MessageContent {
	content := &domain.MessageContent{
		TextRole: "author", Kind: "mixed", NativeType: new(strings.Repeat("a", 64)),
		Summary: strings.Repeat("\x00", 256), Availability: "metadata_only", Reason: new("metadata_limited"),
		Elements: []domain.ContentElement{},
	}
	for range 16 {
		content.Elements = append(content.Elements, domain.ContentElement{
			Kind: "image", Label: new(strings.Repeat("\x00", 512)), MIMEType: new(strings.Repeat("\x00", 128)),
			FileName: new(strings.Repeat("\x00", 256)), StickerID: new(strings.Repeat("\x00", 128)),
			SizeBytes: new(int64(9007199254740991)), DurationMS: new(int64(9007199254740991)),
			Width: new(int64(100000)), Height: new(int64(100000)), BytesAvailable: false,
		})
	}
	return content
}

func TestV3ProjectionValidatesBeforeTrimming(t *testing.T) {
	// Arrange: legal maximum identities/text and content, including escaped IDs.
	encoder, err := NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	escaped := strings.Repeat("\x00", 256)
	m := domain.Message{
		Conversation:     domain.ConversationRef{Type: "direct", ID: escaped},
		ConversationName: &escaped, ID: escaped, SenderID: escaped, SenderName: &escaped,
		SentAt: time.Now().UTC(), Text: strings.Repeat("\x00", 2049),
		Content: maximalProjectionContent(), Direction: "incoming",
	}
	// Act / Assert: even the schema's maximum escaped event ID fits recovery.
	wire, err := encoder.EncodeProfile(domain.ConversationMessageCreatedV3, strings.Repeat("\x00", 128), m)
	if err != nil || len(wire) > maximumRecoveryEnvelopeBytes {
		t.Fatal("maximum valid envelope rejected", err)
	}
	// Act / Assert: trimming must not launder an invalid seventeenth element.
	m.Content.Elements = append(m.Content.Elements, m.Content.Elements[0])
	if _, err = encoder.EncodeProfile(domain.ConversationMessageCreatedV3, "event", m); err == nil {
		t.Fatal("projection accepted schema-invalid source content")
	}
}
