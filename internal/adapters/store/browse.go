package store

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

var _ conversations.DirectoryPort = (*Reader)(nil)

func (r *Reader) Conversations(ctx context.Context, q conversations.DirectoryRequest) (conversations.Directory, error) {
	return catalogValue[conversations.Directory](r.owner.Conversations(ctx, q.Type, q.Query, q.Limit, q.Cursor))
}
func (r *Reader) Conversation(ctx context.Context, ref domain.ConversationRef) (conversations.Detail, error) {
	return catalogValue[conversations.Detail](r.owner.Conversation(ctx, ref))
}
func (r *Reader) Browse(ctx context.Context, q conversations.BrowseRequest) (conversations.Browse, error) {
	return catalogValue[conversations.Browse](r.owner.Browse(ctx, q.Conversation, q.Since, q.Until, q.Order, q.Limit, q.Cursor))
}
