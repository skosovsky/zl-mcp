package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestPreloadDirectoryMergePreservesCorpusPolicyAndKnownMetadata(t *testing.T) {
	// Arrange
	ctx := context.Background()
	direct := domain.ConversationRef{Type: "direct", ID: "same"}
	group := domain.ConversationRef{Type: "group", ID: "same"}
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{direct: true, group: true}})
	defer s.Close()
	if _, err := s.PutContacts(ctx, []domain.Contact{{ID: "same", Name: "Trusted existing name", Aliases: []string{}, Friendship: "friend"}}); err != nil {
		t.Fatal(err)
	}
	observed := "Weaker preload name"
	newName := "Synthetic group"
	// Act
	n, err := s.PutPreloadEntries(ctx, []domain.PreloadEntry{{Conversation: direct, Name: &observed}, {Conversation: group, Name: &newName}, {Conversation: domain.ConversationRef{Type: "direct", ID: "excluded"}}})
	// Assert
	if err != nil || n != 2 {
		t.Fatal("preload merge failed", err)
	}
	var name, source string
	if err := s.DB.QueryRowContext(ctx, "SELECT name,metadata_source FROM conversations WHERE conversation_type='direct' AND conversation_id='same'").Scan(&name, &source); err != nil {
		t.Fatal(err)
	}
	if name != "Trusted existing name" || source != "contacts_catalog" {
		t.Fatal("weaker preload replaced known contact metadata")
	}
	if err := s.DB.QueryRowContext(ctx, "SELECT name,metadata_source FROM conversations WHERE conversation_type='group' AND conversation_id='same'").Scan(&name, &source); err != nil {
		t.Fatal(err)
	}
	if name != newName || source != "preload_catalog" {
		t.Fatal("typed preload group metadata lost")
	}
	for _, table := range []string{"messages", "message_identities", "message_events", "event_deliveries", "peer_first_incoming", "send_operations", "event_subscriptions"} {
		var count int
		if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("directory merge changed %s", table)
		}
	}
	var excluded int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM conversations WHERE conversation_id='excluded'").Scan(&excluded); err != nil {
		t.Fatal(err)
	}
	if excluded != 0 {
		t.Fatal("preload expanded selected collection policy")
	}
	for _, tool := range []string{"zalo_list_conversations", "zalo_get_conversation"} {
		var result map[string]any
		if tool == "zalo_list_conversations" {
			result, err = s.Conversations(ctx, "", "", 20, "")
		} else {
			result, err = s.Conversation(ctx, group)
		}
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		schema, err := contracts.Compile(tool, "output")
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPreloadDirectoryRejectsMalformedOrFailedWholePage(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	ref := domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}
	// Act / Assert: no valid prefix is committed on invalid identity or SQL failure.
	if _, err := s.PutPreloadEntries(ctx, []domain.PreloadEntry{{Conversation: ref}, {Conversation: domain.ConversationRef{Type: "unknown", ID: "bad"}}}); err == nil {
		t.Fatal("unknown identity accepted")
	}
	if _, err := s.DB.ExecContext(ctx, `CREATE TRIGGER fail_preload BEFORE INSERT ON conversations WHEN NEW.conversation_id='fail' BEGIN SELECT RAISE(FAIL,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutPreloadEntries(ctx, []domain.PreloadEntry{{Conversation: ref}, {Conversation: domain.ConversationRef{Type: "direct", ID: "fail"}}}); err == nil {
		t.Fatal("SQL failure hidden")
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM conversations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("preload merge returned a partial catalogue")
	}
}
