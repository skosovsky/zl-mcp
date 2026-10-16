package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestIngestionCountersSeparateCommitDuplicatesPolicyAndFailure(t *testing.T) {
	// Arrange: only the direct namespace is allowed.
	ctx := context.Background()
	ref := domain.ConversationRef{Type: "direct", ID: "same"}
	s, err := OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "messages.sqlite"),
		domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{ref: true}},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := domain.Message{
		Conversation: ref,
		ID:           "m",
		SenderID:     "peer",
		SentAt:       time.Now(),
		Text:         "synthetic",
		Source:       "live",
	}
	// Act: commit, replay duplicate, policy exclusion and transaction failure.
	for range 2 {
		if err := s.Put(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	m.Conversation.Type = "group"
	if err := s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(
		ctx,
		`CREATE TRIGGER reject_ingestion BEFORE INSERT ON messages BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`,
	); err != nil {
		t.Fatal(err)
	}
	m.Conversation.Type, m.ID = "direct", "failed"
	if err := s.Put(ctx, m); err == nil {
		t.Fatal("synthetic failure was ignored")
	}
	// Assert: failure/duplicate do not inflate the committed insertion counter.
	counts := s.IngestionDiagnostics()
	if counts["direct"] != (IngestionCount{Received: 3, Inserted: 1, Duplicates: 1, Errors: 1}) ||
		counts["group"] != (IngestionCount{Received: 1, Excluded: 1}) {
		t.Fatalf("incorrect counts: %+v", counts)
	}
}

func TestUnsupportedContentCounterCountsAdmittedAttemptsOnly(t *testing.T) {
	// Arrange: unknown projection, a legacy projection and an excluded namespace.
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	s, err := OpenWithPolicy(
		t.Context(),
		filepath.Join(t.TempDir(), "content-counts.sqlite"),
		domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{ref: true}},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := domain.Message{
		Conversation: ref,
		ID:           "unknown",
		SenderID:     "peer",
		SentAt:       time.Now().UTC(),
		Source:       "live",
		Content: &domain.MessageContent{
			Kind:         "unsupported",
			TextRole:     "none",
			Summary:      "Unsupported message content",
			Availability: "unavailable",
			Reason:       new("unknown_native_type"),
			Elements:     []domain.ContentElement{},
		},
	}
	// Act: a replay duplicate is still an observed normalization attempt.
	for range 2 {
		if err = s.Put(t.Context(), m); err != nil {
			t.Fatal(err)
		}
	}
	unsupported := m.Content
	m.ID = "legacy"
	m.Content = domain.LegacyContent()
	if err = s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	m.ID = "text"
	m.Text = "hello"
	m.Content = &domain.MessageContent{
		Kind:         "text",
		TextRole:     "author",
		Summary:      "Text message",
		Availability: "text",
		Elements:     []domain.ContentElement{},
	}
	if err = s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	m.Content = unsupported
	m.Conversation.Type = "group"
	if err = s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(
		t.Context(),
		`CREATE TRIGGER reject_content BEFORE INSERT ON messages BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`,
	); err != nil {
		t.Fatal(err)
	}
	m.Conversation = ref
	m.ID = "failed"
	if err = s.Put(t.Context(), m); err == nil {
		t.Fatal("synthetic failure ignored")
	}
	// Assert: no content, IDs or arbitrary reason labels enter the aggregate.
	counts := s.IngestionDiagnostics()["direct"]
	if counts.UnsupportedContent != 3 || counts.Inserted != 3 || counts.Duplicates != 1 || counts.Errors != 1 ||
		s.IngestionDiagnostics()["group"].UnsupportedContent != 0 {
		t.Fatal("unsupported attempts counted as unique records", counts)
	}
}
