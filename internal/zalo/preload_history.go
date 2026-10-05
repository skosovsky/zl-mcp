package zalo

import (
	"context"
	"errors"
	"sort"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (c *Client) PreloadHistoryPage(ctx context.Context, ref domain.ConversationRef, limit int) (domain.HistoryPage, error) {
	if !ref.Valid() || len(ref.ID) > 256 || limit < 1 || limit > 50 {
		return domain.HistoryPage{}, domain.Invalid("Invalid preload history request.")
	}
	snapshot, err := c.ConversationPreload(ctx)
	if errors.Is(err, domain.ErrConversationPreloadUnsupported) {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported
	}
	if err != nil {
		return domain.HistoryPage{}, err
	}
	return selectPreloadHistory(snapshot, ref, limit)
}

func selectPreloadHistory(snapshot domain.PreloadSnapshot, ref domain.ConversationRef, limit int) (domain.HistoryPage, error) {
	if (ref.Type == domain.ConversationDirect && !snapshot.DirectMessagesAvailable) || (ref.Type == domain.ConversationGroup && !snapshot.GroupMessagesAvailable) {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported
	}
	page := domain.HistoryPage{Messages: []domain.Message{}, LimitedSnapshot: true}
	for _, m := range snapshot.Messages {
		if m.Ref() == ref {
			page.Messages = append(page.Messages, m)
		}
	}
	sort.SliceStable(page.Messages, func(i, j int) bool {
		a, b := page.Messages[i], page.Messages[j]
		if a.SentAt.Equal(b.SentAt) {
			return a.ID < b.ID
		}
		return a.SentAt.After(b.SentAt)
	})
	if len(page.Messages) > limit {
		page.Messages = page.Messages[:limit]
	}
	return page, nil
}
