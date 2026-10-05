package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"golang.org/x/text/unicode/norm"
)

func folded(v string) string { return strings.ToLower(norm.NFC.String(v)) }
func (s *Store) Groups(ctx context.Context, query string, limit int, token string) (map[string]any, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return nil, domain.Invalid("limit must be between 1 and 50")
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT group_id,name,member_count,updated_at FROM groups WHERE membership='member' ORDER BY group_id")
	if e != nil {
		return nil, e
	}
	groups := []domain.Group{}
	var updated *string
	for rows.Next() {
		var g domain.Group
		var at string
		if e = rows.Scan(&g.ID, &g.Name, &g.MemberCount, &at); e != nil {
			rows.Close()
			return nil, e
		}
		g.CollectionEnabled = s.Allowed(g.ID)
		if updated == nil || at > *updated {
			v := at
			updated = &v
		}
		if query == "" || strings.Contains(folded(g.Name), folded(query)) {
			groups = append(groups, g)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var catalog string
	e = s.DB.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='catalog_updated_at'").Scan(&catalog)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, &domain.Error{Code: "COLLECTOR_UNAVAILABLE", Message: "No group catalog has been collected yet.", Retryable: true, NextAction: domain.NextAction{Instruction: "Start zl-mcp collect after login."}, Details: map[string]any{}}
	}
	if e != nil {
		return nil, e
	}
	updated = &catalog
	c := struct {
		Query string
		Last  string
	}{Query: query}
	if token != "" {
		if e = s.DecodeCursor(token, &c); e != nil {
			return nil, e
		}
		if c.Query != query {
			return nil, domain.Invalid("Cursor belongs to another group query")
		}
	}
	start := sort.Search(len(groups), func(i int) bool { return groups[i].ID > c.Last })
	end := min(start+limit, len(groups))
	page := groups[start:end]
	var next *string
	if end < len(groups) {
		c.Last = page[len(page)-1].ID
		v, err := s.EncodeCursor(c)
		if err != nil {
			return nil, err
		}
		next = &v
	}
	var reason *string
	if len(page) == 0 {
		v := "no_matching_groups"
		if query == "" {
			v = "no_groups"
		}
		reason = &v
	}
	at, _ := time.Parse(time.RFC3339Nano, catalog)
	state, stateErr := s.State(ctx)
	if stateErr != nil {
		return nil, stateErr
	}
	return map[string]any{"groups": page, "has_more": next != nil, "next_cursor": next, "empty_reason": reason, "catalog_updated_at": updated, "stale": time.Since(at) > 6*time.Minute || state["collector_state"] != "connected"}, nil
}
func (s *Store) Group(ctx context.Context, id string) (map[string]any, error) {
	var g domain.Group
	var desc *string
	var at, member string
	e := s.DB.QueryRowContext(ctx, "SELECT group_id,name,description,member_count,updated_at,membership FROM groups WHERE group_id=?", id).Scan(&g.ID, &g.Name, &desc, &g.MemberCount, &at, &member)
	if e != nil {
		return nil, e
	}
	g.CollectionEnabled = s.Allowed(id)
	ts, _ := time.Parse(time.RFC3339Nano, at)
	return map[string]any{"group": g, "description": desc, "membership": member, "updated_at": at, "stale": time.Since(ts) > 6*time.Minute}, nil
}
func (s *Store) ReplaceCatalog(ctx context.Context, groups []domain.Group) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "UPDATE groups SET membership='unknown'"); e != nil {
		return e
	}
	at := now()
	if _, e = tx.ExecContext(ctx, "UPDATE conversations SET availability='unknown' WHERE conversation_type='group'"); e != nil {
		return e
	}
	for _, g := range groups {
		if _, e = tx.ExecContext(ctx, `INSERT INTO conversations(conversation_type,conversation_id,name,metadata_source,availability,first_discovered_at,updated_at) VALUES('group',?,?,'group_catalog','member',?,?) ON CONFLICT(conversation_type,conversation_id) DO UPDATE SET name=excluded.name,metadata_source='group_catalog',availability='member',updated_at=excluded.updated_at`, g.ID, g.Name, at, at); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO groups(group_id,name,member_count,updated_at) VALUES(?,?,?,?) ON CONFLICT(group_id) DO UPDATE SET name=excluded.name,member_count=excluded.member_count,updated_at=excluded.updated_at,membership='member'`, g.ID, g.Name, g.MemberCount, at); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO metadata VALUES('catalog_updated_at',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", at); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Coverage(ctx context.Context, id string) (domain.Coverage, error) {
	return s.conversationCoverage(ctx, domain.ConversationRef{Type: domain.ConversationGroup, ID: id})
}
func (s *Store) conversationCoverage(ctx context.Context, ref domain.ConversationRef) (domain.Coverage, error) {
	id := ref.ID
	c := domain.Coverage{GroupID: id, KnownGaps: []domain.Gap{}}
	if s.retention > 0 {
		v := s.retention
		c.RetentionDays = &v
	}
	var start sql.NullString
	e := s.DB.QueryRowContext(ctx, "SELECT started_at FROM collection_started WHERE conversation_type=? AND group_id=?", ref.Type, id).Scan(&start)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return c, e
	}
	parse := func(v sql.NullString) *time.Time {
		if !v.Valid {
			return nil
		}
		at, err := time.Parse(time.RFC3339Nano, v.String)
		if err != nil {
			return nil
		}
		return &at
	}
	c.CollectionStartedAt = parse(start)
	var earliest, latest sql.NullString
	if e = s.DB.QueryRowContext(ctx, "SELECT min(sent_at),max(sent_at) FROM visible_messages WHERE conversation_type=? AND group_id=?", ref.Type, id).Scan(&earliest, &latest); e != nil {
		return c, e
	}
	c.EarliestStoredAt = parse(earliest)
	c.LatestStoredAt = parse(latest)
	rows, e := s.DB.QueryContext(ctx, "SELECT started_at,ended_at,reason FROM collection_gaps WHERE conversation_type=? AND group_id=? ORDER BY started_at", ref.Type, id)
	if e != nil {
		return c, e
	}
	defer rows.Close()
	for rows.Next() {
		var from string
		var to sql.NullString
		var g domain.Gap
		if e = rows.Scan(&from, &to, &g.Reason); e != nil {
			return c, e
		}
		g.From, e = time.Parse(time.RFC3339Nano, from)
		if e != nil {
			return c, e
		}
		g.To = parse(to)
		c.KnownGaps = append(c.KnownGaps, g)
	}
	return c, rows.Err()
}
func (s *Store) Summary(ctx context.Context) (map[string]any, error) {
	refs, err := s.collectionRefs(ctx)
	if err != nil {
		return nil, err
	}
	gaps, err := s.conversationGapCounts(ctx)
	if err != nil {
		return nil, err
	}
	groups := 0
	for _, ref := range refs {
		if ref.Type != domain.ConversationGroup {
			continue
		}
		groups++
	}
	return map[string]any{"group_count": groups, "history_complete": false, "groups_with_known_gaps": gaps["group"]}, nil
}
func display(m domain.Message) domain.Message { return clip(m, 8000) }
func clip(m domain.Message, limit int) domain.Message {
	if utf8.RuneCountInString(m.Text) > limit {
		m.Text = string([]rune(m.Text)[:limit])
		m.TextTruncated = true
		v := "zalo://groups/" + url.PathEscape(m.GroupID) + "/messages/" + url.PathEscape(m.ID)
		if m.Conversation.Valid() {
			v = ConversationMessageURI(m.Conversation, m.ID)
		}
		m.TextResourceURI = &v
	}
	return m
}
func (s *Store) Context(ctx context.Context, g, id string, before, after int) (map[string]any, error) {
	return s.context(ctx, domain.ConversationRef{Type: domain.ConversationGroup, ID: g}, id, before, after, false)
}
func (s *Store) context(ctx context.Context, ref domain.ConversationRef, id string, before, after int, general bool) (map[string]any, error) {
	if before < 0 || before > 20 || after < 0 || after > 20 {
		return nil, domain.Invalid("Context window must be between 0 and 20.")
	}
	read := func(id string) (domain.Message, error) {
		if general {
			return s.ConversationMessage(ctx, ref, id)
		}
		return s.Message(ctx, ref.ID, id)
	}
	anchor, e := read(id)
	if e != nil {
		return nil, e
	}
	get := func(operator, direction string, limit int) ([]domain.Message, error) {
		rows, e := s.DB.QueryContext(ctx, "SELECT "+messageColumns+" FROM visible_messages WHERE conversation_type=? AND group_id=? AND (sent_at "+operator+" ? OR (sent_at=? AND message_id "+operator+" ?)) ORDER BY sent_at "+direction+",message_id "+direction+" LIMIT ?", ref.Type, ref.ID, anchor.SentAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), anchor.SentAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), id, limit)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		msgs := []domain.Message{}
		for rows.Next() {
			m, e := scanMessage(rows)
			if e != nil {
				return nil, e
			}
			if general {
				m.Conversation = ref
				if ref.Type == domain.ConversationDirect {
					m.GroupID = ""
				}
			}
			msgs = append(msgs, display(m))
		}
		return msgs, rows.Err()
	}
	pre, e := get("<", "DESC", before)
	if e != nil {
		return nil, e
	}
	post, e := get(">", "ASC", after)
	if e != nil {
		return nil, e
	}
	for i, j := 0, len(pre)-1; i < j; i, j = i+1, j-1 {
		pre[i], pre[j] = pre[j], pre[i]
	}
	var reply *domain.Message
	if anchor.ReplyTo != nil {
		r, e := read(*anchor.ReplyTo)
		if e == nil {
			v := display(r)
			reply = &v
		} else if !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
	}
	coverage, e := s.conversationCoverage(ctx, ref)
	if e != nil {
		return nil, e
	}
	result := map[string]any{"anchor": display(anchor), "before": pre, "after": post, "reply_to": reply, "coverage": coverage, "context_truncated": false, "omitted_before_id": nil, "omitted_after_id": nil, "next_action": nil}
	for {
		b, e := json.Marshal(result)
		if e != nil {
			return nil, e
		}
		if len(b) <= 20<<10 {
			return result, nil
		}
		if len(pre) == 0 && len(post) == 0 {
			anchorValue := result["anchor"].(domain.Message)
			if utf8.RuneCountInString(anchorValue.Text) > 1000 {
				result["anchor"] = clip(anchorValue, max(1000, utf8.RuneCountInString(anchorValue.Text)/2))
				continue
			}
			if reply != nil && utf8.RuneCountInString(reply.Text) > 1000 {
				v := clip(*reply, max(1000, utf8.RuneCountInString(reply.Text)/2))
				reply = &v
				result["reply_to"] = reply
				continue
			}
			return nil, domain.ResponseTooLarge("Context metadata exceeds the response budget.", "Report that context cannot fit. Read full message text only through a resource URI previously returned by the server; do not claim complete coverage.")
		}
		result["context_truncated"] = true
		result["next_action"] = domain.NextAction{Instruction: "Read omitted message IDs with zalo_get_message_context."}
		if len(post) >= len(pre) && len(post) > 0 {
			result["omitted_after_id"] = post[len(post)-1].ID
			post = post[:len(post)-1]
		} else {
			result["omitted_before_id"] = pre[0].ID
			pre = pre[1:]
		}
		result["before"] = pre
		result["after"] = post
	}
}
func (s *Store) SetState(ctx context.Context, state map[string]any) error {
	b, e := json.Marshal(state)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, "INSERT INTO collector_state VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,heartbeat=excluded.heartbeat", string(b), now())
	return e
}

// TouchHeartbeat updates liveness without overwriting collector transitions.
func (s *Store) TouchHeartbeat(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE collector_state SET heartbeat=? WHERE id=1", now())
	return err
}

func (s *Store) State(ctx context.Context) (map[string]any, error) {
	var b, heartbeat string
	e := s.DB.QueryRowContext(ctx, "SELECT payload,heartbeat FROM collector_state WHERE id=1").Scan(&b, &heartbeat)
	state := map[string]any{}
	if e == nil {
		e = json.Unmarshal([]byte(b), &state)
		if e != nil {
			return nil, e
		}
		at, err := time.Parse(time.RFC3339Nano, heartbeat)
		if err != nil {
			return nil, err
		}
		if time.Since(at) > 30*time.Second && state["collector_state"] != "auth_required" {
			state["collector_state"] = "stopped"
		}
	} else if errors.Is(e, sql.ErrNoRows) {
		state = map[string]any{"authenticated": false, "collector_state": "stopped", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": nil}
	} else {
		return nil, e
	}
	collection, err := s.CollectionStatus(ctx)
	if err != nil {
		return nil, err
	}
	counts := collection["message_counts"].(map[string]int)
	state["enabled_group_count"] = collection["conversation_counts"].(map[string]int)["group"]
	state["stored_message_count"] = counts["direct"] + counts["group"]
	summary, e := s.Summary(ctx)
	state["coverage_summary"] = summary
	return state, e
}
func (s *Store) BeginGap(ctx context.Context, reason string) error {
	from := now()
	if reason == "collector_start_or_restart" {
		var previous string
		err := s.DB.QueryRowContext(ctx, "SELECT heartbeat FROM collector_state WHERE id=1").Scan(&previous)
		if err == nil {
			from = previous
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	refs, e := s.collectionRefs(ctx)
	if e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, ref := range refs {
		if _, e = tx.ExecContext(ctx, `INSERT INTO collection_gaps(group_id,conversation_type,started_at,reason) SELECT ?,?,?,? WHERE NOT EXISTS(SELECT 1 FROM collection_gaps WHERE conversation_type=? AND conversation_id=? AND ended_at IS NULL)`, ref.ID, ref.Type, from, reason, ref.Type, ref.ID); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) EndGaps(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, "UPDATE collection_gaps SET ended_at=? WHERE ended_at IS NULL", now())
	return e
}
