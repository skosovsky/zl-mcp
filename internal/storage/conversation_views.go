package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"

	"github.com/skosovsky/zl-mcp/internal/domain"
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

// ConversationCoverageValue preserves native domain values for read adapters.
// The existing map view delegates to the same access check and SQL implementation.
func (s *Store) ConversationCoverageValue(ctx context.Context, ref domain.ConversationRef) (domain.Coverage, error) {
	if !ref.Valid() {
		return domain.Coverage{}, domain.Invalid("Typed conversation required.")
	}
	if !s.AllowsConversation(ref) {
		return domain.Coverage{}, subscriptionPermission("Conversation is outside collection policy.")
	}
	return s.conversationCoverage(ctx, ref)
}
func (s *Store) ConversationCoverage(ctx context.Context, ref domain.ConversationRef) (map[string]any, error) {
	c, err := s.ConversationCoverageValue(ctx, ref)
	if err != nil {
		return nil, err
	}
	return coverageView(c, ref), nil
}

func (s *Store) ConversationContext(
	ctx context.Context,
	ref domain.ConversationRef,
	id string,
	before, after int,
) (map[string]any, error) {
	result, err := s.context(ctx, ref, id, before, after, true)
	if err != nil {
		return nil, err
	}
	anchor, ok := result["anchor"].(domain.Message)
	if !ok {
		return nil, errors.New("invalid context anchor projection")
	}
	result["anchor"] = ConversationMessageView(anchor)
	for _, key := range []string{"before", "after"} {
		values := []map[string]any{}
		messages, ok := result[key].([]domain.Message)
		if !ok {
			return nil, errors.New("invalid context messages projection")
		}
		for _, m := range messages {
			values = append(values, ConversationMessageView(m))
		}
		result[key] = values
	}
	reply, ok := result["reply_to"].(*domain.Message)
	if !ok {
		return nil, errors.New("invalid context reply projection")
	}
	if r := reply; r != nil {
		result["reply_to"] = ConversationMessageView(*r)
	} else {
		result["reply_to"] = nil
	}
	coverage, ok := result["coverage"].(domain.Coverage)
	if !ok {
		return nil, errors.New("invalid context coverage projection")
	}
	result["coverage"] = coverageView(coverage, ref)
	if result["next_action"] != nil {
		result["next_action"] = domain.NextAction{
			Instruction: "Read omitted message IDs with zalo_get_conversation_message_context.",
		}
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
	return map[string]any{
		"conversation_count":            len(refs),
		"history_complete":              false,
		"conversations_with_known_gaps": gaps["direct"] + gaps["group"],
	}, nil
}

func (s *Store) Conversations(
	ctx context.Context,
	kind, query string,
	limit int,
	token string,
) (map[string]any, error) {
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
	rows, err := s.DB.QueryContext(ctx, directorySelect+" ORDER BY c.conversation_type,c.conversation_id")
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
		var aliasesJSON, friendship string
		var hasMessages bool
		if err = rows.Scan(
			&ref.Type,
			&ref.ID,
			&name,
			&source,
			&availability,
			&first,
			&updated,
			&aliasesJSON,
			&friendship,
			&hasMessages,
		); err != nil {
			//nolint:gosec,sqlclosecheck // G104: Close the read-only query immediately on error; later paths close before reuse of the connection.
			rows.Close()
			return nil, err
		}
		if !s.AllowsConversation(ref) {
			continue
		}
		discovered++
		sources[source] = true
		aliases, metadataSources, err := directoryMetadata(aliasesJSON, source)
		if err != nil {
			//nolint:gosec // G104: Read-only query cleanup; Scan/Rows.Err determines the result.
			rows.Close()
			return nil, err
		}
		for _, metadataSource := range metadataSources {
			sources[metadataSource] = true
		}
		if kind != "" && kind != ref.Type {
			continue
		}
		if query != "" && !directoryNameMatches(name, aliases, query) {
			continue
		}
		if ref.Type < c.Kind || (ref.Type == c.Kind && ref.ID <= c.ID) {
			continue
		}
		if len(values) <= limit {
			values = append(
				values,
				map[string]any{
					"conversation_type":   ref.Type,
					"conversation_id":     ref.ID,
					"name":                name,
					"metadata_source":     source,
					"availability":        availability,
					"first_discovered_at": first,
					"updated_at":          updated,
					"collection_enabled":  true,
					"aliases":             aliases,
					"friendship":          friendship,
					"has_stored_messages": hasMessages,
					"metadata_sources":    metadataSources,
				},
			)
		}
	}
	err = rows.Err()
	//nolint:gosec // G104: Read-only query cleanup; Scan/Rows.Err determines the result.
	rows.Close()
	if err != nil {
		return nil, err
	}
	var next, reason any
	more := len(values) > limit
	if more {
		values = values[:limit]
		last := values[len(values)-1]
		kind, kindOK := last["conversation_type"].(string)
		id, idOK := last["conversation_id"].(string)
		if !kindOK || !idOK {
			return nil, errors.New("invalid conversation cursor projection")
		}
		c.Kind, c.ID = kind, id
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
	return map[string]any{
		"conversations":    values,
		"has_more":         more,
		"next_cursor":      next,
		"catalog_complete": false,
		"catalog_sources":  ordered,
		"empty_reason":     reason,
	}, nil
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
	var aliasesJSON, friendship string
	var hasMessages bool
	err := s.DB.QueryRowContext(ctx, directorySelect+" WHERE c.conversation_type=? AND c.conversation_id=?", ref.Type, ref.ID).
		Scan(&ref.Type, &ref.ID, &name, &source, &availability, &first, &updated, &aliasesJSON, &friendship, &hasMessages)
	if err != nil {
		return nil, err
	}
	coverage, err := s.ConversationCoverage(ctx, ref)
	if err != nil {
		return nil, err
	}
	aliases, sources, err := directoryMetadata(aliasesJSON, source)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"conversation": map[string]any{
			"conversation_type":   ref.Type,
			"conversation_id":     ref.ID,
			"name":                name,
			"metadata_source":     source,
			"availability":        availability,
			"first_discovered_at": first,
			"updated_at":          updated,
			"collection_enabled":  true,
			"aliases":             aliases,
			"friendship":          friendship,
			"has_stored_messages": hasMessages,
			"metadata_sources":    sources,
		},
		"coverage":         coverage,
		"catalog_complete": false,
	}, nil
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
	return map[string]any{
		"mode":                mode,
		"conversation_counts": conversations,
		"message_counts":      messages,
		"catalog_complete":    false,
		"history_complete":    false,
	}, nil
}
