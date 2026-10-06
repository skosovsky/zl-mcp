package storage

import (
	"context"
	"database/sql"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) migrateTombstones(ctx context.Context) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var done int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=11").Scan(&done); e != nil {
		return e
	}
	if done > 0 {
		return tx.Commit()
	}
	_, e = tx.ExecContext(ctx, `CREATE TABLE message_tombstones(conversation_type TEXT NOT NULL,conversation_id TEXT NOT NULL,message_id TEXT NOT NULL,deleted_at TEXT NOT NULL,PRIMARY KEY(conversation_type,conversation_id,message_id));
 DROP VIEW visible_messages;
 CREATE VIEW visible_messages AS SELECT m.* FROM messages m WHERE NOT EXISTS(SELECT 1 FROM message_tombstones d WHERE d.conversation_type=m.conversation_type AND d.conversation_id=m.group_id AND d.message_id=m.message_id) AND NOT EXISTS(
 SELECT 1 FROM history_message_expiry e WHERE e.conversation_type=m.conversation_type AND e.conversation_id=m.group_id AND e.message_id=m.message_id
 AND (e.expired=1 OR e.expires_ms <= CAST(strftime('%s','now') AS INTEGER)*1000+CAST(substr(strftime('%f','now'),4,3) AS INTEGER)));
 INSERT INTO schema_migrations VALUES(11);`)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) deleteObservedMessage(ctx context.Context, ref domain.ConversationRef, id string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = s.deleteObservedMessageTx(ctx, tx, ref, id); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) deleteObservedMessageTx(ctx context.Context, tx *sql.Tx, ref domain.ConversationRef, id string) error {
	if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO message_tombstones VALUES(?,?,?,?)", ref.Type, ref.ID, id, now()); e != nil {
		return e
	}
	if ref.Type == domain.ConversationDirect {
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO peer_first_incoming(peer_id,first_seq,certainty) VALUES(?,NULL,'unknown')", ref.ID); e != nil {
			return e
		}
	}
	return s.removeMessagesTx(ctx, tx, "conversation_type=? AND conversation_id=? AND message_id=?", "record_deleted", ref.Type, ref.ID, id)
}
