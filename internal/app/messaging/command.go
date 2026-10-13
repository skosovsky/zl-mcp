package messaging

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// CommandPort delegates to the existing send ledger and permission owner.
type CommandPort interface {
	Send(context.Context, domain.SendRequest) (domain.SendOperation, error)
}

func (h *Handler) Send(ctx context.Context, request domain.SendRequest) (domain.SendOperation, error) {
	ctx, cancel := context.WithTimeout(ctx, handlerSendTimeout)
	defer cancel()
	return h.Commands.Send(ctx, request)
}
