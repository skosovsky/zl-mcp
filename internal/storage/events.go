package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) migrateEvents(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS message_events(seq INTEGER PRIMARY KEY,event_id TEXT UNIQUE NOT NULL,group_id TEXT NOT NULL,message_json TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS message_events_group ON message_events(group_id,seq);
CREATE TABLE IF NOT EXISTS message_identities(group_id TEXT NOT NULL,message_id TEXT NOT NULL,first_seq INTEGER NOT NULL,PRIMARY KEY(group_id,message_id));
INSERT OR IGNORE INTO message_identities(group_id,message_id,first_seq) SELECT group_id,message_id,seq FROM messages WHERE NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=4);
CREATE TABLE IF NOT EXISTS event_subscription_revisions(id TEXT PRIMARY KEY,revision INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS event_subscriptions(id TEXT PRIMARY KEY,principal TEXT NOT NULL,group_id TEXT NOT NULL,callback TEXT NOT NULL,secret TEXT NOT NULL,active INTEGER NOT NULL,generation TEXT NOT NULL,start_seq INTEGER NOT NULL,expires_at TEXT,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS event_deliveries(id INTEGER PRIMARY KEY AUTOINCREMENT,event_id TEXT NOT NULL,subscription_id TEXT NOT NULL,generation TEXT NOT NULL,payload BLOB NOT NULL,state TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt_at TEXT NOT NULL,deadline TEXT NOT NULL,lease_until TEXT,last_reason TEXT,completed_at TEXT,UNIQUE(event_id,subscription_id,generation));
CREATE INDEX IF NOT EXISTS event_deliveries_ready ON event_deliveries(state,next_attempt_at);
CREATE INDEX IF NOT EXISTS event_deliveries_subscription_order ON event_deliveries(subscription_id,id,state);
CREATE TABLE IF NOT EXISTS event_fanout(id INTEGER PRIMARY KEY CHECK(id=1),seq INTEGER NOT NULL);
INSERT OR IGNORE INTO event_fanout VALUES(1,0);
INSERT OR IGNORE INTO schema_migrations VALUES(2);`)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(event_deliveries)")
	if err != nil {
		return err
	}
	hasSeq := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		hasSeq = hasSeq || name == "message_seq"
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasSeq {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE event_deliveries ADD COLUMN message_seq INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations VALUES(3)"); err != nil {
		return err
	}
	return tx.Commit()
}

func eventRandomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func appendMessageEvent(ctx context.Context, tx *sql.Tx, seq int64, m domain.Message) error {
	result, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO message_identities(group_id,conversation_type,message_id,first_seq) VALUES(?,?,?,?)", m.Ref().ID, m.Ref().Type, m.ID, seq)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		return nil
	}
	id, err := eventRandomID()
	if err != nil {
		return err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO message_events(seq,event_id,group_id,conversation_type,message_json,created_at) VALUES(?,?,?,?,?,?)", seq, "evt_"+id, m.Ref().ID, m.Ref().Type, string(body), now())
	return err
}

type EventSubscription struct {
	Profile          string
	Scope            string
	ConversationType string
	ID               string
	Principal        string
	GroupID          string
	Callback         string
	Secret           string
	Generation       string
	StartSeq         int64
	ExpiresAt        *time.Time
}

var ErrSubscriptionCancelled = errors.New("subscription cancelled during verification")

// SubscriptionRevision is captured before callback verification. A durable
// cancellation increments it even when no active subscription exists yet.
func (s *Store) SubscriptionRevision(ctx context.Context, id string) (int64, error) {
	if _, err := s.DB.ExecContext(ctx, "INSERT OR IGNORE INTO event_subscription_revisions(id) VALUES(?)", id); err != nil {
		return 0, err
	}
	var revision int64
	err := s.DB.QueryRowContext(ctx, "SELECT revision FROM event_subscription_revisions WHERE id=?", id).Scan(&revision)
	return revision, err
}

// ActivateSubscription is called only after identity, callback and key checks.
// Its transaction orders the activation boundary with the collector's Put.
func (s *Store) ActivateSubscription(ctx context.Context, sub EventSubscription, revision int64, at time.Time) (EventSubscription, error) {
	var normalizeErr error
	sub, normalizeErr = normalizeSubscription(sub)
	if normalizeErr != nil {
		return sub, normalizeErr
	}
	if sub.Scope == "conversation" && !s.AllowsConversation(domain.ConversationRef{Type: sub.ConversationType, ID: sub.GroupID}) {
		return sub, subscriptionPermission("Conversation is not enabled for collection.")
	}
	if sub.ID == "" || sub.Principal == "" || sub.Callback == "" || sub.Secret == "" {
		return sub, domain.Invalid("Subscription identity and delivery are required.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return sub, err
	}
	defer tx.Rollback()
	var current int64
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM event_subscription_revisions WHERE id=?", sub.ID).Scan(&current); err != nil {
		return sub, err
	}
	if current != revision {
		return sub, ErrSubscriptionCancelled
	}
	var principal, group, callback, generation, profile, scope, kind string
	var active int
	var start int64
	var expiry sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT principal,group_id,callback,active,generation,start_seq,expires_at,profile,scope,conversation_type FROM event_subscriptions WHERE id=?", sub.ID).Scan(&principal, &group, &callback, &active, &generation, &start, &expiry, &profile, &scope, &kind)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return sub, err
	}
	if err == nil && (principal != sub.Principal || group != sub.GroupID || callback != sub.Callback || profile != sub.Profile || scope != sub.Scope || kind != sub.ConversationType) {
		return sub, subscriptionPermission("Subscription belongs to another identity.")
	}
	reuse := err == nil && active == 1
	if reuse && expiry.Valid {
		expires, err := time.Parse(time.RFC3339Nano, expiry.String)
		if err != nil {
			return sub, err
		}
		reuse = at.Before(expires)
	}
	if reuse {
		sub.Generation, sub.StartSeq = generation, start
	} else {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM event_subscriptions WHERE active=1 AND (expires_at IS NULL OR julianday(expires_at)>julianday(?))", at.UTC().Format(time.RFC3339Nano)).Scan(&count); err != nil {
			return sub, err
		}
		if count >= 100 {
			return sub, fmt.Errorf("active subscription capacity exceeded")
		}
		if err = tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name='messages'),0)").Scan(&sub.StartSeq); err != nil {
			return sub, err
		}
		sub.Generation, err = eventRandomID()
		if err != nil {
			return sub, err
		}
	}
	var expires any
	if sub.ExpiresAt != nil {
		expires = sub.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO event_subscriptions(id,principal,group_id,callback,secret,active,generation,start_seq,expires_at,created_at,profile,scope,conversation_type) VALUES(?,?,?,?,?,1,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET secret=excluded.secret,active=1,generation=excluded.generation,start_seq=excluded.start_seq,expires_at=excluded.expires_at`, sub.ID, sub.Principal, sub.GroupID, sub.Callback, sub.Secret, sub.Generation, sub.StartSeq, expires, at.UTC().Format(time.RFC3339Nano), sub.Profile, sub.Scope, sub.ConversationType)
	if err != nil {
		return sub, err
	}
	return sub, tx.Commit()
}

func (s *Store) CancelSubscription(ctx context.Context, id, principal string, at time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	err = tx.QueryRowContext(ctx, "SELECT principal FROM event_subscriptions WHERE id=?", id).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && owner != principal {
		return subscriptionPermission("Subscription belongs to another identity.")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO event_subscription_revisions(id,revision) VALUES(?,1) ON CONFLICT(id) DO UPDATE SET revision=revision+1`, id)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE event_subscriptions SET active=0 WHERE id=?", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE event_deliveries SET state='cancelled',completed_at=?,lease_until=NULL,last_reason='cancelled' WHERE subscription_id=? AND state IN ('pending','sending')", at.UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	return tx.Commit()
}

func subscriptionPermission(message string) error {
	return &domain.Error{Code: "PERMISSION_DENIED", Message: message, NextAction: domain.NextAction{Instruction: "Use an authorized subscription and an enabled group."}, Details: map[string]any{}}
}
