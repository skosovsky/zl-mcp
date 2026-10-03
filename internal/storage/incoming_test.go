package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestConcurrentIncomingInsertsAndRetentionNeverResetFirst(t *testing.T) {
	// Arrange: concurrent live/replay writes with duplicate IDs and old timestamps.
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "state.sqlite"), domain.CollectionPolicy{All: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Now().UTC().Add(-48 * time.Hour)
	var wg sync.WaitGroup
	// Act.
	for n := range 40 {
		wg.Go(func() {
			m := domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: fmt.Sprint(n % 10), SenderID: "peer", Direction: "incoming", SentAt: at.Add(time.Duration(n%10) * time.Minute), Text: "synthetic", Source: "replay"}
			if err := s.Put(ctx, m); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	// Assert: exactly one first fact, based on insertion seq rather than sent_at.
	var count, first, earliest int64
	if err = s.DB.QueryRow("SELECT count(*) FROM message_events WHERE first_incoming=1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT first_seq FROM peer_first_incoming WHERE peer_id='peer'").Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT min(seq) FROM message_events WHERE group_id='peer'").Scan(&earliest); err != nil {
		t.Fatal(err)
	}
	if count != 1 || first != earliest {
		t.Fatal("concurrent replay produced multiple first facts or selected by sent_at")
	}
	// Act: expire all bodies, then receive another incoming from the same peer.
	if err = s.Retain(ctx); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err = s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&retained); err != nil || retained != 0 {
		t.Fatal("old corpus was not expired", err)
	}
	if err = s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: "after-retention", SenderID: "peer", Direction: "incoming", SentAt: time.Now(), Text: "new synthetic", Source: "live"}); err != nil {
		t.Fatal(err)
	}
	// Assert: deleting text does not make this peer new again.
	var got int
	if err = s.DB.QueryRow("SELECT first_incoming FROM message_events WHERE seq=(SELECT seq FROM messages WHERE message_id='after-retention')").Scan(&got); err != nil || got != 0 {
		t.Fatal("retention reset first incoming", err)
	}
}

func TestIncomingFiltersPreserveFirstAcrossReplayDeletionRestart(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	for _, sub := range []EventSubscription{
		{ID: "first", Profile: domain.ConversationMessageCreatedV2, Scope: "direct", ConversationType: "direct", Direction: "incoming", FirstIncomingOnly: true},
		{ID: "incoming", Profile: domain.ConversationMessageCreatedV2, Scope: "direct", ConversationType: "direct", Direction: "incoming"},
		{ID: "outgoing", Profile: domain.ConversationMessageCreatedV2, Scope: "direct", ConversationType: "direct", Direction: "outgoing"},
	} {
		sub.Principal = "owner"
		sub.Callback = "https://receiver.example/events"
		sub.Secret = "synthetic"
		r, err := s.SubscriptionRevision(ctx, sub.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.ActivateSubscription(ctx, sub, r, at); err != nil {
			t.Fatal(err)
		}
	}
	put := func(store *Store, id, direction string) {
		t.Helper()
		if err := store.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: id, SenderID: "peer", Direction: direction, SentAt: at.Add(-time.Hour), Text: "synthetic", Source: "replay"}); err != nil {
			t.Fatal(err)
		}
	}
	// Act.
	put(s, "own", "outgoing")
	put(s, "first-message", "incoming")
	put(s, "first-message", "incoming")
	put(s, "second-message", "incoming")
	if _, err = s.FanoutProfileEvents(ctx, at, DefaultDeliveryPolicy(), func(_, _ string, m domain.Message) ([]byte, error) { return []byte("synthetic"), nil }); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteConversation(ctx, domain.ConversationRef{Type: "direct", ID: "peer"}, "first-message"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put(s, "first-message", "incoming")
	put(s, "third-message", "incoming")
	if _, err = s.FanoutProfileEvents(ctx, at, DefaultDeliveryPolicy(), func(_, _ string, m domain.Message) ([]byte, error) { return []byte("synthetic"), nil }); err != nil {
		t.Fatal(err)
	}
	// Assert.
	for id, want := range map[string]int{"first": 1, "incoming": 3, "outgoing": 1} {
		var got int
		if err = s.DB.QueryRow("SELECT count(*) FROM event_deliveries WHERE subscription_id=?", id).Scan(&got); err != nil || got != want {
			t.Fatalf("subscription=%s got=%d want=%d err=%v", id, got, want, err)
		}
	}
	var first int
	if err = s.DB.QueryRow("SELECT count(*) FROM message_events WHERE first_incoming=1").Scan(&first); err != nil || first != 0 {
		t.Fatal("deleted identity emitted another first")
	}
}

func TestUnknownDirectionNeverAssertsFirstIncoming(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "state.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: "unknown", SenderID: "peer", SentAt: time.Now(), Text: "synthetic", Source: "live"}
	// Act.
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	m.ID = "incoming"
	m.Direction = "incoming"
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	// Assert.
	var asserted int
	if err = s.DB.QueryRow("SELECT count(*) FROM message_events WHERE first_incoming IS NOT NULL").Scan(&asserted); err != nil || asserted != 0 {
		t.Fatal("unknown evidence became first incoming")
	}
}
