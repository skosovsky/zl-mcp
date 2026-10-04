package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func historicalFixture(ref domain.ConversationRef, id string) domain.Message {
	return domain.Message{Conversation: ref, GroupID: ref.ID, ID: id, SenderID: ref.ID, SentAt: time.Now().UTC().AddDate(0, 0, -2), Text: "Synthetic historical text", Direction: "incoming"}
}

func TestHistoryPageSilentAtomicAndReplayAfterRetention(t *testing.T) {
	// Arrange
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	m := historicalFixture(ref, "historical")

	// Act
	counts, err := s.PutHistoryPage(ctx, ref, []domain.Message{m, m})

	// Assert
	if err != nil || counts != (HistoryPageCounts{Inserted: 1, Duplicates: 1}) {
		t.Fatalf("page result=%+v error=%v", counts, err)
	}
	for _, table := range []string{"message_events", "event_deliveries", "collection_started"} {
		var n int
		if err = s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("history created %s records: count=%d error=%v", table, n, err)
		}
	}
	var certainty string
	if err = s.DB.QueryRow("SELECT certainty FROM peer_first_incoming WHERE peer_id=?", ref.ID).Scan(&certainty); err != nil || certainty != "unknown" {
		t.Fatalf("historical peer novelty: certainty=%s error=%v", certainty, err)
	}
	stored, err := s.ConversationMessage(ctx, ref, m.ID)
	if err != nil || stored.Source != "history" || stored.Text != m.Text {
		t.Fatalf("history not readable with provenance: error=%v source=%s", err, stored.Source)
	}
	if err = s.Retain(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Source = "replay"
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM message_events").Scan(&n); err != nil || n != 0 {
		t.Fatalf("replay notified about retained historical identity: count=%d error=%v", n, err)
	}
	m.ID = "later-incoming"
	m.SentAt = time.Now().UTC()
	m.Source = "live"
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	var first any
	if err = s.DB.QueryRow("SELECT first_incoming FROM message_events").Scan(&first); err != nil || first != nil {
		t.Fatalf("historically known peer became new: first=%v error=%v", first, err)
	}
}

func TestHistoryPageRollbackAndExistingLivePreservation(t *testing.T) {
	for _, cause := range []string{"invalid_record", "database_failure"} {
		t.Run(cause, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
			a, b := historicalFixture(ref, "first"), historicalFixture(ref, "second")
			if cause == "invalid_record" {
				b.SenderID = ""
			} else if _, err = s.DB.Exec(`CREATE TRIGGER reject_history BEFORE INSERT ON messages WHEN new.message_id='second' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
				t.Fatal(err)
			}
			// Act
			counts, err := s.PutHistoryPage(ctx, ref, []domain.Message{a, b})
			// Assert
			if err == nil || counts != (HistoryPageCounts{}) {
				t.Fatalf("failed page reported success: counts=%+v error=%v", counts, err)
			}
			for _, table := range []string{"messages", "message_identities", "peer_first_incoming", "conversations"} {
				var n int
				if err = s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
					t.Fatalf("partial %s escaped transaction: count=%d error=%v", table, n, err)
				}
			}
		})
	}
	// Arrange
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	live := historicalFixture(ref, "live")
	live.Source, live.Text = "live", "Original live text"
	if err = s.Put(ctx, live); err != nil {
		t.Fatal(err)
	}
	var originalFirst int64
	if err = s.DB.QueryRow("SELECT first_seq FROM peer_first_incoming").Scan(&originalFirst); err != nil {
		t.Fatal(err)
	}
	conflict := live
	conflict.Source, conflict.Text = "", "Conflicting history text"
	// Act
	counts, err := s.PutHistoryPage(ctx, ref, []domain.Message{conflict, historicalFixture(ref, "old")})
	// Assert
	if err != nil || counts != (HistoryPageCounts{Inserted: 1, Duplicates: 1}) {
		t.Fatalf("merge failed: counts=%+v error=%v", counts, err)
	}
	stored, err := s.ConversationMessage(ctx, ref, live.ID)
	if err != nil || stored.Text != live.Text || stored.Source != "live" {
		t.Fatal("history overwrote existing live record", err)
	}
	var first int64
	if err = s.DB.QueryRow("SELECT first_seq FROM peer_first_incoming").Scan(&first); err != nil || first != originalFirst {
		t.Fatal("history changed known first incoming", err)
	}
	var n int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM message_events").Scan(&n); err != nil || n != 1 {
		t.Fatalf("ordinary event changed: count=%d error=%v", n, err)
	}
}
