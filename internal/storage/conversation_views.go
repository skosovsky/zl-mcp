package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"net/url"
	"sort"
	"strings"
)

func ConversationMessageURI(ref domain.ConversationRef, id string) string {
	return "zalo://conversations/" + ref.Type + "/" + url.PathEscape(ref.ID) + "/messages/" + url.PathEscape(id)
}
func ConversationMessageView(m domain.Message) map[string]any {
	b, _ := json.Marshal(m)
	v := map[string]any{}
	_ = json.Unmarshal(b, &v)
	delete(v, "group_id")
	v["conversation_type"] = m.Ref().Type
	v["conversation_id"] = m.Ref().ID
	return v
}
func coverageView(c domain.Coverage, ref domain.ConversationRef) map[string]any {
	b, _ := json.Marshal(c)
	v := map[string]any{}
	_ = json.Unmarshal(b, &v)
	delete(v, "group_id")
	v["conversation_type"] = ref.Type
	v["conversation_id"] = ref.ID
	return v
}
func (s *Store) ConversationCoverage(ctx context.Context, ref domain.ConversationRef) (map[string]any, error) {
	if !ref.Valid() {
		return nil, domain.Invalid("Typed conversation required.")
	}
	if !s.AllowsConversation(ref) {
		return nil, subscriptionPermission("Conversation is outside collection policy.")
	}
	c, err := s.conversationCoverage(ctx, ref)
	if err != nil {
		return nil, err
	}
	return coverageView(c, ref), nil
}
func (s *Store) ConversationContext(ctx context.Context, ref domain.ConversationRef, id string, before, after int) (map[string]any, error) {
	result, err := s.context(ctx, ref, id, before, after, true)
	if err != nil {
		return nil, err
	}
	result["anchor"] = ConversationMessageView(result["anchor"].(domain.Message))
	for _, key := range []string{"before", "after"} {
		values := []map[string]any{}
		for _, m := range result[key].([]domain.Message) {
			values = append(values, ConversationMessageView(m))
		}
		result[key] = values
	}
	if r := result["reply_to"].(*domain.Message); r != nil {
		result["reply_to"] = ConversationMessageView(*r)
	} else {
		result["reply_to"] = nil
	}
	result["coverage"] = coverageView(result["coverage"].(domain.Coverage), ref)
	if result["next_action"] != nil {
		result["next_action"] = domain.NextAction{Instruction: "Read omitted message IDs with zalo_get_conversation_message_context."}
	}
	return result, nil
}
func (s *Store) ConversationSummary(ctx context.Context) (map[string]any, error) {
	refs, err := s.collectionRefs(ctx)
	if err != nil {
		return nil, err
	}
	gaps, err := s.conversationGapCounts(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"conversation_count": len(refs), "history_complete": false, "conversations_with_known_gaps": gaps["direct"] + gaps["group"]}, nil
}
func (s *Store) ConversationSearchResult(ctx context.Context, q Search, p *SearchPage) (map[string]any, error) {
	hits := []map[string]any{}
	coverage := []map[string]any{}
	seen := map[domain.ConversationRef]bool{}
	for _, h := range p.Hits {
		b, err := json.Marshal(h)
		if err != nil {
			return nil, err
		}
		v := map[string]any{}
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		delete(v, "group_id")
		delete(v, "group_name")
		v["conversation_type"] = h.Conversation.Type
		v["conversation_id"] = h.Conversation.ID
		v["conversation_name"] = h.GroupName
		hits = append(hits, v)
		if !seen[h.Conversation] {
			c, err := s.ConversationCoverage(ctx, h.Conversation)
			if err != nil {
				return nil, err
			}
			coverage = append(coverage, c)
			seen[h.Conversation] = true
		}
	}
	var reason any
	if len(hits) == 0 {
		reason = "no_matches"
		if q.ConversationID != "" {
			ref := domain.ConversationRef{Type: q.ConversationType, ID: q.ConversationID}
			c, err := s.ConversationCoverage(ctx, ref)
			if err != nil {
				return nil, err
			}
			coverage = append(coverage, c)
			if c["earliest_stored_at"] == nil {
				reason = "no_collected_data"
			}
		} else {
			state, err := s.State(ctx)
			if err != nil {
				return nil, err
			}
			if state["stored_message_count"] == 0 {
				reason = "no_collected_data"
			}
		}
	}
	summary, err := s.ConversationSummary(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"messages": hits, "has_more": p.NextCursor != nil, "next_cursor": p.NextCursor, "empty_reason": reason, "coverage": coverage, "coverage_summary": summary, "snapshot_at": p.SnapshotAt}, nil
}

