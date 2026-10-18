package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strconv"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// The additive table deliberately has no backfill: older rows do not establish
// observed ST/AT/CMD values. Message deletion/retention cascades to this table.
func (s *Store) migrateSeenMetadata(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve migration failure.
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS message_seen_metadata(
 message_seq INTEGER PRIMARY KEY REFERENCES messages(seq) ON DELETE CASCADE,
 client_message_id TEXT NOT NULL,recipient_id TEXT NOT NULL,message_type TEXT NOT NULL,
 timestamp TEXT NOT NULL,st INTEGER NOT NULL,at INTEGER NOT NULL,cmd INTEGER NOT NULL,
 direction TEXT NOT NULL CHECK(direction IN ('incoming','outgoing')));
 INSERT OR IGNORE INTO schema_migrations VALUES(18);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func putSeenMetadata(ctx context.Context, tx *sql.Tx, m domain.Message) error {
	p := m.SeenMetadata
	if p == nil {
		return nil
	}
	stamp, err := strconv.ParseInt(p.Timestamp, 10, 64)
	if err != nil || stamp <= 0 || stamp != m.SentAt.UnixMilli() || !domain.ValidMessageClientID(p.ClientMessageID) ||
		p.ClientMessageID != m.ProtocolClientMessageID || p.RecipientID == "" ||
		p.MessageType == "" ||
		(m.Direction != "incoming" && m.Direction != "outgoing") ||
		(m.Ref().Type == domain.ConversationGroup && p.RecipientID != m.Ref().ID) {
		return domain.Invalid("Read receipt provenance does not match the message.")
	}
	// Replay can fill missing provenance only for the retained exact identity;
	// existing evidence is immutable, including direction and the native counters.
	_, err = tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO message_seen_metadata
 SELECT m.seq,?,?,?,?,?,?,?,? FROM visible_messages m
 JOIN message_client_ids c ON c.message_seq=m.seq AND c.client_message_id=?
 WHERE m.conversation_type=? AND m.group_id=? AND m.message_id=? AND m.sender_id=? AND m.sent_at=?`,
		p.ClientMessageID,
		p.RecipientID,
		p.MessageType,
		p.Timestamp,
		p.ST,
		p.AT,
		p.CMD,
		m.Direction,
		p.ClientMessageID,
		m.Ref().Type,
		m.Ref().ID,
		m.ID,
		m.SenderID,
		m.SentAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
	)
	return err
}

// SeenTarget is private provenance; it is never serialized into a tool result.
type SeenTarget struct {
	SenderID string
	Metadata domain.SeenMetadata
}

func (s *Store) SeenTarget(
	ctx context.Context,
	ref domain.ConversationRef,
	id string,
) (SeenTarget, error) {
	var target SeenTarget
	if !ref.Valid() || !s.AllowsConversation(ref) {
		return target, domain.Invalid("Read receipt source is outside collection policy.")
	}
	p := &target.Metadata
	err := s.DB.QueryRowContext(ctx, `SELECT m.sender_id,p.client_message_id,p.recipient_id,p.message_type,p.timestamp,p.st,p.at,p.cmd
 FROM visible_messages m JOIN message_seen_metadata p ON p.message_seq=m.seq
 WHERE m.conversation_type=? AND m.group_id=? AND m.message_id=? AND p.direction='incoming'`, ref.Type, ref.ID, id).
		Scan(&target.SenderID, &p.ClientMessageID, &p.RecipientID, &p.MessageType, &p.Timestamp, &p.ST, &p.AT, &p.CMD)
	if err != nil {
		return target, err
	}
	account, err := historyAccount(ctx, s.DB)
	if err != nil {
		return SeenTarget{}, err
	}
	sender := sha256.Sum256([]byte(target.SenderID))
	if hex.EncodeToString(sender[:]) == account {
		return SeenTarget{}, sql.ErrNoRows
	}
	if ref.Type == domain.ConversationDirect {
		recipient := sha256.Sum256([]byte(p.RecipientID))
		if target.SenderID != ref.ID || hex.EncodeToString(recipient[:]) != account {
			return SeenTarget{}, sql.ErrNoRows
		}
	}
	return target, nil
}

// MessageActionTarget resolves exact retained server/client identifiers. Ordinary
// reads cannot manufacture this value, and archive-row IDs cannot be targets.
func (s *Store) MessageActionTarget(
	ctx context.Context,
	ref domain.ConversationRef,
	id string,
	read bool,
) (domain.MessageActionTarget, error) {
	target := domain.MessageActionTarget{Conversation: ref, MessageID: id, ClientMessageID: "", SenderID: "", Seen: nil}
	if !ref.Valid() || !s.AllowsConversation(ref) {
		return target, domain.Invalid("Action source is outside collection policy.")
	}
	if read {
		source, err := s.SeenTarget(ctx, ref, id)
		if err != nil {
			return target, err
		}
		target.SenderID = source.SenderID
		target.ClientMessageID = source.Metadata.ClientMessageID
		target.Seen = &source.Metadata
		return target, nil
	}
	err := s.DB.QueryRowContext(ctx, `SELECT m.sender_id,c.client_message_id FROM visible_messages m JOIN message_client_ids c ON c.message_seq=m.seq WHERE m.conversation_type=? AND m.group_id=? AND m.message_id=?`, ref.Type, ref.ID, id).
		Scan(&target.SenderID, &target.ClientMessageID)
	return target, err
}
