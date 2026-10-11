// Package operations adapts existing owners without acquiring ledger or session ownership.
package operations

import (
	"context"
	"encoding/json"

	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Backend is the existing local control/native owner contract. Its legacy maps
// stay inside this adapter; consumer ports expose only fixed typed reads.
type Backend interface {
	Call(context.Context, string, any) (map[string]any, error)
}

type Reader struct{ owner Backend }

func NewReader(owner Backend) *Reader { return &Reader{owner: owner} }

var (
	_ messaging.StatusPort = (*Reader)(nil)
	_ history.StatusPort   = (*Reader)(nil)
	_ membership.ReadPort  = (*Reader)(nil)
)

func decode[T any](value map[string]any, err error) (T, error) {
	var result T
	if err != nil {
		return result, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}

func (r *Reader) SendStatus(ctx context.Context, requestID string) (domain.SendOperation, error) {
	return decode[domain.SendOperation](r.owner.Call(ctx, "zalo_get_send_status", map[string]any{"request_id": requestID}))
}

func (r *Reader) ImportStatus(ctx context.Context, operationID string) (domain.HistoryImportStatus, error) {
	return decode[domain.HistoryImportStatus](r.owner.Call(ctx, "zalo_get_history_import_status", map[string]any{"operation_id": operationID}))
}

func (r *Reader) InspectInvite(ctx context.Context, inviteURL string) (membership.Invite, error) {
	return decode[membership.Invite](r.owner.Call(ctx, "zalo_inspect_invite", map[string]any{"invite_url": inviteURL}))
}

func (r *Reader) JoinStatus(ctx context.Context, operationID string) (membership.JoinStatus, error) {
	return decode[membership.JoinStatus](r.owner.Call(ctx, "zalo_get_join_status", map[string]any{"operation_id": operationID}))
}

func (r *Reader) Group(ctx context.Context, groupID string) (conversations.GroupDetail, error) {
	return decode[conversations.GroupDetail](r.owner.Call(ctx, "zalo_get_group", map[string]any{"group_id": groupID}))
}
