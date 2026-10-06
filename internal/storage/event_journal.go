package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// The transport is single-account; principal is supplied by the authenticated
// server, never by arguments or message text. Checkpoints are generation scoped.
func (s *Store) migrateEventJournal(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS event_processing(subscription_id TEXT NOT NULL,generation TEXT NOT NULL,ack_id INTEGER NOT NULL DEFAULT 0,pruned_id INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(subscription_id,generation));
UPDATE event_subscriptions SET active=0 WHERE profile='zalo.conversation.message.created';
UPDATE event_deliveries SET state='cancelled',payload=X'',lease_until=NULL,completed_at=COALESCE(completed_at,strftime('%Y-%m-%dT%H:%M:%fZ','now')),last_reason='profile_removed' WHERE subscription_id IN (SELECT id FROM event_subscriptions WHERE profile='zalo.conversation.message.created') AND state IN ('pending','sending');
INSERT OR IGNORE INTO schema_migrations VALUES(12);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EventSubscriptions(ctx context.Context, principal string) (map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,profile,scope,conversation_type,group_id,direction,first_incoming_only FROM event_subscriptions WHERE principal=? AND active=1 AND profile!='zalo.conversation.message.created' AND (expires_at IS NULL OR julianday(expires_at)>julianday(?)) ORDER BY id`, principal, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []map[string]any{}
	for rows.Next() {
		var id, profile, scope, kind, peer, dir string
		var first bool
		if err = rows.Scan(&id, &profile, &scope, &kind, &peer, &dir, &first); err != nil {
			return nil, err
		}
		if scope == "conversation" && !s.AllowsConversation(domain.ConversationRef{Type: kind, ID: peer}) {
			continue
		}
		var k, p any
		if kind != "" {
			k = kind
		}
		if peer != "" {
			p = peer
		}
		values = append(values, map[string]any{"subscription_id": id, "event_name": profile, "scope": scope, "conversation_type": k, "conversation_id": p, "direction": dir, "first_incoming_only": first})
	}
	return map[string]any{"subscriptions": values}, rows.Err()
}

type processingReceipt struct {
	Subscription string `json:"s"`
	Generation   string `json:"g"`
	From         int64  `json:"f"`
	To           int64  `json:"t"`
}

func (s *Store) receipt(r processingReceipt) string {
	b, _ := json.Marshal(r)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("event-processing:"))
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Store) parseReceipt(token string) (processingReceipt, error) {
	var r processingReceipt
	body, signature, ok := strings.Cut(token, ".")
	if !ok {
		return r, domain.Invalid("Invalid processing receipt.")
	}
	b, bodyErr := base64.RawURLEncoding.DecodeString(body)
	sig, signatureErr := base64.RawURLEncoding.DecodeString(signature)
	if bodyErr != nil || signatureErr != nil {
		return r, domain.Invalid("Invalid processing receipt.")
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("event-processing:"))
	mac.Write(b)
	if len(b) == 0 || !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(b, &r) != nil || r.To <= r.From {
		return r, domain.Invalid("Invalid processing receipt.")
	}
	return r, nil
}
func (s *Store) journalSubscription(ctx context.Context, tx *sql.Tx, id, principal string) (string, error) {
	var gen, profile, scope, kind, peer string
	var active bool
	var expiry sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT generation,profile,scope,conversation_type,group_id,active,expires_at FROM event_subscriptions WHERE id=? AND principal=? AND (expires_at IS NULL OR julianday(expires_at)>julianday(?))`, id, principal, now()).Scan(&gen, &profile, &scope, &kind, &peer, &active, &expiry)
	if err != nil {
		return "", err
	}
	if !active || profile == "zalo.conversation.message.created" || (scope == "conversation" && !s.AllowsConversation(domain.ConversationRef{Type: kind, ID: peer})) {
		return "", subscriptionPermission("Subscription is inactive or access was revoked.")
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO event_processing(subscription_id,generation) VALUES(?,?)`, id, gen)
	return gen, err
}

// Reads never advance processing. Each receipt confirms an ordered prefix of
// exactly this page; optimistic acknowledgement rejects stale parallel pages.
func (s *Store) ReadSubscriptionEvents(ctx context.Context, principal, id string, limit int) (map[string]any, error) {
	if limit < 1 || limit > 20 {
		return nil, domain.Invalid("Limit must be between 1 and 20.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	gen, err := s.journalSubscription(ctx, tx, id, principal)
	if err != nil {
		return nil, err
	}
	var ack, pruned int64
	if err = tx.QueryRowContext(ctx, `SELECT ack_id,pruned_id FROM event_processing WHERE subscription_id=? AND generation=?`, id, gen).Scan(&ack, &pruned); err != nil {
		return nil, err
	}
	entries := []map[string]any{}
	from := ack
	if pruned > from {
		entries = append(entries, map[string]any{"event": nil, "delivery_state": "pruned", "gap_reason": "retention_expired", "receipt": s.receipt(processingReceipt{id, gen, from, pruned})})
		from = pruned
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.id,d.payload,d.state,m.conversation_type,m.conversation_id FROM event_deliveries d LEFT JOIN messages m ON m.seq=d.message_seq WHERE d.subscription_id=? AND d.generation=? AND d.id>? ORDER BY d.id LIMIT ?`, id, gen, from, limit+1)
	if err != nil {
		return nil, err
	}
	more, blocked := false, false
	used := 0
	for rows.Next() {
		var n int64
		var payload []byte
		var state string
		var kind, peer sql.NullString
		if err = rows.Scan(&n, &payload, &state, &kind, &peer); err != nil {
			rows.Close()
			return nil, err
		}
		if state == "pending" || state == "sending" {
			blocked = true
			break
		}
		if len(entries) >= limit {
			more = true
			break
		}
		var event any
		var reason any
		if !kind.Valid || !s.AllowsConversation(domain.ConversationRef{Type: kind.String, ID: peer.String}) {
			reason = "message_unavailable_or_access_revoked"
		} else if len(payload) == 0 {
			reason = "payload_unavailable"
		} else if json.Unmarshal(payload, &event) != nil {
			reason = "invalid_payload"
			event = nil
		}
		if state != "delivered" {
			event = nil
			reason = "delivery_" + state
		}
		entry := map[string]any{"event": event, "delivery_state": state, "gap_reason": reason, "receipt": s.receipt(processingReceipt{id, gen, from, n})}
		b, _ := json.Marshal(entry)
		if used+len(b) > 28<<10 {
			if len(entries) == 0 {
				rows.Close()
				return nil, domain.ResponseTooLarge("Event exceeds recovery response budget.", "Read the retained message through its context tool; report this limitation.")
			}
			more = true
			break
		}
		used += len(b)
		entries = append(entries, entry)
		from = n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"subscription_id": id, "events": entries, "has_more": more, "blocked_on_delivery": blocked}, nil
}
func (s *Store) AckSubscriptionEvents(ctx context.Context, principal, id, token string) (map[string]any, error) {
	r, err := s.parseReceipt(token)
	if err != nil {
		return nil, err
	}
	if r.Subscription != id {
		return nil, domain.Invalid("Receipt belongs to another subscription.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	gen, err := s.journalSubscription(ctx, tx, id, principal)
	if err != nil {
		return nil, err
	}
	if r.Generation != gen {
		return nil, domain.Invalid("Subscription generation changed.")
	}
	var ack int64
	if err = tx.QueryRowContext(ctx, `SELECT ack_id FROM event_processing WHERE subscription_id=? AND generation=?`, id, gen).Scan(&ack); err != nil {
		return nil, err
	}
	if ack < r.To {
		if ack != r.From {
			return nil, domain.Invalid("Processing cursor changed; read remaining events again.")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE event_processing SET ack_id=? WHERE subscription_id=? AND generation=? AND ack_id=?`, r.To, id, gen, r.From); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"acknowledged": true}, nil
}

func (s *Store) pruneEventJournal(ctx context.Context, at time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cutoff := at.Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO event_processing(subscription_id,generation,pruned_id) SELECT subscription_id,generation,max(id) FROM event_deliveries WHERE completed_at IS NOT NULL AND julianday(completed_at)<julianday(?) AND NOT EXISTS(SELECT 1 FROM event_deliveries earlier WHERE earlier.subscription_id=event_deliveries.subscription_id AND earlier.generation=event_deliveries.generation AND earlier.id<event_deliveries.id AND (earlier.completed_at IS NULL OR julianday(earlier.completed_at)>=julianday(?))) GROUP BY subscription_id,generation ON CONFLICT(subscription_id,generation) DO UPDATE SET pruned_id=max(pruned_id,excluded.pruned_id)`, cutoff, cutoff)
	if err != nil {
		return err
	}
	// Do not prune beyond an unfinished earlier record: doing so would let a
	// retention gap acknowledge work that has not yet completed delivery.
	_, err = tx.ExecContext(ctx, `DELETE FROM event_deliveries WHERE completed_at IS NOT NULL AND julianday(completed_at)<julianday(?) AND NOT EXISTS(SELECT 1 FROM event_deliveries earlier WHERE earlier.subscription_id=event_deliveries.subscription_id AND earlier.generation=event_deliveries.generation AND earlier.id<event_deliveries.id AND (earlier.completed_at IS NULL OR julianday(earlier.completed_at)>=julianday(?)))`, cutoff, cutoff)
	if err != nil {
		return err
	}
	return tx.Commit()
}
