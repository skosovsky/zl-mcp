package storage

import (
	"context"
	"database/sql"
	"errors"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"path/filepath"
	"testing"
	"time"
)

func TestDeletedMessageCannotBeRestoredByHistoryOrReplay(t *testing.T) {
	// Arrange: an observed direct message is subsequently deleted by its exact identity.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.sqlite")
	s, e := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { s.Close() }()
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	m := historicalFixture(ref, "deleted")
	m.SentAt = time.Now().UTC()
	m.Source = "live"
	if e = s.Put(ctx, m); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteConversation(ctx, ref, m.ID); e != nil {
		t.Fatal(e)
	}
	// Act: replay and explicit history retries occur after restart.
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	m.Source = "replay"
	if e = s.Put(ctx, m); e != nil {
		t.Fatal(e)
	}
	m.Source = ""
	if _, e = s.PutHistoryPage(ctx, ref, []domain.Message{m}); e != nil {
		t.Fatal(e)
	}
	// Assert: neither the content nor a new Event is resurrected.
	if _, e = s.ConversationMessage(ctx, ref, m.ID); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("deleted message was restored", e)
	}
	var count int
	if e = s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&count); e != nil || count != 0 {
		t.Fatal("deleted payload retained", e)
	}
}

func TestDeletionBeforeMessageAndTypedNamespace(t *testing.T) {
	// Arrange: deletion precedes its payload and knows no incoming-direction fact.
	ctx := context.Background()
	s, e := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	direct := domain.ConversationRef{Type: "direct", ID: "same"}
	group := domain.ConversationRef{Type: "group", ID: "same"}
	if e = s.DeleteConversation(ctx, direct, "target"); e != nil {
		t.Fatal(e)
	}
	// Act: late payload is suppressed only for the exact typed identity.
	for _, ref := range []domain.ConversationRef{direct, group, {Type: "direct", ID: "other"}} {
		m := historicalFixture(ref, "target")
		m.Source = "live"
		if e = s.Put(ctx, m); e != nil {
			t.Fatal(e)
		}
	}
	// Assert: deletion neither removes another namespace nor claims a new contact.
	if _, e = s.ConversationMessage(ctx, direct, "target"); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("deleted late payload restored")
	}
	for _, ref := range []domain.ConversationRef{group, {Type: "direct", ID: "other"}} {
		if _, e = s.ConversationMessage(ctx, ref, "target"); e != nil {
			t.Fatal("unrelated namespace removed", e)
		}
	}
	var certainty string
	if e = s.DB.QueryRow("SELECT certainty FROM peer_first_incoming WHERE peer_id='same'").Scan(&certainty); e != nil || certainty != "unknown" {
		t.Fatal("deletion fabricated novelty", e)
	}
	if e = s.Retain(ctx); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.DB.QueryRow("SELECT count(*) FROM message_tombstones").Scan(&count); e != nil || count != 1 {
		t.Fatal("retention removed marker", e)
	}
}
func TestDeletionRollbackKeepsOriginalPayloadAndEvent(t *testing.T) {
	// Arrange: a storage failure occurs after tombstone and event cleanup began.
	ctx := context.Background()
	s, e := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	m := historicalFixture(ref, "target")
	m.Source = "live"
	if e = s.Put(ctx, m); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`CREATE TRIGGER fail_removal BEFORE DELETE ON messages BEGIN SELECT RAISE(ABORT,'synthetic failure');END`); e != nil {
		t.Fatal(e)
	}
	// Act.
	if e = s.DeleteConversation(ctx, ref, m.ID); e == nil {
		t.Fatal("failed deletion accepted")
	}
	// Assert: original corpus/event and absence of a permanent marker are atomic.
	stored, e := s.ConversationMessage(ctx, ref, m.ID)
	if e != nil || stored.Text != m.Text {
		t.Fatal("failed deletion lost content", e)
	}
	for table, want := range map[string]int{"messages": 1, "message_events": 1, "message_tombstones": 0} {
		var count int
		if e = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); e != nil || count != want {
			t.Fatal("partial deletion escaped", table, e)
		}
	}
}
