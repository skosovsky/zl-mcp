package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestContactsMetadataDoesNotCreateMessagesOrNovelty(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	peer := domain.ConversationRef{Type: "direct", ID: "peer"}
	// Act
	count, err := s.PutContacts(ctx, []domain.Contact{{ID: peer.ID, Name: "Hoài An", Aliases: []string{"Đặng Seoul"}, Friendship: "unknown"}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.Conversations(ctx, "direct", "dang seoul", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	records := list["conversations"].([]map[string]any)
	if count != 1 || len(records) != 1 || records[0]["has_stored_messages"] != false || records[0]["availability"] != "unknown" || list["catalog_complete"] != false {
		t.Fatalf("wrong metadata-only catalogue: %#v", list)
	}
	for _, table := range []string{"messages", "message_events", "peer_first_incoming"} {
		var n int
		if err = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("metadata generated %s records", table)
		}
	}
	// Act: observing a real message upgrades availability without erasing aliases.
	if err = s.Put(ctx, domain.Message{Conversation: peer, ID: "message", SenderID: peer.ID, SentAt: time.Now().UTC(), Text: "synthetic", Source: "live"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Conversation(ctx, peer)
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	record := got["conversation"].(map[string]any)
	if record["availability"] != "observed" || record["has_stored_messages"] != true || len(record["aliases"].([]string)) != 2 {
		t.Fatalf("lost merged metadata: %#v", record)
	}
}

func TestContactsPolicyAtomicPageAndReopen(t *testing.T) {
	// Arrange
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	ref := domain.ConversationRef{Type: "direct", ID: "allowed"}
	policy := domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{ref: true}}
	s, err := OpenWithPolicy(ctx, path, policy, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	// Act
	count, err := s.PutContacts(ctx, []domain.Contact{{ID: "excluded", Name: "Private", Friendship: "friend"}, {ID: ref.ID, Name: "Allowed", Friendship: "friend"}})
	// Assert
	if err != nil || count != 1 {
		t.Fatalf("policy: %d %v", count, err)
	}
	// Act: the second invalid record rolls back the entire page.
	count, err = s.PutContacts(ctx, []domain.Contact{{ID: ref.ID, Name: "Changed", Friendship: "friend"}, {ID: "", Friendship: "unknown"}})
	if err == nil || count != 0 {
		t.Fatal("invalid page accepted")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, policy, 90)
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	var name string
	var n int
	if err = s.DB.QueryRowContext(ctx, "SELECT name FROM directory_contacts WHERE peer_id=?", ref.ID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM directory_contacts").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if name != "Allowed" || n != 1 {
		t.Fatal("rollback, policy or reopen lost")
	}
}
