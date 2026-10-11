package operations

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/archives"
)

// ArchiveReader borrows the same backend as operation reads; no library handle,
// DB, account/session or cleanup worker is created here.
type ArchiveReader struct{ owner Backend }

func NewArchiveReader(owner Backend) *ArchiveReader { return &ArchiveReader{owner: owner} }

var _ archives.ReadPort = (*ArchiveReader)(nil)

func (r *ArchiveReader) Sources(ctx context.Context) (archives.Sources, error) {
	return decode[archives.Sources](r.owner.Call(ctx, "zalo_list_archive_sources", map[string]any{}))
}
func (r *ArchiveReader) Conversations(ctx context.Context, q archives.DirectoryRequest) (archives.Directory, error) {
	return decode[archives.Directory](r.owner.Call(ctx, "zalo_list_conversations", q))
}
func (r *ArchiveReader) Browse(ctx context.Context, q archives.BrowseRequest) (archives.Browse, error) {
	return decode[archives.Browse](r.owner.Call(ctx, "zalo_list_conversation_messages", q))
}
func (r *ArchiveReader) Record(ctx context.Context, token string) (archives.Record, error) {
	return decode[archives.Record](r.owner.Call(ctx, "read_archive_resource", map[string]any{"token": token}))
}
