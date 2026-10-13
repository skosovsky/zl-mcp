// Package history exposes capabilities around the existing import owner.
package history

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// StatusPort cannot import or cancel history.
type StatusPort interface {
	ImportStatus(context.Context, string) (domain.HistoryImportStatus, error)
}

type Handler struct {
	Reads    StatusPort
	Commands CommandPort
}

func (h *Handler) Status(ctx context.Context, operationID string) (domain.HistoryImportStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, handlerStatusTimeout)
	defer cancel()
	return h.Reads.ImportStatus(ctx, operationID)
}
