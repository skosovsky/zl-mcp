package messaging

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// MessageActionPort keeps the durable write owner behind typed consumer requests.
// Reads and event processing never invoke this capability implicitly.
type MessageActionPort interface {
	MarkMessagesRead(context.Context, MarkReadRequest) (domain.MessageActionOperation, error)
	SetMessageReaction(context.Context, ReactionRequest) (domain.MessageActionOperation, error)
	MessageActionStatus(context.Context, string) (domain.MessageActionOperation, error)
}

type MarkReadRequest struct {
	RequestID        string   `json:"request_id"`
	ConversationType string   `json:"conversation_type"`
	ConversationID   string   `json:"conversation_id"`
	MessageIDs       []string `json:"message_ids"`
}

type ReactionRequest struct {
	RequestID        string `json:"request_id"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	MessageID        string `json:"message_id"`
	Action           string `json:"action"`
	Reaction         string `json:"reaction,omitempty"`
}

func (h *Handler) MarkMessagesRead(
	ctx context.Context,
	request MarkReadRequest,
) (domain.MessageActionOperation, error) {
	ctx, cancel := context.WithTimeout(ctx, handlerSendTimeout)
	defer cancel()
	return h.Actions.MarkMessagesRead(ctx, request)
}

func (h *Handler) SetMessageReaction(
	ctx context.Context,
	request ReactionRequest,
) (domain.MessageActionOperation, error) {
	// A replacement has two individually bounded native calls.
	ctx, cancel := context.WithTimeout(ctx, ReactionTimeout)
	defer cancel()
	return h.Actions.SetMessageReaction(ctx, request)
}
func (h *Handler) MessageActionStatus(ctx context.Context, id string) (domain.MessageActionOperation, error) {
	ctx, cancel := context.WithTimeout(ctx, handlerStatusTimeout)
	defer cancel()
	return h.Actions.MessageActionStatus(ctx, id)
}
