package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) migrateIncoming(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=6").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE message_events ADD COLUMN direction TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE message_events ADD COLUMN first_incoming INTEGER;
ALTER TABLE event_subscriptions ADD COLUMN direction TEXT NOT NULL DEFAULT 'all';
ALTER TABLE event_subscriptions ADD COLUMN first_incoming_only INTEGER NOT NULL DEFAULT 0;
CREATE TABLE peer_first_incoming(peer_id TEXT PRIMARY KEY,first_seq INTEGER,certainty TEXT NOT NULL CHECK(certainty IN ('known','unknown')));
INSERT INTO peer_first_incoming SELECT group_id,min(seq),'known' FROM messages WHERE conversation_type='direct' AND sender_id=group_id GROUP BY group_id;
INSERT OR IGNORE INTO peer_first_incoming SELECT group_id,NULL,'unknown' FROM message_identities WHERE conversation_type='direct' GROUP BY group_id;
INSERT INTO schema_migrations VALUES(6);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// markIncoming is called only for a new permanent message identity, atomically.
func markIncoming(ctx context.Context, tx *sql.Tx, seq int64, m *domain.Message) error {
	m.FirstIncoming = nil
	if m.Direction == "" {
		m.Direction = "unknown"
	}
	if m.Direction != "incoming" && m.Direction != "outgoing" && m.Direction != "unknown" {
		return domain.Invalid("Invalid message direction.")
	}
	if m.Ref().Type == domain.ConversationDirect && m.Direction == "unknown" {
		_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO peer_first_incoming(peer_id,first_seq,certainty) VALUES(?,NULL,'unknown')", m.Ref().ID)
		return err
	}
	if m.Ref().Type != domain.ConversationDirect || m.Direction != "incoming" {
		return nil
	}
	var first sql.NullInt64
	var certainty string
	err := tx.QueryRowContext(ctx, "SELECT first_seq,certainty FROM peer_first_incoming WHERE peer_id=?", m.Ref().ID).Scan(&first, &certainty)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = tx.ExecContext(ctx, "INSERT INTO peer_first_incoming(peer_id,first_seq,certainty) VALUES(?,?,'known')", m.Ref().ID, seq); err != nil {
			return err
		}
		v := true
		m.FirstIncoming = &v
		return nil
	}
	if err != nil {
		return err
	}
	if certainty == "known" {
		v := false
		m.FirstIncoming = &v
	}
	return nil
}
