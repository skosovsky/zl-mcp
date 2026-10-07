package mobilebackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func expiredBackupFixture(t *testing.T) (string, RetainedArchiveManifest, map[string][]byte) {
	t.Helper()
	cachePath := filepath.Join(t.TempDir(), "cache")
	cache, err := NewRetainedArchiveStore(cachePath, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	captured := time.Now().Add(-48 * time.Hour).Truncate(time.Millisecond)
	cache.ledger.now = func() time.Time { return captured }
	manifest, err := cache.Save(context.Background(), retainedTestID, "123", retainedFixture(t), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if err = os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	name, _ := retainedName(retainedTestID)
	original := map[string][]byte{}
	for _, file := range []string{name, snapshotKeyName, snapshotClaimsName} {
		data, err := os.ReadFile(filepath.Join(cachePath, file))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(backup, file), data, 0600); err != nil {
			t.Fatal(err)
		}
		original[file] = data
	}
	cache.ledger.now = time.Now
	if _, _, err = cache.Read(context.Background(), retainedTestID, "123"); !errors.Is(err, ErrRetainedExpired) {
		t.Fatal("cache was not expired", err)
	}
	return backup, manifest, original
}

func TestRestoreExpiredBackupWithoutChangingOriginal(t *testing.T) {
	// Arrange: an encrypted recovery copy whose original cache is already deleted.
	backup, original, files := expiredBackupFixture(t)
	path := filepath.Join(t.TempDir(), "library")
	library, err := NewArchiveLibraryStore(path, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte("123"))
	binding := hex.EncodeToString(h[:])
	ctx := context.Background()
	// Act: explicit restore, repeated restore, and restart without cache or network.
	first, err := library.RestoreBackup(ctx, backup, retainedTestID, binding, original.Digest)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := library.RestoreBackup(ctx, backup, retainedTestID, binding, original.Digest)
	if err != nil {
		t.Fatal(err)
	}
	library.Close()
	library, err = NewArchiveLibraryStore(path, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	read, manifest, err := library.ReadBound(ctx, retainedTestID, binding)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Clear()
	// Assert: full source and historical expiry survive; backup artifacts never change.
	if !reflect.DeepEqual(first, retry) || !reflect.DeepEqual(manifest, original) || len(read.archive.Files) != 3 || string(read.archive.Files[0].Data) != "synthetic-a" {
		t.Fatal("restore changed provenance or lost content")
	}
	for name, before := range files {
		after, err := os.ReadFile(filepath.Join(backup, name))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("restore changed recovery copy")
		}
	}
	if err = library.RemoveBound(ctx, retainedTestID, binding); err != nil {
		t.Fatal(err)
	}
	if _, err = library.RestoreBackup(ctx, backup, retainedTestID, binding, original.Digest); err == nil {
		t.Fatal("removed library source resurrected")
	}
}

func TestRestoreBackupRejectsUnsafeInputsWithoutPublishing(t *testing.T) {
	for _, mode := range []string{"foreign-owner", "digest", "ciphertext", "missing-key", "symlink", "permissions", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: independently saved expired backup and empty durable library.
			backup, manifest, _ := expiredBackupFixture(t)
			library, err := NewArchiveLibraryStore(filepath.Join(t.TempDir(), "library"), 8<<20)
			if err != nil {
				t.Fatal(err)
			}
			defer library.Close()
			h := sha256.Sum256([]byte("123"))
			binding := hex.EncodeToString(h[:])
			digest := manifest.Digest
			ctx := context.Background()
			name, _ := retainedName(retainedTestID)
			switch mode {
			case "foreign-owner":
				h = sha256.Sum256([]byte("124"))
				binding = hex.EncodeToString(h[:])
			case "digest":
				digest = "0000000000000000000000000000000000000000000000000000000000000000"
			case "ciphertext":
				data, e := os.ReadFile(filepath.Join(backup, name))
				if e != nil {
					t.Fatal(e)
				}
				data[len(data)-1] ^= 1
				if e = os.WriteFile(filepath.Join(backup, name), data, 0600); e != nil {
					t.Fatal(e)
				}
			case "missing-key":
				if err = os.Remove(filepath.Join(backup, snapshotKeyName)); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				link := filepath.Join(t.TempDir(), "link")
				if err = os.Symlink(backup, link); err != nil {
					t.Fatal(err)
				}
				backup = link
			case "permissions":
				if err = os.Chmod(filepath.Join(backup, name), 0644); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			// Act / Assert: no durable claim/source, no repair or key creation.
			if _, err = library.RestoreBackup(ctx, backup, retainedTestID, binding, digest); err == nil || library.ledger.claims[retainedTestID] {
				t.Fatal("unsafe recovery published")
			}
			if mode == "missing-key" {
				if _, err = os.Stat(filepath.Join(backup, snapshotKeyName)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing backup key regenerated")
				}
			}
		})
	}
}

func TestRestoreBackupCapacityFailureDoesNotSpendSource(t *testing.T) {
	// Arrange: complete recovery source, but no room for its encrypted publication.
	backup, manifest, _ := expiredBackupFixture(t)
	library, err := NewArchiveLibraryStore(filepath.Join(t.TempDir(), "library"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	h := sha256.Sum256([]byte("123"))
	// Act: admission checks the complete source before applying the capacity limit.
	_, err = library.RestoreBackup(context.Background(), backup, retainedTestID, hex.EncodeToString(h[:]), manifest.Digest)
	// Assert: no partial publication or spent claim, leaving a later valid retry possible.
	if !errors.Is(err, ErrRetainedCapacity) || library.ledger.claims[retainedTestID] {
		t.Fatal("capacity failure partially published", err)
	}
}
