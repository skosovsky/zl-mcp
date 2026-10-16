package storage

import (
	"context"
	"database/sql"
	"sort"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Resolve only exact, locally visible references at first insertion. A client
// ID is not a global ID and is never published without a unique local match.
func resolveActivityTargets(
	ctx context.Context,
	tx *sql.Tx,
	a domain.Activity,
) (domain.Activity, error) {
	if a.Reaction == nil || len(a.ReactionClientMessageIDs) == 0 {
		return a, nil
	}
	detail := *a.Reaction
	detail.TargetMessageIDs = append([]string{}, a.Reaction.TargetMessageIDs...)
	seen := map[string]bool{}
	for _, id := range detail.TargetMessageIDs {
		seen[id] = true
	}
	for _, clientID := range a.ReactionClientMessageIDs {
		var id sql.NullString
		var count, conflict int
		err := tx.QueryRowContext(ctx, `SELECT MIN(m.message_id),COUNT(DISTINCT m.message_id),
COALESCE(MAX(CASE WHEN p.client_message_id IS NOT NULL AND q.client_message_id IS NOT NULL AND p.client_message_id<>q.client_message_id THEN 1 ELSE 0 END),0)
FROM visible_messages m
LEFT JOIN message_client_ids p ON p.message_seq=m.seq
LEFT JOIN message_quote_metadata q ON q.message_seq=m.seq
WHERE m.conversation_type=? AND m.group_id=? AND (p.client_message_id=? OR q.client_message_id=?)`,
			a.ConversationType, a.ConversationID, clientID, clientID).Scan(&id, &count, &conflict)
		if err != nil {
			return a, err
		}
		if count == 1 && conflict == 0 && id.Valid && !seen[id.String] {
			detail.TargetMessageIDs = append(detail.TargetMessageIDs, id.String)
			seen[id.String] = true
		}
	}
	sort.Strings(detail.TargetMessageIDs)
	a.Reaction = &detail
	a.ReactionClientMessageIDs = nil
	return a, a.Validate()
}
