package store

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (r *Reader) CoverageGaps(
	ctx context.Context,
	ref domain.ConversationRef,
	limit int,
	cursor string,
) (domain.CoverageGaps, error) {
	value, err := r.owner.CoverageGaps(ctx, ref, limit, cursor)
	return value, readError(err)
}
