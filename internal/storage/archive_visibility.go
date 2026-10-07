package storage

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ArchiveMessageSuppressed checks known local visibility without writing any history state.
// A missing global ID cannot be linked to a tombstone; callers report this limitation.
func (s *Store) ArchiveMessageSuppressed(ctx context.Context, ref domain.ConversationRef, id string, nowMS int64) (bool, error) {
	if !s.AllowsConversation(ref) {
		return false, subscriptionPermission("Conversation is outside collection policy.")
	}
	if id == "" {
		return false, nil
	}
	var suppressed bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM message_tombstones WHERE conversation_type=? AND conversation_id=? AND message_id=?) OR EXISTS(SELECT 1 FROM history_message_expiry WHERE conversation_type=? AND conversation_id=? AND message_id=? AND (expired=1 OR expires_ms<=?))`, ref.Type, ref.ID, id, ref.Type, ref.ID, id, nowMS).Scan(&suppressed)
	return suppressed, err
}
