package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestArchiveVisibilityUsesExactTypedIDAndExpiryWithoutWrites(t *testing.T) {
	// Arrange: one direct tombstone and one expired group record under colliding IDs.
	ctx := context.Background()
	now := time.Now().UnixMilli()
	store, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.DB.Exec(`INSERT INTO message_tombstones VALUES('direct','12','1','2026-10-07T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(`INSERT INTO history_message_expiry VALUES('group','12','2',?,0)`, now-1); err != nil {
		t.Fatal(err)
	}
	// Act / Assert: recalls and expiry hide only the exact genuine typed global ID.
	for _, item := range []struct {
		kind, id string
		hidden   bool
	}{{"direct", "1", true}, {"group", "1", false}, {"group", "2", true}, {"direct", "2", false}, {"direct", "", false}} {
		hidden, err := store.ArchiveMessageSuppressed(ctx, domain.ConversationRef{Type: item.kind, ID: "12"}, item.id, now)
		if err != nil || hidden != item.hidden {
			t.Fatal("archive visibility crossed typed identities", err)
		}
	}
	for table, want := range map[string]int{"message_tombstones": 1, "history_message_expiry": 1, "messages": 0, "message_events": 0} {
		var n int
		if err = store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != want {
			t.Fatal("visibility read changed runtime state", err)
		}
	}
}
