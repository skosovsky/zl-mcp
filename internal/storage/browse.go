package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Browse reads a typed local conversation without a full-text search predicate.
// Its signed cursor freezes the insertion high-water mark, not retention or gaps.
func (s *Store) Browse(ctx context.Context, ref domain.ConversationRef, since, until, order string, limit int, token string) (map[string]any, error) {
	if !ref.Valid() {
		return nil, domain.Invalid("Typed conversation required.")
	}
	if !s.AllowsConversation(ref) {
		return nil, subscriptionPermission("Conversation is outside collection policy.")
	}
	if limit == 0 {
		limit = 20
	}
	if order == "" {
		order = "desc"
	}
	if limit < 1 || limit > 50 || (order != "asc" && order != "desc") {
		return nil, domain.Invalid("Use limit 1–50 and order asc or desc.")
	}
	var bounds [2]time.Time
	for i, value := range []string{since, until} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, domain.Invalid("since/until must be RFC3339 instants, e.g. 2026-10-01T00:00:00Z.")
		}
		bounds[i] = parsed
	}
	if !bounds[0].IsZero() && !bounds[1].IsZero() && !bounds[0].Before(bounds[1]) {
		return nil, domain.Invalid("since must precede until.")
	}
	filter, _ := json.Marshal([]string{"browse", ref.Type, ref.ID, since, until, order})
	sum := sha256.Sum256(filter)
	fingerprint := hex.EncodeToString(sum[:])
	c := cursor{Fingerprint: fingerprint, SnapshotAt: now()}
	if token != "" {
		if err := s.DecodeCursor(token, &c); err != nil {
			return nil, err
		}
		if c.Fingerprint != fingerprint {
			return nil, domain.Invalid("Cursor filters or order differ from the original browse.")
		}
	} else if err := s.DB.QueryRowContext(ctx, "SELECT coalesce(max(seq),0) FROM messages").Scan(&c.Snapshot); err != nil {
		return nil, err
	}
	clauses := []string{"m.conversation_type=?", "m.group_id=?", "m.seq<=?"}
	args := []any{ref.Type, ref.ID, c.Snapshot}
	for i, bound := range bounds {
		if bound.IsZero() {
			continue
		}
		op := ">="
		if i == 1 {
			op = "<"
		}
		clauses = append(clauses, "m.sent_at"+op+"?")
		args = append(args, bound.UTC().Format("2006-01-02T15:04:05.000000000Z"))
	}
	if c.ID != "" {
		op := ">"
		if order == "desc" {
			op = "<"
		}
		clauses = append(clauses, "(m.sent_at"+op+"? OR (m.sent_at=? AND m.message_id"+op+"?))")
		args = append(args, c.At, c.At, c.ID)
	}
	args = append(args, limit+1)
	rows, err := s.DB.QueryContext(ctx, `SELECT m.message_id,m.sender_id,m.sender_name,g.name,m.sent_at,m.text FROM messages m LEFT JOIN conversations g ON g.conversation_type=m.conversation_type AND g.conversation_id=m.group_id WHERE `+strings.Join(clauses, " AND ")+" ORDER BY m.sent_at "+order+",m.message_id "+order+" LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	values := []map[string]any{}
	times := []string{}
	senders := []string{}
	for rows.Next() {
		var id, sender, stamp, body string
		var senderName, name *string
		if err = rows.Scan(&id, &sender, &senderName, &name, &stamp, &body); err != nil {
			rows.Close()
			return nil, err
		}
		runes := []rune(body)
		truncated := len(runes) > 150
		var uri any
		if truncated {
			runes = runes[:150]
			uri = ConversationMessageURI(ref, id)
		}
		values = append(values, map[string]any{"conversation_type": ref.Type, "conversation_id": ref.ID, "conversation_name": name, "message_id": id, "sender_id": sender, "sender_name": senderName, "sent_at": stamp, "excerpt": string(runes), "text_truncated": truncated, "text_resource_uri": uri, "direction": "unknown"})
		times = append(times, stamp)
		senders = append(senders, sender)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var ownHash string
	err = s.DB.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='account'").Scan(&ownHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	for i, v := range values {
		hash := sha256.Sum256([]byte(senders[i]))
		if ownHash != "" && hex.EncodeToString(hash[:]) == ownHash {
			v["direction"] = "outgoing"
		} else if (ref.Type == domain.ConversationDirect && senders[i] == ref.ID) || (ref.Type == domain.ConversationGroup && ownHash != "") {
			v["direction"] = "incoming"
		}
	}
	var next any
	more := len(values) > limit
	if more {
		values = values[:limit]
		c.At = times[limit-1]
		c.ID = values[limit-1]["message_id"].(string)
		encoded, e := s.EncodeCursor(c)
		if e != nil {
			return nil, e
		}
		next = encoded
	}
	coverage, err := s.ConversationCoverage(ctx, ref)
	if err != nil {
		return nil, err
	}
	var reason any
	if len(values) == 0 {
		var exists int
		if err = s.DB.QueryRowContext(ctx, "SELECT count(*) FROM conversations WHERE conversation_type=? AND conversation_id=?", ref.Type, ref.ID).Scan(&exists); err != nil {
			return nil, err
		}
		reason = "no_records_in_period"
		if exists == 0 {
			reason = "unknown_conversation"
		} else if coverage["earliest_stored_at"] == nil {
			reason = "no_collected_data"
		}
	}
	var requestedSince, requestedUntil any
	if since != "" {
		requestedSince = since
	}
	if until != "" {
		requestedUntil = until
	}
	return map[string]any{"messages": values, "has_more": more, "next_cursor": next, "empty_reason": reason, "coverage": coverage, "snapshot_at": c.SnapshotAt, "coverage_observed_at": now(), "requested_since": requestedSince, "requested_until": requestedUntil}, nil
}
