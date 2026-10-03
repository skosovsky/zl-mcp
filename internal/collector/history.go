package collector

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// HistoryPage shares cancellation and authentication with the service's current
// session. Unsupported sources never fall back to a second login or listener.
func (g *sessionGuard) HistoryPage(parent context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
	source, ok := g.upstream.(domain.HistorySource)
	if !ok {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported
	}
	ctx, stop, err := g.request(parent)
	if err != nil {
		return domain.HistoryPage{}, err
	}
	defer stop()
	page, err := source.HistoryPage(ctx, ref, cursor, limit)
	return page, g.observe(err)
}
