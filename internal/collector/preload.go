package collector

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ConversationPreload is optional and shares the existing authenticated session.
// The production catalogue loop merges metadata only; messages require explicit import.
func (g *sessionGuard) ConversationPreload(parent context.Context) (domain.PreloadSnapshot, error) {
	source, ok := g.upstream.(domain.ConversationPreloadSource)
	if !ok {
		return domain.PreloadSnapshot{}, domain.ErrConversationPreloadUnsupported
	}
	ctx, stop, err := g.request(parent)
	if err != nil {
		return domain.PreloadSnapshot{}, err
	}
	defer stop()
	page, err := source.ConversationPreload(ctx)
	return page, g.observe(err)
}
