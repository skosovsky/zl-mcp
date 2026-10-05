package storage

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// PutPreloadEntries validates a whole observed page before one atomic metadata
// merge. It changes no messages, novelty facts, subscriptions or send permission.
func (s *Store) PutPreloadEntries(ctx context.Context, entries []domain.PreloadEntry) (int, error) {
	if len(entries) > 5000 {
		return 0, domain.Invalid("Preload directory page exceeds bounds.")
	}
	seen := map[domain.ConversationRef]bool{}
	for _, entry := range entries {
		ref := entry.Conversation
		if !ref.Valid() || ref.ID == "0" || len(ref.ID) > 256 || !utf8.ValidString(ref.ID) || strings.TrimSpace(ref.ID) != ref.ID || seen[ref] || (entry.Name != nil && (len(*entry.Name) > 4096 || !utf8.ValidString(*entry.Name))) {
			return 0, domain.Invalid("Invalid preload directory metadata.")
		}
		seen[ref] = true
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = historyAccount(ctx, tx); err != nil {
		return 0, err
	}
	count := 0
	stamp := now()
	for _, entry := range entries {
		ref := entry.Conversation
		if !s.AllowsConversation(ref) {
			continue
		}
		var name any
		if entry.Name != nil && *entry.Name != "" {
			name = *entry.Name
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversations VALUES(?,?,?,'preload_catalog','unknown',?,?) ON CONFLICT(conversation_type,conversation_id) DO UPDATE SET name=COALESCE(conversations.name,excluded.name),updated_at=excluded.updated_at`, ref.Type, ref.ID, name, stamp, stamp); err != nil {
			return 0, err
		}
		count++
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}
