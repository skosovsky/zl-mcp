package mobilebackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

// CleanupTerminal removes only authenticated snapshots whose bound request and
// account are terminal according to the trusted local operation journal. It
// retains spent UUIDs, so cleanup never authorizes a renewed source or dispatch.
func (s *SnapshotStore) CleanupTerminal(ctx context.Context, eligible func(context.Context, string, string, string) (bool, error)) error {
	if eligible == nil || !s.lock(ctx) {
		return ErrSnapshot
	}
	defer s.unlock()
	if s.root == nil {
		return ErrSnapshot
	}
	if _, _, err := s.sweep(ctx); err != nil {
		return err
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return ErrSnapshot
	}
	entries, err := dir.ReadDir(maxSnapshots + 3)
	dir.Close()
	if err != nil && err != io.EOF || len(entries) > maxSnapshots+2 {
		return ErrSnapshot
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".snapshot") {
			continue // sweep already rejected unknown artifacts.
		}
		id := strings.TrimSuffix(entry.Name(), ".snapshot")
		meta, data, err := s.decode(entry.Name(), id)
		clear(data)
		if err != nil || ctx.Err() != nil {
			return ErrSnapshot
		}
		owner := sha256.Sum256([]byte(meta.Account))
		remove, err := eligible(ctx, id, meta.Fingerprint, hex.EncodeToString(owner[:]))
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ErrSnapshot
		}
		if remove {
			if err := s.root.Remove(entry.Name()); err != nil {
				return ErrSnapshot
			}
			if err := snapshotSyncDirectory(s.root); err != nil {
				return err
			}
		}
	}
	return nil
}
