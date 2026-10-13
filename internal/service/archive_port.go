package service

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// ArchiveReader borrows storage and library ownership. It forwards to the existing
// offline archive implementation; it neither starts a session nor owns shutdown.
type ArchiveReader struct{ owner *membershipPort }

func NewArchiveReader(
	store *storage.Store,
	library *mobilebackup.RetainedArchiveStore,
	stateDir string,
) *ArchiveReader {
	//nolint:exhaustruct_v5 // membershipPort initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
	return &ArchiveReader{owner: &membershipPort{store: store, library: library, stateDir: stateDir}}
}
func (r *ArchiveReader) Call(ctx context.Context, method string, args any) (map[string]any, error) {
	return r.owner.archiveRead(ctx, method, args)
}