func (s *Store) Conversations(ctx context.Context, kind, query string, limit int, token string) (map[string]any, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 || (kind != "" && kind != "group" && kind != "direct") {
		return nil, domain.Invalid("Invalid conversation filter or page limit.")
	}
	h := sha256.Sum256([]byte(kind + "\x00" + query))
	fingerprint := hex.EncodeToString(h[:])
	c := struct{ Fingerprint, Kind, ID string }{Fingerprint: fingerprint}
	if token != "" {
		if err := s.DecodeCursor(token, &c); err != nil {
			return nil, err
		}
		if c.Fingerprint != fingerprint {
			return nil, domain.Invalid("Cursor filters differ from the original query.")
		}
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT conversation_type,conversation_id,name,metadata_source,availability,first_discovered_at,updated_at FROM conversations ORDER BY conversation_type,conversation_id")
	if err != nil {
		return nil, err
	}
	values := []map[string]any{}
	sources := map[string]bool{}
	discovered := 0
	for rows.Next() {
		var ref domain.ConversationRef
		var name *string
		var source, availability, first, updated string
		if err = rows.Scan(&ref.Type, &ref.ID, &name, &source, &availability, &first, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		if !s.AllowsConversation(ref) {
			continue
		}
		discovered++
		sources[source] = true
		if kind != "" && kind != ref.Type {
			continue
		}
		if query != "" && (name == nil || !strings.Contains(folded(*name), folded(query))) {
			continue
		}
		if ref.Type < c.Kind || (ref.Type == c.Kind && ref.ID <= c.ID) {
			continue
		}
		if len(values) <= limit {
			values = append(values, map[string]any{"conversation_type": ref.Type, "conversation_id": ref.ID, "name": name, "metadata_source": source, "availability": availability, "first_discovered_at": first, "updated_at": updated, "collection_enabled": true})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var next, reason any
	more := len(values) > limit
	if more {
		values = values[:limit]
		last := values[len(values)-1]
		c.Kind = last["conversation_type"].(string)
		c.ID = last["conversation_id"].(string)
		t, err := s.EncodeCursor(c)
		if err != nil {
			return nil, err
		}
		next = t
	}
	if len(values) == 0 {
		reason = "no_matching_conversations"
		if discovered == 0 {
			reason = "no_discovered_conversations"
		}
	}
	ordered := []string{}
	for source := range sources {
		ordered = append(ordered, source)
	}
	sort.Strings(ordered)
	return map[string]any{"conversations": values, "has_more": more, "next_cursor": next, "catalog_complete": false, "catalog_sources": ordered, "empty_reason": reason}, nil
}
func (s *Store) Conversation(ctx context.Context, ref domain.ConversationRef) (map[string]any, error) {
	if !ref.Valid() {
		return nil, domain.Invalid("Typed conversation required.")
	}
	if !s.AllowsConversation(ref) {
		return nil, subscriptionPermission("Conversation is outside collection policy.")
	}
	var name *string
	var source, availability, first, updated string
	err := s.DB.QueryRowContext(ctx, "SELECT name,metadata_source,availability,first_discovered_at,updated_at FROM conversations WHERE conversation_type=? AND conversation_id=?", ref.Type, ref.ID).Scan(&name, &source, &availability, &first, &updated)
	if err != nil {
		return nil, err
	}
	coverage, err := s.ConversationCoverage(ctx, ref)
	if err != nil {
		return nil, err
	}
	return map[string]any{"conversation": map[string]any{"conversation_type": ref.Type, "conversation_id": ref.ID, "name": name, "metadata_source": source, "availability": availability, "first_discovered_at": first, "updated_at": updated, "collection_enabled": true}, "coverage": coverage, "catalog_complete": false}, nil
}

func (s *Store) CollectionStatus(ctx context.Context) (map[string]any, error) {
	refs, err := s.collectionRefs(ctx)
	if err != nil {
		return nil, err
	}
	conversations := map[string]int{"direct": 0, "group": 0}
	messages, err := s.conversationMessageCounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		conversations[ref.Type]++
	}
	mode := "selected"
	if s.policy.All {
		mode = "all"
	}
	return map[string]any{"mode": mode, "conversation_counts": conversations, "message_counts": messages, "catalog_complete": false, "history_complete": false}, nil
}
