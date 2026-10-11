package messaging

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"time"
)

// CommandPort delegates to the existing send ledger and permission owner.
type CommandPort interface {
	Send(context.Context, domain.SendRequest) (domain.SendOperation, error)
}

func (h *Handler) Send(ctx context.Context, request domain.SendRequest) (domain.SendOperation, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	return h.Commands.Send(ctx, request)
}
