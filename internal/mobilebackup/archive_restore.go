package mobilebackup

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"io"
	"os"
	"path/filepath"
	"time"
)

// openArchiveBackup reads only existing key/claims. In particular it never runs
// cache inventory, expiry cleanup, publication or key creation on a recovery copy.
func openArchiveBackup(dir string) (*RetainedArchiveStore, error) {
	if !filepath.IsAbs(dir) {
		return nil, ErrRetainedArchive
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrRetainedArchive
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, ErrRetainedArchive
	}
	failed := true
	defer func() {
		if failed {
			root.Close()
		}
	}()
	info, err = root.Lstat(snapshotKeyName)
	if err != nil || !privateRegular(info) || info.Size() != 32 {
		return nil, ErrRetainedArchive
	}
	f, err := root.Open(snapshotKeyName)
	if err != nil {
		return nil, ErrRetainedArchive
	}
	if !samePrivateFile(f, info) {
		f.Close()
		return nil, ErrRetainedArchive
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	f.Close()
	defer clear(key)
	if err != nil || len(key) != 32 {
		return nil, ErrRetainedArchive
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrRetainedArchive
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrRetainedArchive
	}
	claims, err := readSnapshotClaims(root)
	if err != nil {
		return nil, ErrRetainedArchive
	}
	store := &RetainedArchiveStore{backup: true, ledger: &SnapshotStore{root: root, aead: aead, claims: claims, now: time.Now, mu: make(chan struct{}, 1)}}
	store.ledger.mu <- struct{}{}
	failed = false
	return store, nil
}

// RestoreBackup explicitly admits an authenticated recovery copy after its
// original cache expiry. It neither changes that copy nor fabricates provenance.
func (s *RetainedArchiveStore) RestoreBackup(ctx context.Context, dir, id, accountKey, expectedDigest string) (PreservationReceipt, error) {
	if s == nil || !s.permanent || ctx.Err() != nil {
		return PreservationReceipt{}, ErrRetainedArchive
	}
	backup, err := openArchiveBackup(dir)
	if err != nil {
		return PreservationReceipt{}, err
	}
	defer backup.Close()
	if !backup.lock(ctx) {
		return PreservationReceipt{}, ErrRetainedArchive
	}
	meta, a, stored, err := backup.readBound(ctx, id, accountKey)
	backup.ledger.unlock()
	defer a.Clear()
	if err != nil {
		return PreservationReceipt{}, err
	}
	if meta.Digest != expectedDigest {
		return PreservationReceipt{}, ErrRetainedConflict
	}
	return s.preserveAuthenticated(ctx, meta, a, stored, accountKey)
}
