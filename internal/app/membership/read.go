// Package membership exposes read capabilities around the native join owner.
package membership

import (
	"context"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type Invite struct {
	GroupID          *string `json:"group_id"`
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	MemberCount      *int    `json:"member_count"`
	ApprovalRequired *bool   `json:"approval_required"`
	Membership       string  `json:"membership"`
	PreviewID        *string `json:"preview_id"`
	PreviewExpiresAt *string `json:"preview_expires_at"`
}

type JoinFailure struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	NextAction domain.NextAction `json:"next_action"`
}

type JoinStatus struct {
	Status            string       `json:"status"`
	OperationID       string       `json:"operation_id"`
	GroupID           *string      `json:"group_id"`
	CollectionEnabled bool         `json:"collection_enabled"`
	RequestID         string       `json:"request_id"`
	CheckedAt         string       `json:"checked_at"`
	RetrySafe         bool         `json:"retry_safe"`
	PollAfterMS       *int         `json:"poll_after_ms"`
	Error             *JoinFailure `json:"error"`
}

// ReadPort cannot approve or start a join.
type ReadPort interface {
	InspectInvite(context.Context, string) (Invite, error)
	JoinStatus(context.Context, string) (JoinStatus, error)
}

type Handler struct{ Reads ReadPort }

func (h *Handler) Inspect(ctx context.Context, inviteURL string) (Invite, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	return h.Reads.InspectInvite(ctx, inviteURL)
}

func (h *Handler) Status(ctx context.Context, operationID string) (JoinStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	return h.Reads.JoinStatus(ctx, operationID)
}
