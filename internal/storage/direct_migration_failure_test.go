package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestDirectMessagingMigrationsRollbackDDLAndPreserveCorpus(t *testing.T) {
	for _, version := range []int{5, 6} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			// Arrange: a retained record and a late failure inside the migration.
			ctx := context.Background()
			s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "state.sqlite"), domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			m := domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: "retained", SenderID: "peer", SentAt: time.Now(), Text: "synthetic retained", Source: "live", Direction: "incoming"}
			if err = s.Put(ctx, m); err != nil {
				t.Fatal(err)
			}
			reset := `DROP TABLE send_operations; DROP TABLE message_quote_metadata; DELETE FROM schema_migrations WHERE version=5;`
			migrate := s.migrateSending
			if version == 6 {
				reset = `DROP TABLE peer_first_incoming;
ALTER TABLE message_events DROP COLUMN direction;
ALTER TABLE message_events DROP COLUMN first_incoming;
ALTER TABLE event_subscriptions DROP COLUMN direction;
ALTER TABLE event_subscriptions DROP COLUMN first_incoming_only;
DELETE FROM schema_migrations WHERE version=6;`
				migrate = s.migrateIncoming
			}
			if _, err = s.DB.Exec(reset); err != nil {
				t.Fatal(err)
			}
			if _, err = s.DB.Exec(fmt.Sprintf(`CREATE TRIGGER reject_migration BEFORE INSERT ON schema_migrations WHEN NEW.version=%d BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END`, version)); err != nil {
				t.Fatal(err)
			}
			// Act.
			migrationErr := migrate(ctx)
			// Assert: neither the version marker nor partial DDL may survive.
			if migrationErr == nil {
				t.Fatal("injected failure did not abort migration")
			}
			var versions, artifacts int
			if err = s.DB.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=?", version).Scan(&versions); err != nil {
				t.Fatal(err)
			}
			query := "SELECT count(*) FROM sqlite_master WHERE name IN ('send_operations','message_quote_metadata')"
			if version == 6 {
				query = `SELECT (SELECT count(*) FROM pragma_table_info('message_events') WHERE name IN ('direction','first_incoming')) + (SELECT count(*) FROM pragma_table_info('event_subscriptions') WHERE name IN ('direction','first_incoming_only')) + (SELECT count(*) FROM sqlite_master WHERE name='peer_first_incoming')`
			}
			if err = s.DB.QueryRow(query).Scan(&artifacts); err != nil {
				t.Fatal(err)
			}
			var text string
			if err = s.DB.QueryRow("SELECT text FROM messages WHERE message_id='retained'").Scan(&text); err != nil {
				t.Fatal(err)
			}
			if versions != 0 || artifacts != 0 || text != m.Text {
				t.Fatal("failed migration left partial schema or changed corpus")
			}
			// Act / Assert: removing the failure permits a normal repeatable migration.
			if _, err = s.DB.Exec("DROP TRIGGER reject_migration"); err != nil {
				t.Fatal(err)
			}
			if err = migrate(ctx); err != nil {
				t.Fatal(err)
			}
			if err = migrate(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
