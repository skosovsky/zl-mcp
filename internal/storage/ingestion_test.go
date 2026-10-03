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
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{ref: true}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := domain.Message{Conversation: ref, ID: "m", SenderID: "peer", SentAt: time.Now(), Text: "synthetic", Source: "live"}
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
	if _, err := s.DB.ExecContext(ctx, `CREATE TRIGGER reject_ingestion BEFORE INSERT ON messages BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	m.Conversation.Type, m.ID = "direct", "failed"
	if err := s.Put(ctx, m); err == nil {
		t.Fatal("synthetic failure was ignored")
	}
	// Assert: failure/duplicate do not inflate the committed insertion counter.
	counts := s.IngestionDiagnostics()
	if counts["direct"] != (IngestionCount{Received: 3, Inserted: 1, Duplicates: 1, Errors: 1}) || counts["group"] != (IngestionCount{Received: 1, Excluded: 1}) {
		t.Fatalf("incorrect counts: %+v", counts)
	}
}
