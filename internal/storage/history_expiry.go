package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type ExpiringHistoryRecord = domain.ExpiringHistoryRecord

func (s *Store) migrateHistoryExpiry(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS history_message_expiry(
 conversation_type TEXT NOT NULL,conversation_id TEXT NOT NULL,message_id TEXT NOT NULL,
 expires_ms INTEGER NOT NULL CHECK(expires_ms>0),expired INTEGER NOT NULL DEFAULT 0 CHECK(expired IN (0,1)),PRIMARY KEY(conversation_type,conversation_id,message_id));
 CREATE INDEX IF NOT EXISTS history_expiry_deadline ON history_message_expiry(expires_ms);
 CREATE VIEW IF NOT EXISTS visible_messages AS SELECT m.* FROM messages m WHERE NOT EXISTS(
 SELECT 1 FROM history_message_expiry e WHERE e.conversation_type=m.conversation_type AND e.conversation_id=m.group_id AND e.message_id=m.message_id
 AND (e.expired=1 OR e.expires_ms <= CAST(strftime('%s','now') AS INTEGER)*1000+CAST(substr(strftime('%f','now'),4,3) AS INTEGER)));
 INSERT OR IGNORE INTO schema_migrations VALUES(10);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// PutExpiringHistoryPage persists silent messages and original deadlines atomically.
// A durable importer must use its own transaction for the page and checkpoint.
func (s *Store) PutExpiringHistoryPage(ctx context.Context, ref domain.ConversationRef, records []ExpiringHistoryRecord) (HistoryPageCounts, error) {
	if len(records) > 50 {
		return HistoryPageCounts{}, domain.Invalid("History requires at most 50 records.")
	}
	messages := make([]domain.Message, len(records))
	for i, r := range records {
		messages[i] = r.Message
		if r.ExpiresAtMS < 0 || r.ExpiresAtMS > 0 && (r.Message.SentAt.UnixMilli() <= 0 || r.ExpiresAtMS <= r.Message.SentAt.UnixMilli()) {
			return HistoryPageCounts{}, domain.Invalid("Invalid historical expiry deadline.")
		}
	}
	if err := s.validateHistoryPage(ref, messages); err != nil {
		return HistoryPageCounts{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryPageCounts{}, err
	}
	defer tx.Rollback()
	counts, err := s.putExpiringHistoryPageTx(ctx, tx, records)
	if err != nil {
		return HistoryPageCounts{}, err
	}
	if err = s.expireHistoryTx(ctx, tx, time.Now()); err != nil {
		return HistoryPageCounts{}, err
	}
	if err = tx.Commit(); err != nil {
		return HistoryPageCounts{}, err
	}
	return counts, nil
}
func (s *Store) putExpiringHistoryPageTx(ctx context.Context, tx *sql.Tx, records []ExpiringHistoryRecord) (HistoryPageCounts, error) {
	counts := HistoryPageCounts{}
	for _, r := range records {
		single, e := s.putHistoryPageTx(ctx, tx, []domain.Message{r.Message})
		if e != nil {
			return HistoryPageCounts{}, e
		}
		counts.Inserted += single.Inserted
		counts.Duplicates += single.Duplicates
		if single.Inserted == 1 && r.ExpiresAtMS > 0 {
			ref := r.Message.Ref()
			if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO history_message_expiry(conversation_type,conversation_id,message_id,expires_ms) VALUES(?,?,?,?)`, ref.Type, ref.ID, r.Message.ID, r.ExpiresAtMS); e != nil {
				return HistoryPageCounts{}, e
			}
		}
	}
	return counts, nil
}

// ExpireHistoryAt is a trusted maintenance port, never a user-supplied clock.
func (s *Store) ExpireHistoryAt(ctx context.Context, at time.Time) error {
	if at.IsZero() || at.UnixMilli() <= 0 {
		return domain.Invalid("Invalid expiry clock.")
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = s.expireHistoryTx(ctx, tx, at); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) expireHistoryTx(ctx context.Context, tx *sql.Tx, at time.Time) error {
	if _, e := tx.ExecContext(ctx, "UPDATE history_message_expiry SET expired=1 WHERE expired=0 AND expires_ms<=?", at.UnixMilli()); e != nil {
		return e
	}
	return s.removeMessagesTx(ctx, tx, `EXISTS (SELECT 1 FROM history_message_expiry e WHERE e.conversation_type=messages.conversation_type AND e.conversation_id=messages.group_id AND e.message_id=messages.message_id AND e.expired=1)`, "record_expired")
}
