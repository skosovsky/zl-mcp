package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) migrateMessageClientIDs(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the first migration failure.
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=17`).
		Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `
CREATE TABLE message_client_ids(
 message_seq INTEGER PRIMARY KEY REFERENCES messages(seq) ON DELETE CASCADE,
 client_message_id TEXT NOT NULL);
CREATE INDEX message_client_lookup ON message_client_ids(client_message_id);
INSERT INTO schema_migrations VALUES(17);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func putMessageClientID(ctx context.Context, tx *sql.Tx, m domain.Message) error {
	id := m.ProtocolClientMessageID
	if id == "" {
		return nil
	}
	if !domain.ValidMessageClientID(id) {
		return domain.Invalid("Invalid message client identifier.")
	}
	ref := m.Ref()
	var seq int64
	err := tx.QueryRowContext(ctx, `SELECT seq FROM visible_messages WHERE conversation_type=? AND group_id=? AND message_id=? AND sender_id=? AND sent_at=?`,
		ref.Type, ref.ID, m.ID, m.SenderID, m.SentAt.UTC().Format("2006-01-02T15:04:05.000000000Z")).
		Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT client_message_id FROM message_client_ids WHERE message_seq=?`, seq).
		Scan(&existing)
	if err == nil {
		if existing == id {
			return nil
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO metadata(key,value) VALUES('message_client_id_conflicts','1')
ON CONFLICT(key) DO UPDATE SET value=CAST(MIN(CAST(value AS INTEGER)+1,2147483647) AS TEXT)`)
		return err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO message_client_ids VALUES(?,?)`, seq, id)
	return err
}
