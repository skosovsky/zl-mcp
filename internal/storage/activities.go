package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) migrateActivities(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the first migration failure.
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=16").
		Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `
CREATE TABLE activity_observations(
 seq INTEGER PRIMARY KEY AUTOINCREMENT,event_id TEXT NOT NULL UNIQUE,source_key TEXT NOT NULL UNIQUE,
 family TEXT NOT NULL CHECK(family IN ('call','reaction')),conversation_type TEXT NOT NULL CHECK(conversation_type IN ('direct','group')),
 conversation_id TEXT NOT NULL,direction TEXT NOT NULL CHECK(direction IN ('incoming','outgoing','unknown')),
 payload_json TEXT NOT NULL,created_at TEXT NOT NULL,expired INTEGER NOT NULL DEFAULT 0 CHECK(expired IN (0,1)));
CREATE TABLE activity_fanout(id INTEGER PRIMARY KEY CHECK(id=1),seq INTEGER NOT NULL);
INSERT INTO activity_fanout VALUES(1,0);
ALTER TABLE event_deliveries ADD COLUMN activity_seq INTEGER;
CREATE INDEX delivery_activity ON event_deliveries(activity_seq);
INSERT INTO schema_migrations VALUES(16);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// PutActivity preserves a deduplication tombstone independently of messages.
// Historical imports never call this port. Duplicate observations do not change
// novelty, source identity, payload or their activation order.
func (s *Store) PutActivity(ctx context.Context, a domain.Activity) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if !s.AllowsConversation(a.Ref()) {
		return nil
	}
	identity, _ := json.Marshal(
		[]string{a.Family, a.ConversationType, a.ConversationID, a.SourceID},
	)
	digest := sha256.Sum256(identity)
	key := hex.EncodeToString(digest[:])
	legacyKey := key
	if a.LegacyReactionSourceID != "" {
		legacyIdentity, _ := json.Marshal(
			[]string{a.Family, a.ConversationType, a.ConversationID, a.LegacyReactionSourceID},
		)
		legacyDigest := sha256.Sum256(legacyIdentity)
		legacyKey = hex.EncodeToString(legacyDigest[:])
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the first persistence failure.
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM activity_observations WHERE source_key=? OR source_key=?`, key, legacyKey).
		Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return tx.Commit()
	}
	var previousMessage int
	if a.Family == "call" && a.ProtocolMessageID != "" {
		if err = tx.QueryRowContext(ctx,
			`SELECT count(*) FROM message_identities WHERE conversation_type=? AND group_id=? AND message_id=?`,
			a.ConversationType, a.ConversationID, a.ProtocolMessageID).Scan(&previousMessage); err != nil {
			return err
		}
	}
	a, err = resolveActivityTargets(ctx, tx, a)
	if err != nil {
		return err
	}
	body, err := json.Marshal(a)
	if err != nil {
		return err
	}
	expired := previousMessage != 0
	if expired {
		body = []byte{}
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO activity_observations(event_id,source_key,family,conversation_type,conversation_id,direction,payload_json,created_at,expired) VALUES(?,?,?,?,?,?,?,?,?)`,
		"act_"+key,
		key,
		a.Family,
		a.ConversationType,
		a.ConversationID,
		a.Direction,
		string(body),
		time.Now().UTC().Format(time.RFC3339Nano),
		expired,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func subscriptionSequence(profile string) string {
	if domain.IsActivityProfile(profile) {
		return "activity_observations"
	}
	return "messages"
}
