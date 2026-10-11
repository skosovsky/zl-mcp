package store

import (
	"context"
	"encoding/json"

	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
)

var _ conversations.CatalogPort = (*Reader)(nil)
var _ shared.AdmissionPort = (*Reader)(nil)

func catalogValue[T any](value map[string]any, err error) (T, error) {
	var result T
	if err != nil {
		return result, readError(err)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(b, &result)
	return result, err
}
func (r *Reader) AllowRead(ctx context.Context) (bool, error) { return r.owner.AllowRead(ctx) }
func (r *Reader) CollectorStatus(ctx context.Context) (conversations.CollectorStatus, error) {
	return catalogValue[conversations.CollectorStatus](r.owner.State(ctx))
}
func (r *Reader) CollectionStatus(ctx context.Context) (conversations.Collection, error) {
	return catalogValue[conversations.Collection](r.owner.CollectionStatus(ctx))
}
func (r *Reader) ContactDiagnostics(ctx context.Context) (conversations.CatalogDiagnostics, error) {
	return catalogValue[conversations.CatalogDiagnostics](r.owner.ContactStatus(ctx))
}
func (r *Reader) ProfileDiagnostics(ctx context.Context) (conversations.ProfileDiagnostics, error) {
	return catalogValue[conversations.ProfileDiagnostics](r.owner.ProfileStatus(ctx))
}
func (r *Reader) PreloadDiagnostics(ctx context.Context) (conversations.PreloadDiagnostics, error) {
	return catalogValue[conversations.PreloadDiagnostics](r.owner.PreloadStatus(ctx))
}
func (r *Reader) Groups(ctx context.Context, request conversations.GroupRequest) (conversations.GroupList, error) {
	return catalogValue[conversations.GroupList](r.owner.Groups(ctx, request.Query, request.Limit, request.Cursor))
}
func (r *Reader) Group(ctx context.Context, id string) (conversations.GroupDetail, error) {
	return catalogValue[conversations.GroupDetail](r.owner.Group(ctx, id))
}
