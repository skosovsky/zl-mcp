// Package messaging exposes capabilities around the existing durable send owner.
package messaging

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// StatusPort cannot start a send or retry an unknown result.
type StatusPort interface {
	SendStatus(context.Context, string) (domain.SendOperation, error)
}

type Handler struct {
	Reads    StatusPort
	Commands CommandPort
}

func (h *Handler) Status(ctx context.Context, requestID string) (domain.SendOperation, error) {
	ctx, cancel := context.WithTimeout(ctx, handlerStatusTimeout)
	defer cancel()
	return h.Reads.SendStatus(ctx, requestID)
}
