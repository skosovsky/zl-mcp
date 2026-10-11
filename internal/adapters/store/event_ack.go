package store

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
)

var _ eventaccess.AcknowledgementPort = (*Reader)(nil)

func (r *Reader) Acknowledge(ctx context.Context, principal, id, receipt string) (eventaccess.Acknowledgement, error) {
	return catalogValue[eventaccess.Acknowledgement](r.owner.AckSubscriptionEvents(ctx, principal, id, receipt))
}
