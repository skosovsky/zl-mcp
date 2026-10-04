package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestUnenrichedProfileScanAdvancesAcrossExcludedPages(t *testing.T) {
	// Arrange
	ctx := context.Background()
	ref := domain.ConversationRef{Type: "direct", ID: "peer-150"}
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{ref: true}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 201; i++ {
		if _, err = s.DB.ExecContext(ctx, `INSERT INTO conversations VALUES('direct',?,NULL,'stored_message','observed',?,?)`, fmt.Sprintf("peer-%03d", i), now(), now()); err != nil {
			t.Fatal(err)
		}
	}
	// Act
	first, cursor, more, err := s.UnenrichedDirectIDs(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	second, next, again, err := s.UnenrichedDirectIDs(ctx, cursor, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	if len(first) != 0 || cursor != "peer-099" || !more || len(second) != 1 || second[0] != ref.ID || next != "peer-199" || !again {
		t.Fatal("excluded-page continuation lost")
	}
	// Act / Assert: a successfully cached profile is not re-requested.
	if _, err = s.PutContacts(ctx, []domain.Contact{{ID: ref.ID, Name: "Synthetic", Friendship: "unknown"}}); err != nil {
		t.Fatal(err)
	}
	cached, _, _, err := s.UnenrichedDirectIDs(ctx, cursor, 100)
	if err != nil || len(cached) != 0 {
		t.Fatal("cached profile re-requested", err)
	}
	// Act / Assert: old metadata becomes eligible again without deleting identity.
	if _, err = s.DB.ExecContext(ctx, "UPDATE directory_contacts SET updated_at='2000-01-01T00:00:00Z' WHERE peer_id=?", ref.ID); err != nil {
		t.Fatal(err)
	}
	stale, _, _, err := s.UnenrichedDirectIDs(ctx, cursor, 100)
	if err != nil || len(stale) != 1 || stale[0] != ref.ID {
		t.Fatal("stale metadata was never refreshed", err)
	}
}
