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
	dir, err := s.root.Open(".")
	if err != nil {
		return ErrSnapshot
	}
	entries, err := dir.ReadDir(maxSnapshots + 4)
	dir.Close()
	if err != nil && err != io.EOF || len(entries) > maxSnapshots+3 {
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
		name := entry.Name()
		if name == snapshotKeyName || name == snapshotClaimsName {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			id := strings.TrimSuffix(name, ".tmp")
			_, ok := snapshotName(id)
			info, e := s.root.Lstat(name)
			if !ok || e != nil || !privateRegular(info) || s.root.Remove(name) != nil {
				return ErrSnapshot
			}
			changed = true
			continue
		}
		id := strings.TrimSuffix(name, ".snapshot")
		expected, ok := snapshotName(id)
		info, e := s.root.Lstat(name)
		if !ok || expected != name || !s.claims[id] || e != nil || !privateRegular(info) {
			return ErrSnapshot
		}
		seen[id] = true
		if len(seen) > maxSnapshots || info.Size() < int64(len(snapshotMagic)+s.aead.NonceSize()+s.aead.Overhead()+4) || info.Size() > int64(MaxFileBytes)+snapshotMetadataLimit+64 {
			return ErrSnapshotCapacity
		}
		// A hint can only skip work. It cannot authorize deletion or serve data.
		// Terminal/expired candidates are always decoded and checked again below.
		if hint, cached := s.terminalHints[id]; cached && s.now().UnixMilli() < hint.ExpiresMS {
			owner := sha256.Sum256([]byte(hint.Account))
			remove, e := eligible(ctx, id, hint.Fingerprint, hex.EncodeToString(owner[:]))
			if e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ErrSnapshot
			}
			if !remove {
				continue
			}
		}
		meta, data, e := s.decode(name, id)
		clear(data)
		if e != nil || ctx.Err() != nil {
			return ErrSnapshot
		}
		s.terminalHints[id] = meta
		remove := s.now().UnixMilli() >= meta.ExpiresMS
		if !remove {
			owner := sha256.Sum256([]byte(meta.Account))
			remove, e = eligible(ctx, id, meta.Fingerprint, hex.EncodeToString(owner[:]))
			if e != nil {
				return e
			}
		}
		if ctx.Err() != nil {
			return ErrSnapshot
		}
		if remove {
			if err := s.root.Remove(name); err != nil {
				return ErrSnapshot
			}
			delete(s.terminalHints, id)
			changed = true
		}
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
