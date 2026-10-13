package store

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
)

var _ eventaccess.JournalPort = (*Reader)(nil)

func (r *Reader) Subscriptions(ctx context.Context, principal string) (eventaccess.Subscriptions, error) {
	return catalogValue[eventaccess.Subscriptions](r.owner.EventSubscriptions(ctx, principal))
}
func (r *Reader) ReadEvents(ctx context.Context, principal, id string, limit int) (eventaccess.Page, error) {
	return catalogValue[eventaccess.Page](r.owner.ReadSubscriptionEvents(ctx, principal, id, limit))
}
