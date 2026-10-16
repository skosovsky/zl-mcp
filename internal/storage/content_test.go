package storage

import (
	"context"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

//nolint:govet // Test error scopes deliberately keep fault injection and recovery adjacent.
func TestContentMigrationRollsBackAndOldColumnWritersStillWork(t *testing.T) {
	// Arrange: recreate only the pre-v15 content schema in a temporary store.
	s := openTest(t)
	ctx := context.Background()
	putTest(t, s, "before", "g1", "kept", time.Now())
	if _, err := s.DB.ExecContext(ctx, `
ALTER TABLE messages DROP COLUMN content_json;
DELETE FROM schema_migrations WHERE version=15;
CREATE TRIGGER reject_content_migration BEFORE INSERT ON schema_migrations WHEN NEW.version=15
BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END;`); err != nil {
		t.Fatal(err)
	}
	// Act: the version marker fails after the column ALTER.
	err := s.migrateMessageContent(ctx)
	// Assert: schema and data roll back together.
	if err == nil {
		t.Fatal("migration failure hidden")
	}
	var column, version, records int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('messages') WHERE name='content_json'`).
		Scan(&column); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=15`).
		Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE message_id='before'`).
		Scan(&records); err != nil {
		t.Fatal(err)
	}
	if column != 0 || version != 0 || records != 1 {
		t.Fatal("migration escaped rollback", column, version, records)
	}
	// Arrange / Act: retry after removing the fault, then exercise an old writer.
	if _, err := s.DB.ExecContext(ctx, `DROP TRIGGER reject_content_migration`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateMessageContent(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(
		ctx,
		`INSERT INTO messages(group_id,message_id,sender_id,sent_at,received_at,text,attachments,source) VALUES('g1','old-writer','peer','2026-10-15T00:00:00Z','2026-10-15T00:00:00Z','old','[]','live')`,
	); err != nil {
		t.Fatal(err)
	}
	got, err := s.Message(ctx, "g1", "old-writer")
	// Assert: no rich type is invented for an old projection.
	if err != nil || got.Content != nil || got.Text != "old" {
		t.Fatal("old column reader/writer failed", err)
	}
}

func TestContentMetadataRecallDoesNotRemainInReadOrOutbox(t *testing.T) {
	// Arrange: only bounded synthetic metadata, no media fetch.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	sub := EventSubscription{
		ID:               "rich",
		Profile:          domain.ConversationMessageCreatedV3,
		Principal:        "owner",
		Scope:            "conversation",
		ConversationType: "group",
		GroupID:          "g1",
		Callback:         "https://receiver.example",
		Secret:           "synthetic",
	}
	revision, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateSubscription(ctx, sub, revision, at); err != nil {
		t.Fatal(err)
	}
	m := testEventMessage("rich", at)
	m.Content = domain.LegacyContent()
	if err := s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FanoutProfileEvents(
		ctx,
		at,
		DefaultDeliveryPolicy(),
		func(_, _ string, m domain.Message) ([]byte, error) { return encodeContentMessage(m) },
	); err != nil {
		t.Fatal(err)
	}
	// Act: recall clears payload and metadata with the underlying row.
	if err := s.DeleteConversation(ctx, domain.ConversationRef{Type: "group", ID: "g1"}, "rich"); err != nil {
		t.Fatal(err)
	}
	// Assert: neither context/resource storage nor pending callback retains data.
	if _, err := s.Message(ctx, "g1", "rich"); err == nil {
		t.Fatal("recalled metadata readable")
	}
	var bodies int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM event_deliveries WHERE length(payload)>0`).
		Scan(&bodies); err != nil {
		t.Fatal(err)
	}
	if bodies != 0 {
		t.Fatal("recalled metadata survived in callback")
	}
}
