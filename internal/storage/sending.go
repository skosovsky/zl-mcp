package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

var ErrSendConflict = errors.New("send request_id conflicts with original arguments")
var ErrSendCapacity = errors.New("send operation ledger is full")
var ErrSendState = errors.New("send operation is not sending")

const SendLedgerCapacity = 100000

func (s *Store) migrateSending(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS send_operations(
 request_id TEXT PRIMARY KEY,recipient_id TEXT NOT NULL,fingerprint TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('pending','sending','sent','failed','unknown')),
 message_id TEXT,reason TEXT,updated_at TEXT NOT NULL);
 INSERT OR IGNORE INTO schema_migrations VALUES(5);`)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS message_quote_metadata(
 message_seq INTEGER PRIMARY KEY REFERENCES messages(seq) ON DELETE CASCADE,
 client_message_id TEXT NOT NULL,message_type TEXT NOT NULL,timestamp TEXT NOT NULL,ttl INTEGER NOT NULL);`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SendQuote(ctx context.Context, peer, id string) (*domain.SendQuote, error) {
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: peer}
	if !s.AllowsConversation(ref) {
		return nil, domain.Invalid("Reply source is outside collection policy.")
	}
	var q domain.SendQuote
	err := s.DB.QueryRowContext(ctx, `SELECT m.message_id,m.sender_id,m.text,q.client_message_id,q.message_type,q.timestamp,q.ttl FROM visible_messages m JOIN message_quote_metadata q ON q.message_seq=m.seq WHERE m.conversation_type='direct' AND m.group_id=? AND m.message_id=?`, peer, id).Scan(&q.MessageID, &q.SenderID, &q.Text, &q.Metadata.ClientMessageID, &q.Metadata.MessageType, &q.Metadata.Timestamp, &q.Metadata.TTL)
	if err != nil {
		return nil, err
	}
	if q.Metadata.MessageType != "webchat" || q.Metadata.ClientMessageID == "" {
		return nil, domain.Invalid("Reply source lacks supported text quote metadata.")
	}
	return &q, nil
}

// RecoverInterruptedSends runs once under the service account lock, before calls.
// Opening another Store for read/diagnostics must not interrupt a live sender.
func (s *Store) RecoverInterruptedSends(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE send_operations SET state='unknown',reason='interrupted',updated_at=? WHERE state='sending'", now())
	return err
}

func (s *Store) PrepareSend(ctx context.Context, r domain.SendRequest) (domain.SendOperation, error) {
	if err := r.Validate(); err != nil {
		return domain.SendOperation{}, err
	}
	id, err := canonicalSendID(r.RequestID)
	if err != nil {
		return domain.SendOperation{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.SendOperation{}, err
	}
	defer tx.Rollback()
	var fingerprint string
	err = tx.QueryRowContext(ctx, "SELECT fingerprint FROM send_operations WHERE request_id=?", id).Scan(&fingerprint)
	if err == nil && fingerprint != r.Fingerprint() {
		return domain.SendOperation{}, ErrSendConflict
	}
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM send_operations").Scan(&count); err != nil {
			return domain.SendOperation{}, err
		}
		if count >= SendLedgerCapacity {
			return domain.SendOperation{}, ErrSendCapacity
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO send_operations(request_id,recipient_id,fingerprint,state,updated_at) VALUES(?,?,?,'pending',?)", id, r.RecipientID, r.Fingerprint(), now())
	}
	if err != nil {
		return domain.SendOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.SendOperation{}, err
	}
	return s.SendStatus(ctx, id)
}

func (s *Store) ClaimSend(ctx context.Context, id string) (bool, error) {
	id, err := canonicalSendID(id)
	if err != nil {
		return false, err
	}
	r, err := s.DB.ExecContext(ctx, "UPDATE send_operations SET state='sending',updated_at=? WHERE request_id=? AND state='pending'", now(), id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

func (s *Store) CompleteSend(ctx context.Context, id, state string, messageID *string, reason *string) error {
	id, err := canonicalSendID(id)
	if err != nil {
		return err
	}
	if state != "sent" && state != "failed" && state != "unknown" {
		return domain.Invalid("Invalid send completion state.")
	}
	if state == "sent" && (messageID == nil || *messageID == "") {
		return domain.Invalid("A sent result requires upstream message ID.")
	}
	if state != "sent" && messageID != nil {
		return domain.Invalid("Unconfirmed send must not supply a message ID.")
	}
	if state == "sent" && reason != nil {
		return domain.Invalid("Sent operation must not contain a failure reason.")
	}
	if reason != nil {
		switch *reason {
		case "interrupted", "upstream_rejected", "upstream_ambiguous", "quote_unavailable", "not_authenticated", "permission_denied":
		default:
			return domain.Invalid("Invalid send reason category.")
		}
	}
	r, err := s.DB.ExecContext(ctx, "UPDATE send_operations SET state=?,message_id=?,reason=?,updated_at=? WHERE request_id=? AND state='sending'", state, messageID, reason, now(), id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrSendState
	}
	return nil
}

func (s *Store) SendStatus(ctx context.Context, id string) (domain.SendOperation, error) {
	var op domain.SendOperation
	id, err := canonicalSendID(id)
	if err != nil {
		return op, err
	}
	var stamp string
	err = s.DB.QueryRowContext(ctx, "SELECT request_id,recipient_id,state,message_id,reason,updated_at FROM send_operations WHERE request_id=?", id).Scan(&op.RequestID, &op.RecipientID, &op.Status, &op.MessageID, &op.Reason, &stamp)
	if err != nil {
		return op, err
	}
	op.UpdatedAt, err = time.Parse(time.RFC3339Nano, stamp)
	return op, err
}
