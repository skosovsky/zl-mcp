package operations

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

var _ messaging.MessageActionPort = (*Reader)(nil)

func (r *Reader) MarkMessagesRead(
	ctx context.Context,
	request messaging.MarkReadRequest,
) (domain.MessageActionOperation, error) {
	return decode[domain.MessageActionOperation](r.owner.Call(ctx, "zalo_mark_messages_read", request))
}

func (r *Reader) SetMessageReaction(
	ctx context.Context,
	request messaging.ReactionRequest,
) (domain.MessageActionOperation, error) {
	return decode[domain.MessageActionOperation](r.owner.Call(ctx, "zalo_set_message_reaction", request))
}
func (r *Reader) MessageActionStatus(ctx context.Context, id string) (domain.MessageActionOperation, error) {
	return decode[domain.MessageActionOperation](r.owner.Call(ctx, "zalo_get_message_action_status", struct {
		RequestID string `json:"request_id"`
	}{id}))
}
