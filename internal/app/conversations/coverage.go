package conversations

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// CoveragePort is separate from existing read ports, preserving narrow consumers.
type CoveragePort interface {
	CoverageGaps(context.Context, domain.ConversationRef, int, string) (domain.CoverageGaps, error)
}

func (h *Handler) CoverageGaps(
	ctx context.Context,
	ref domain.ConversationRef,
	limit int,
	cursor string,
) (domain.CoverageGaps, error) {
	port, ok := h.Reads.(CoveragePort)
	if !ok {
		return domain.CoverageGaps{}, domain.Invalid("Coverage continuation is unavailable.")
	}
	return port.CoverageGaps(ctx, ref, limit, cursor)
}
