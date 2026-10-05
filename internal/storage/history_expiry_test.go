package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestHistoricalExpiryIsSilentDurableAndCannotReplay(t *testing.T) {
	// Arrange: an already expired historical message, no age-retention policy.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.sqlite")
	s, e := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { s.Close() }()
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	message := historicalFixture(ref, "expired")
	message.SentAt = time.Now().Add(-time.Hour).UTC()
	deadline := time.Now().Add(-time.Minute).UnixMilli()
	// Act: expiry is recorded and content removed before the page transaction commits.
	counts, e := s.PutExpiringHistoryPage(ctx, ref, []ExpiringHistoryRecord{{Message: message, ExpiresAtMS: deadline}})
	if e != nil || counts.Inserted != 1 {
		t.Fatal("history expiry insertion", e)
	}
	// Assert: no exposed payload or Event; permanent identity and deadline survive.
	assertCounts := func() {
		t.Helper()
		for table, want := range map[string]int{"messages": 0, "message_events": 0, "event_deliveries": 0, "message_identities": 1, "history_message_expiry": 1} {
			var n int
			if e := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != want {
				t.Fatal("expiry invariant", table, n, e)
			}
		}
	}
	assertCounts()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	// Act: ordinary replay and silent reimport cannot revive expired content.
	message.Source = "replay"
	if e = s.Put(ctx, message); e != nil {
		t.Fatal(e)
	}
	message.Source = ""
	counts, e = s.PutHistoryPage(ctx, ref, []domain.Message{message})
	if e != nil || counts.Duplicates != 1 {
		t.Fatal("expired history replay", e)
	}
	assertCounts()
}
func TestHistoricalExpiryPreservesDuplicateAndRollsBackInvalidPage(t *testing.T) {
	// Arrange: original live content must not receive expiry from an import replay.
	ctx := context.Background()
	s, e := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	m := historicalFixture(ref, "live")
	m.SentAt = time.Now().Add(-time.Hour).UTC()
	m.Source = "live"
	if e = s.Put(ctx, m); e != nil {
		t.Fatal(e)
	}
	m.Source = ""
	// Act.
	counts, e := s.PutExpiringHistoryPage(ctx, ref, []ExpiringHistoryRecord{{Message: m, ExpiresAtMS: time.Now().Add(-time.Minute).UnixMilli()}})
	// Assert: original remains and its live Event is not removed by new expiry.
	if e != nil || counts.Duplicates != 1 {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow("SELECT count(*) FROM history_message_expiry").Scan(&n); e != nil || n != 0 {
		t.Fatal("live record deadline changed")
	}
	bad := m
	bad.ID = "invalid"
	good := m
	good.ID = "good"
	_, e = s.PutExpiringHistoryPage(ctx, ref, []ExpiringHistoryRecord{{Message: good, ExpiresAtMS: time.Now().Add(time.Hour).UnixMilli()}, {Message: bad, ExpiresAtMS: bad.SentAt.UnixMilli()}})
	if e == nil {
		t.Fatal("invalid deadline accepted")
	}
	if e = s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&n); e != nil || n != 1 {
		t.Fatal("partial page escaped")
	}
}

func TestHistoricalExpiryIsInvisibleBeforePhysicalCleanup(t *testing.T) {
	// Arrange: simulate a passed deadline without running maintenance or sleeping.
	ctx := context.Background()
	s, e := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	m := historicalFixture(ref, "hidden")
	m.SentAt = time.Now().Add(-time.Hour).UTC()
	m.Text = "syntheticexpiryword"
	m.QuoteMetadata = &domain.QuoteMetadata{ClientMessageID: "synthetic-client", MessageType: "webchat", Timestamp: "1"}
	_, e = s.PutExpiringHistoryPage(ctx, ref, []ExpiringHistoryRecord{{Message: m, ExpiresAtMS: time.Now().Add(time.Hour).UnixMilli()}})
	if e != nil {
		t.Fatal(e)
	}
	query := Search{General: true, ConversationType: "direct", ConversationID: ref.ID, Query: "syntheticexpiryword", Limit: 50}
	if hits, _, _, e := s.Search(ctx, query); e != nil || len(hits) != 1 {
		t.Fatal("test lacks visible searchable message", e)
	}
	if _, e := s.SendQuote(ctx, ref.ID, m.ID); e != nil {
		t.Fatal("test lacks quotable message", e)
	}
	if _, e = s.DB.Exec("UPDATE history_message_expiry SET expires_ms=?", time.Now().Add(-time.Minute).UnixMilli()); e != nil {
		t.Fatal(e)
	}
	var raw int
	if e = s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&raw); e != nil || raw != 1 {
		t.Fatal("test lacks physically retained message")
	}
	// Act / Assert: every content entry point rejects the expired row before cleanup.
	if _, e = s.ConversationMessage(ctx, ref, m.ID); e == nil {
		t.Fatal("expired resource exposed")
	}
	if _, e = s.SendQuote(ctx, ref.ID, m.ID); e == nil {
		t.Fatal("expired reply content exposed")
	}
	if _, e = s.context(ctx, ref, m.ID, 1, 1, true); e == nil {
		t.Fatal("expired context exposed")
	}
	page, e := s.Browse(ctx, ref, time.Now().Add(-2*time.Hour).UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), "asc", 50, "")
	if e != nil || len(page["messages"].([]map[string]any)) != 0 {
		t.Fatal("expired browse exposed", e)
	}
	hits, _, _, e := s.Search(ctx, query)
	if e != nil || len(hits) != 0 {
		t.Fatal("expired search exposed", e)
	}
	counts, e := s.conversationMessageCounts(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, count := range counts {
		if count != 0 {
			t.Fatal("expired corpus count exposed")
		}
	}
	// Act: retention off still physically removes expired payload and persists a tombstone.
	if e = s.Retain(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&raw); e != nil || raw != 0 {
		t.Fatal("disabled retention skipped expiry")
	}
	// Assert: moving the deadline back to the future cannot reverse observed expiry.
	if _, e = s.DB.Exec("UPDATE history_message_expiry SET expires_ms=?", time.Now().Add(time.Hour).UnixMilli()); e != nil {
		t.Fatal(e)
	}
	m.Source = "replay"
	if e = s.Put(ctx, m); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&raw); e != nil || raw != 0 {
		t.Fatal("expired tombstone resurrected")
	}
}

func TestHistoricalExpiryPageRollsBackOnDeadlineStorageFailure(t *testing.T) {
	// Arrange: fail after message/identity/FTS insertion but before expiry commits.
	ctx := context.Background()
	s, e := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.DB.Exec(`CREATE TRIGGER reject_deadline BEFORE INSERT ON history_message_expiry BEGIN SELECT RAISE(ABORT,'synthetic failure');END`); e != nil {
		t.Fatal(e)
	}
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	m := historicalFixture(ref, "rollback")
	m.SentAt = time.Now().Add(-time.Hour).UTC()
	// Act.
	counts, e := s.PutExpiringHistoryPage(ctx, ref, []ExpiringHistoryRecord{{Message: m, ExpiresAtMS: time.Now().Add(time.Hour).UnixMilli()}})
	// Assert: no content, permanent identity, metadata or success prefix escapes rollback.
	if e == nil || counts != (HistoryPageCounts{}) {
		t.Fatal("failed page reported insertion")
	}
	for _, table := range []string{"messages", "message_identities", "peer_first_incoming", "conversations", "history_message_expiry", "message_events"} {
		var n int
		if e = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != 0 {
			t.Fatal("partial expiry page escaped", table, n, e)
		}
	}
}
