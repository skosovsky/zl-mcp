package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestIncomingMigrationSeedsEvidenceAndPreservesLegacyQueue(t *testing.T) {
	// Arrange: reconstruct the previous schema with retained and deleted peers.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	sub := EventSubscription{ID: "legacy", Principal: "owner", Callback: "https://receiver.example/events", Secret: "synthetic", Profile: domain.ConversationMessageCreatedV2, Scope: "all"}
	r, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	sub, err = s.ActivateSubscription(ctx, sub, r, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	put := func(peer, id, author, dir string) {
		t.Helper()
		if err := s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: peer}, ID: id, SenderID: author, Direction: dir, SentAt: time.Now(), Text: "synthetic", Source: "live"}); err != nil {
			t.Fatal(err)
		}
	}
	put("known", "old", "known", "incoming")
	put("outgoing-only", "old", "owner", "outgoing")
	put("deleted", "old", "deleted", "incoming")
	if _, err = s.FanoutProfileEvents(ctx, time.Now(), DefaultDeliveryPolicy(), func(_, _ string, _ domain.Message) ([]byte, error) { return []byte("original queued bytes"), nil }); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteConversation(ctx, domain.ConversationRef{Type: "direct", ID: "deleted"}, "old"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DROP TABLE peer_first_incoming;
ALTER TABLE message_events DROP COLUMN direction;
ALTER TABLE message_events DROP COLUMN first_incoming;
ALTER TABLE event_subscriptions DROP COLUMN direction;
ALTER TABLE event_subscriptions DROP COLUMN first_incoming_only;
DELETE FROM schema_migrations WHERE version=6;`); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// Act.
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, peer := range []string{"known", "outgoing-only", "deleted", "brand-new"} {
		if err = s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: peer}, ID: "next", SenderID: peer, Direction: "incoming", SentAt: time.Now(), Text: "new synthetic", Source: "live"}); err != nil {
			t.Fatal(err)
		}
	}
	// Assert.
	for peer, want := range map[string]any{"known": int64(0), "outgoing-only": nil, "deleted": nil, "brand-new": int64(1)} {
		var got any
		if err = s.DB.QueryRow("SELECT first_incoming FROM message_events WHERE group_id=? AND seq=(SELECT max(seq) FROM message_events WHERE group_id=?)", peer, peer).Scan(&got); err != nil || got != want {
			t.Fatalf("peer=%s first=%v want=%v err=%v", peer, got, want, err)
		}
	}
	var generation, direction string
	var start int64
	if err = s.DB.QueryRow("SELECT generation,start_seq,direction FROM event_subscriptions WHERE id='legacy'").Scan(&generation, &start, &direction); err != nil || generation != sub.Generation || start != sub.StartSeq || direction != "all" {
		t.Fatal("legacy subscription changed")
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM event_deliveries WHERE state='pending' AND payload=?", []byte("original queued bytes")).Scan(&count); err != nil || count != 2 {
		t.Fatal("pending queued bytes changed", count, err)
	}
	if err = s.migrateIncoming(ctx); err != nil {
		t.Fatal("migration is not repeatable", err)
	}
}
