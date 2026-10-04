package storage

import (
	"context"
	"database/sql"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type HistoryPageCounts struct {
	Inserted   int
	Duplicates int
}

// PutHistoryPage persists a validated page silently. It is an internal storage
// port, not an MCP capability or a substitute for the durable operation journal.
func (s *Store) PutHistoryPage(ctx context.Context, ref domain.ConversationRef, messages []domain.Message) (HistoryPageCounts, error) {
	if err := s.validateHistoryPage(ref, messages); err != nil {
		return HistoryPageCounts{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryPageCounts{}, err
	}
	defer tx.Rollback()
	counts, err := s.putHistoryPageTx(ctx, tx, messages)
	if err != nil {
		return HistoryPageCounts{}, err
	}
	if err = tx.Commit(); err != nil {
		return HistoryPageCounts{}, err
	}
	return counts, nil
}

func (s *Store) validateHistoryPage(ref domain.ConversationRef, messages []domain.Message) error {
	if !ref.Valid() || len(messages) > 50 {
		return domain.Invalid("History requires a typed conversation and at most 50 records.")
	}
	if !s.AllowsConversation(ref) {
		return subscriptionPermission("Conversation is outside collection policy.")
	}
	// Validate the whole page before entering the transaction. Normalized source
	// records must not be reclassified ordinary live/replay records.
	for _, m := range messages {
		if m.Ref() != ref || m.ID == "" || m.SenderID == "" || m.SentAt.IsZero() || len(m.Text) > 1<<20 || m.Source != "" {
			return domain.Invalid("Invalid normalized history record.")
		}
		if m.Direction != "" && m.Direction != "incoming" && m.Direction != "outgoing" && m.Direction != "unknown" {
			return domain.Invalid("Invalid historical message direction.")
		}
	}
	return nil
}

// A durable operation can use this helper and advance its checkpoint in the
// same transaction. The caller must perform whole-page validation first.
func (s *Store) putHistoryPageTx(ctx context.Context, tx *sql.Tx, messages []domain.Message) (HistoryPageCounts, error) {
	counts := HistoryPageCounts{}
	for _, m := range messages {
		m.Source = "history"
		m.FirstIncoming = nil
		if m.AttachmentTypes == nil {
			m.AttachmentTypes = []string{}
		}
		inserted, err := s.putMessageTx(ctx, tx, m, true)
		if err != nil {
			return HistoryPageCounts{}, err
		}
		if inserted {
			counts.Inserted++
		} else {
			counts.Duplicates++
		}
	}
	return counts, nil
}

func appendHistoryIdentity(ctx context.Context, tx *sql.Tx, seq int64, m domain.Message) error {
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO message_identities(group_id,conversation_type,message_id,first_seq) VALUES(?,?,?,?)", m.Ref().ID, m.Ref().Type, m.ID, seq); err != nil {
		return err
	}
	if m.Ref().Type == domain.ConversationDirect {
		// Incomplete old history cannot prove a new contact. Preserve any existing
		// known fact; unknown does not match a positive first-incoming filter.
		_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO peer_first_incoming(peer_id,first_seq,certainty) VALUES(?,NULL,'unknown')", m.Ref().ID)
		return err
	}
	return nil
}
