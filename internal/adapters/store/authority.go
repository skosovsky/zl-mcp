package store

import (
	"context"
	"os"
	"strconv"

	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
)

func (r *Reader) ReadIdentity(ctx context.Context) (shared.ReadIdentity, error) {
	account, revision, err := r.owner.ReadIdentity(ctx)
	return shared.ReadIdentity{Principal: "local:" + strconv.Itoa(os.Getuid()), Account: account, PolicyRevision: revision}, readError(err)
}
func (r *Reader) CheckGroupSearch(ctx context.Context, input search.GroupRequest) error {
	q := query(input.Query)
	q.GroupID = input.GroupID
	return readError(r.owner.CheckSearchAccess(ctx, q))
}
func (r *Reader) CheckConversationSearch(ctx context.Context, input search.ConversationRequest) error {
	q := query(input.Query)
	q.General = true
	q.ConversationType = input.Conversation.Type
	q.ConversationID = input.Conversation.ID
	return readError(r.owner.CheckSearchAccess(ctx, q))
}
func (r *Reader) CheckGroupContext(_ context.Context, input conversations.ContextRequest) error {
	return readError(r.owner.CheckContextAccess(input.Conversation, input.MessageID, input.Before, input.After, false))
}
func (r *Reader) CheckConversationContext(_ context.Context, input conversations.ContextRequest) error {
	return readError(r.owner.CheckContextAccess(input.Conversation, input.MessageID, input.Before, input.After, true))
}
