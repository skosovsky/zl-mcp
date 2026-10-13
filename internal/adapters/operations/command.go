package operations

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

var (
	_ messaging.CommandPort  = (*Reader)(nil)
	_ history.CommandPort    = (*Reader)(nil)
	_ membership.CommandPort = (*Reader)(nil)
)

func (r *Reader) Send(ctx context.Context, request domain.SendRequest) (domain.SendOperation, error) {
	return decode[domain.SendOperation](r.owner.Call(ctx, "zalo_send_direct_message", request))
}
func (r *Reader) Join(ctx context.Context, request membership.JoinRequest) (membership.JoinStatus, error) {
	return decode[membership.JoinStatus](r.owner.Call(ctx, "zalo_join_group", request))
}
func (r *Reader) StartImport(ctx context.Context, request history.StartRequest) (domain.HistoryImportStatus, error) {
	return decode[domain.HistoryImportStatus](r.owner.Call(ctx, "zalo_import_conversation_history", request))
}
func (r *Reader) CancelImport(ctx context.Context, id string) (domain.HistoryImportStatus, error) {
	return decode[domain.HistoryImportStatus](r.owner.Call(ctx, "zalo_cancel_history_import", struct {
		OperationID string `json:"operation_id"`
	}{id}))
}
