package mobilebackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

// CleanupTerminal removes only authenticated snapshots whose bound request and
// account are terminal according to the trusted local operation journal. It
// retains spent UUIDs, so cleanup never authorizes a renewed source or dispatch.
func (s *SnapshotStore) CleanupTerminal(
	ctx context.Context,
	eligible func(context.Context, string, string, string) (bool, error),
) error {
	if eligible == nil || !s.lock(ctx) {
		return ErrSnapshot
	}
	defer s.unlock()
	if s.root == nil {
		return ErrSnapshot
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return ErrSnapshot
	}
	entries, err := dir.ReadDir(maxSnapshots + snapshotDirectoryExtras)
	//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
	dir.Close()
	if err != nil && !errors.Is(err, io.EOF) || len(entries) > maxSnapshots+3 {
		return ErrSnapshotCapacity
	}
	if s.terminalHints == nil {
		s.terminalHints = make(map[string]snapshotMetadata)
	}
	seen := make(map[string]bool, maxSnapshots)
	changed := false
	for _, entry := range entries {
		if ctx.Err() != nil {
			return ErrSnapshot
		}
		entryChanged, entryErr := s.cleanupTerminalEntry(ctx, entry, seen, eligible)
		if entryErr != nil {
			return entryErr
		}
		changed = changed || entryChanged
	}
	for id := range s.terminalHints {
		if !seen[id] {
			delete(s.terminalHints, id)
		}
	}
	if changed {
		return snapshotSyncDirectory(s.root)
	}
	return nil
}

func (s *SnapshotStore) cleanupTerminalEntry(
	ctx context.Context,
	entry os.DirEntry,
	seen map[string]bool,
	eligible func(context.Context, string, string, string) (bool, error),
) (bool, error) {
	changed := false
	name := entry.Name()
	if name == snapshotKeyName || name == snapshotClaimsName {
		return changed, nil
	}
	if before, ok := strings.CutSuffix(name, ".tmp"); ok {
		if tempErr := s.removeSnapshotTemporary(name, before); tempErr != nil {
			return false, tempErr
		}
		changed = true
		return changed, nil
	}
	id := strings.TrimSuffix(name, ".snapshot")
	expected, ok := snapshotName(id)
	info, e := s.root.Lstat(name)
	if !ok || expected != name || !s.claims[id] || e != nil || !privateRegular(info) {
		return false, ErrSnapshot
	}
	seen[id] = true
	if len(seen) > maxSnapshots ||
		info.Size() < int64(len(snapshotMagic)+s.aead.NonceSize()+s.aead.Overhead()+snapshotMetadataSizeBytes) ||
		info.Size() > int64(MaxFileBytes)+snapshotMetadataLimit+64 {
		return false, ErrSnapshotCapacity
	}
	// A hint can only skip work. It cannot authorize deletion or serve data.
	// Terminal/expired candidates are always decoded and checked again below.
	skip, hintErr := s.skipTerminalHint(ctx, id, eligible)
	if hintErr != nil {
		return false, hintErr
	}
	if skip {
		return false, nil
	}
	meta, data, e := s.decode(name, id)
	clear(data)
	if e != nil || ctx.Err() != nil {
		return false, ErrSnapshot
	}
	s.terminalHints[id] = meta
	remove := s.now().UnixMilli() >= meta.ExpiresMS
	if !remove {
		owner := sha256.Sum256([]byte(meta.Account))
		remove, e = eligible(ctx, id, meta.Fingerprint, hex.EncodeToString(owner[:]))
		if e != nil {
			return false, e
		}
	}
	if ctx.Err() != nil {
		return false, ErrSnapshot
	}
	if remove {
		if err := s.root.Remove(name); err != nil {
			return false, ErrSnapshot
		}
		delete(s.terminalHints, id)
		changed = true
	}
	return changed, nil
}

func (s *SnapshotStore) skipTerminalHint(
	ctx context.Context,
	id string,
	eligible func(context.Context, string, string, string) (bool, error),
) (bool, error) {
	hint, cached := s.terminalHints[id]
	if !cached || s.now().UnixMilli() >= hint.ExpiresMS {
		return false, nil
	}
	owner := sha256.Sum256([]byte(hint.Account))
	remove, err := eligible(ctx, id, hint.Fingerprint, hex.EncodeToString(owner[:]))
	if err != nil {
		return false, err
	}
	if ctx.Err() != nil {
		return false, ErrSnapshot
	}
	return !remove, nil
}

func (s *SnapshotStore) removeSnapshotTemporary(name, id string) error {
	_, valid := snapshotName(id)
	info, err := s.root.Lstat(name)
	if !valid || err != nil || !privateRegular(info) || s.root.Remove(name) != nil {
		return ErrSnapshot
	}
	return nil
}
