package membership

import (
	"context"
	"time"
)

type JoinRequest struct {
	PlanToken string `json:"plan_token"`
	RequestID string `json:"request_id"`
}

// CommandPort cannot mint a preview, approve a join or grant authority.
type CommandPort interface {
	Join(context.Context, JoinRequest) (JoinStatus, error)
}

func (h *Handler) Join(ctx context.Context, request JoinRequest) (JoinStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	return h.Commands.Join(ctx, request)
}
