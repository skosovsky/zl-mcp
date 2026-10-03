package storage

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) AllowsConversation(ref domain.ConversationRef) bool { return s.policy.Allows(ref) }

// Aggregate once rather than issuing one SQLite query per discovered peer.
func (s *Store) conversationMessageCounts(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{"direct": 0, "group": 0}
	query := "SELECT conversation_type,conversation_id,COUNT(*) FROM messages GROUP BY conversation_type,conversation_id"
	if s.policy.All {
		query = "SELECT conversation_type,'',COUNT(*) FROM messages GROUP BY conversation_type"
	}
	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ref domain.ConversationRef
		var n int
		if err := rows.Scan(&ref.Type, &ref.ID, &n); err != nil {
			return nil, err
		}
		if s.policy.All || s.AllowsConversation(ref) {
			counts[ref.Type] += n
		}
	}
	return counts, rows.Err()
}

func (s *Store) conversationGapCounts(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{"direct": 0, "group": 0}
	query := "SELECT DISTINCT conversation_type,conversation_id FROM collection_gaps"
	if s.policy.All {
		query += " WHERE EXISTS(SELECT 1 FROM conversations c WHERE c.conversation_type=collection_gaps.conversation_type AND c.conversation_id=collection_gaps.conversation_id)"
	}
	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ref domain.ConversationRef
		if err := rows.Scan(&ref.Type, &ref.ID); err != nil {
			return nil, err
		}
		if s.AllowsConversation(ref) {
			counts[ref.Type]++
		}
	}
	return counts, rows.Err()
}

func (s *Store) collectionRefs(ctx context.Context) ([]domain.ConversationRef, error) {
	seen := map[domain.ConversationRef]bool{}
	for ref, enabled := range s.policy.Selected {
		if enabled {
			seen[ref] = true
		}
	}
	if s.policy.All {
		rows, err := s.DB.QueryContext(ctx, "SELECT conversation_type,conversation_id FROM conversations")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var ref domain.ConversationRef
			if err = rows.Scan(&ref.Type, &ref.ID); err != nil {
				return nil, err
			}
			seen[ref] = true
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	refs := make([]domain.ConversationRef, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	return refs, nil
}

func (s *Store) ConversationMessage(ctx context.Context, ref domain.ConversationRef, id string) (domain.Message, error) {
	if !ref.Valid() || id == "" {
		return domain.Message{}, domain.Invalid("Typed conversation and message ID are required.")
	}
	if !s.AllowsConversation(ref) {
		return domain.Message{}, subscriptionPermission("Conversation is outside the collection policy.")
	}
	m, err := scanMessage(s.DB.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM messages WHERE conversation_type=? AND conversation_id=? AND message_id=?", ref.Type, ref.ID, id))
	if err != nil {
		return m, err
	}
	m.Conversation = ref
	if ref.Type == domain.ConversationDirect {
		m.GroupID = ""
	}
	return m, nil
}

func (s *Store) DeleteConversation(ctx context.Context, ref domain.ConversationRef, id string) error {
	if !ref.Valid() || id == "" {
		return domain.Invalid("Typed conversation and message ID are required.")
	}
	if !s.AllowsConversation(ref) {
		return subscriptionPermission("Conversation is outside the collection policy.")
	}
	return s.removeMessages(ctx, "conversation_type=? AND conversation_id=? AND message_id=?", "record_deleted", ref.Type, ref.ID, id)
}
