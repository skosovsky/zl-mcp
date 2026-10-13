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
func (s *Store) Browse(
	ctx context.Context,
	ref domain.ConversationRef,
	since, until, order string,
	limit int,
	token string,
) (map[string]any, error) {
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
	bounds, boundsErr := browseBounds(since, until)
	if boundsErr != nil {
		return nil, boundsErr
	}
	filter, _ := json.Marshal([]string{"browse", ref.Type, ref.ID, since, until, order})
	sum := sha256.Sum256(filter)
	fingerprint := hex.EncodeToString(sum[:])
	c, cursorErr := s.browseCursor(ctx, fingerprint, token)
	if cursorErr != nil {
		return nil, cursorErr
	}
	clauses, args := browsePredicates(ref, bounds, c, order, limit)
	rows, err := s.DB.QueryContext(
		ctx,
		//nolint:gosec // G202: SQL fragments are fixed internal predicates/operators; all client values use placeholders.
		`SELECT m.message_id,m.sender_id,m.sender_name,g.name,m.sent_at,m.text FROM visible_messages m LEFT JOIN conversations g ON g.conversation_type=m.conversation_type AND g.conversation_id=m.group_id WHERE `+strings.Join(
			clauses,
			" AND ",
		)+" ORDER BY m.sent_at "+order+",m.message_id "+order+" LIMIT ?",
		args...)
	if err != nil {
		return nil, err
	}
	records, scanErr := scanBrowseRows(rows, ref)
	if scanErr != nil {
		return nil, scanErr
	}
	values, times, senders := records.values, records.times, records.senders
	if directionErr := s.setBrowseDirections(ctx, ref, values, senders); directionErr != nil {
		return nil, directionErr
	}
	values, next, more, pageErr := s.pageBrowseValues(values, times, c, limit)
	if pageErr != nil {
		return nil, pageErr
	}
	coverage, err := s.ConversationCoverage(ctx, ref)
	if err != nil {
		return nil, err
	}
	var reason any
	if len(values) == 0 {
		reason, err = s.browseEmptyReason(ctx, ref, coverage)
		if err != nil {
			return nil, err
		}
	}
	var requestedSince, requestedUntil any
	if since != "" {
		requestedSince = since
	}
	if until != "" {
		requestedUntil = until
	}
	return map[string]any{
		"messages":             values,
		"has_more":             more,
		"next_cursor":          next,
		"empty_reason":         reason,
		"coverage":             coverage,
		"snapshot_at":          c.SnapshotAt,
		"coverage_observed_at": now(),
		"requested_since":      requestedSince,
		"requested_until":      requestedUntil,
	}, nil
}

func browseBounds(since, until string) ([2]time.Time, error) {
	var bounds [2]time.Time
	for i, value := range []string{since, until} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return bounds, domain.Invalid("since/until must be RFC3339 instants, e.g. 2026-10-01T00:00:00Z.")
		}
		bounds[i] = parsed
	}
	if !bounds[0].IsZero() && !bounds[1].IsZero() && !bounds[0].Before(bounds[1]) {
		return bounds, domain.Invalid("since must precede until.")
	}
	return bounds, nil
}

func browsePredicates(ref domain.ConversationRef, bounds [2]time.Time, c cursor, order string, limit int) ([]string, []any) {
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
	return clauses, args
}

type browseRecords struct {
	values         []map[string]any
	times, senders []string
}

func scanBrowseRows(rows *sql.Rows, ref domain.ConversationRef) (browseRecords, error) {
	values := []map[string]any{}
	times := []string{}
	senders := []string{}
	for rows.Next() {
		var id, sender, stamp, body string
		var senderName, name *string
		if err := rows.Scan(&id, &sender, &senderName, &name, &stamp, &body); err != nil {
			//nolint:gosec // G104: Close the read-only query immediately on error; later paths close before reuse of the connection.
			rows.Close()
			return browseRecords{}, err
		}
		runes := []rune(body)
		truncated := len(runes) > 150
		var uri any
		if truncated {
			runes = runes[:150]
			uri = ConversationMessageURI(ref, id)
		}
		values = append(
			values,
			map[string]any{
				"conversation_type": ref.Type,
				"conversation_id":   ref.ID,
				"conversation_name": name,
				"message_id":        id,
				"sender_id":         sender,
				"sender_name":       senderName,
				"sent_at":           stamp,
				"excerpt":           string(runes),
				"text_truncated":    truncated,
				"text_resource_uri": uri,
				"direction":         "unknown",
			},
		)
		times = append(times, stamp)
		senders = append(senders, sender)
	}
	err := rows.Err()
	//nolint:gosec // G104: Read-only query cleanup; Scan/Rows.Err determines the result.
	rows.Close()
	if err != nil {
		return browseRecords{}, err
	}
	return browseRecords{values: values, times: times, senders: senders}, nil
}

func (s *Store) browseEmptyReason(ctx context.Context, ref domain.ConversationRef, coverage map[string]any) (any, error) {
	var reason any
	var exists int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM conversations WHERE conversation_type=? AND conversation_id=?", ref.Type, ref.ID).
		Scan(&exists); err != nil {
		return nil, err
	}
	reason = "no_records_in_period"
	if exists == 0 {
		reason = "unknown_conversation"
	} else if coverage["earliest_stored_at"] == nil {
		reason = "no_collected_data"
	}

	return reason, nil
}

func (s *Store) browseCursor(ctx context.Context, fingerprint, token string) (cursor, error) {
	c := cursor{Fingerprint: fingerprint, SnapshotAt: now()}
	if token != "" {
		if err := s.DecodeCursor(token, &c); err != nil {
			return c, err
		}
		if c.Fingerprint != fingerprint {
			return c, domain.Invalid("Cursor filters or order differ from the original browse.")
		}
	} else if err := s.DB.QueryRowContext(ctx, "SELECT coalesce(max(seq),0) FROM messages").Scan(&c.Snapshot); err != nil {
		return c, err
	}
	return c, nil
}

func (s *Store) setBrowseDirections(ctx context.Context, ref domain.ConversationRef, values []map[string]any, senders []string) error {
	var ownHash string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='account'").Scan(&ownHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	for i, v := range values {
		hash := sha256.Sum256([]byte(senders[i]))
		if ownHash != "" && hex.EncodeToString(hash[:]) == ownHash {
			v["direction"] = "outgoing"
		} else if (ref.Type == domain.ConversationDirect && senders[i] == ref.ID) || (ref.Type == domain.ConversationGroup && ownHash != "") {
			v["direction"] = "incoming"
		}
	}
	return nil
}

func (s *Store) pageBrowseValues(values []map[string]any, times []string, c cursor, limit int) ([]map[string]any, any, bool, error) {
	var next any
	more := len(values) > limit
	if more {
		values = values[:limit]
		c.At = times[limit-1]
		messageID, ok := values[limit-1]["message_id"].(string)
		if !ok {
			return nil, nil, false, errors.New("invalid message cursor projection")
		}
		c.ID = messageID
		encoded, e := s.EncodeCursor(c)
		if e != nil {
			return nil, nil, false, e
		}
		next = encoded
	}
	return values, next, more, nil
}
