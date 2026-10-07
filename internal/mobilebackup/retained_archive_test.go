package mobilebackup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
)

const retainedTestID = "00000000-0000-4000-8000-000000000001"

func retainedFixture(t *testing.T) AccountArchive {
	t.Helper()
	decoded := Format1Archive{Archive: selectionArchive(), CiphertextBytes: 123, ContainerBytes: 100, TrailingBytes: 23}
	pairs := []IdentityPair{{Plain: "9007199254740993", Session: "12"}, {Plain: "9007199254740993", Session: "12", Group: true}, {Plain: "18446744073709551615", Session: "13"}}
	a, err := ownAccountArchive(context.Background(), &decoded, pairs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Clear)
	return a
}

func TestRetainedArchiveRestartOfflineReadsAndImmutableRetry(t *testing.T) {
	// Arrange: complete mapped source, dedicated store, controlled acquisition time.
	ctx := context.Background()
	a := retainedFixture(t)
	dir := filepath.Join(t.TempDir(), "account-archives")
	s, err := NewRetainedArchiveStore(dir, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	s.ledger.now = func() time.Time { return now }
	// Act: save and retry without changing source or lifetime, then restart storage.
	first, err := s.Save(ctx, retainedTestID, "123", a, 0)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := retainedName(retainedTestID)
	before, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	retry, err := s.Save(ctx, retainedTestID, "123", a, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewRetainedArchiveStore(dir, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	read, manifest, err := s.Read(ctx, retainedTestID, "123")
	if err != nil {
		t.Fatal(err)
	}
	defer read.Clear()
	after, _ := os.ReadFile(filepath.Join(dir, name))
	// Assert: source and expiry unchanged, all files/mappings survived with no network.
	if !reflect.DeepEqual(first, retry) || !reflect.DeepEqual(first, manifest) || !bytes.Equal(before, after) || len(read.archive.Files) != 3 || read.pairs[2].Session != "13" || string(read.archive.Files[2].Data) != "synthetic-c" || first.Scope != "account" || first.FileCount != 3 || first.DirectFiles != 2 || first.GroupFiles != 1 || first.HistoryComplete || first.ImportPerformed {
		t.Fatal("retained source changed or lost files")
	}
	for _, marker := range []string{"synthetic-a", "synthetic-b", "synthetic-c", "9007199254740993.db", "18446744073709551615.db"} {
		if bytes.Contains(before, []byte(marker)) {
			t.Fatal("plaintext retained on disk")
		}
	}
	b, _ := json.Marshal(manifest)
	var receipt any
	if json.Unmarshal(b, &receipt) != nil {
		t.Fatal("invalid manifest")
	}
	schema, err := contracts.Compile("retained_account_archive_manifest", "output")
	if err != nil || schema.Validate(receipt) != nil {
		t.Fatal("manifest violates executable contract", err)
	}
	buffer := read.archive.Files[0].Data
	read.Clear()
	if !bytes.Equal(buffer, make([]byte, len(buffer))) {
		t.Fatal("read buffer not cleared")
	}
	second, _, err := s.Read(ctx, retainedTestID, "123")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Clear()
	if string(second.archive.Files[0].Data) != "synthetic-a" {
		t.Fatal("clearing a read damaged retained source")
	}
}

func TestRetainedArchiveConflictAndCapacityDoNotReplaceSource(t *testing.T) {
	// Arrange: one stored account archive.
	ctx := context.Background()
	a := retainedFixture(t)
	s, err := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "account-archives"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.Save(ctx, retainedTestID, "123", a, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Act / Assert: conflicting scope/lifetime/content must leave the source intact.
	if _, err = s.Save(ctx, retainedTestID, "124", a, 0); !errors.Is(err, ErrRetainedConflict) {
		t.Fatal("foreign account accepted", err)
	}
	if _, err = s.Save(ctx, retainedTestID, "123", a, time.Hour); !errors.Is(err, ErrRetainedConflict) {
		t.Fatal("expiry changed", err)
	}
	a.archive.Files[0].Data[0] = 'X'
	if _, err = s.Save(ctx, retainedTestID, "123", a, 0); !errors.Is(err, ErrRetainedConflict) {
		t.Fatal("content replaced", err)
	}
	read, receipt, err := s.Read(ctx, retainedTestID, "123")
	if err != nil {
		t.Fatal(err)
	}
	defer read.Clear()
	if !reflect.DeepEqual(first, receipt) || string(read.archive.Files[0].Data) != "synthetic-a" {
		t.Fatal("conflict altered saved source")
	}
	if _, err = s.Save(ctx, "00000000-0000-4000-8000-000000000002", "123", a, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(ctx, "00000000-0000-4000-8000-000000000003", "123", a, 0); !errors.Is(err, ErrRetainedCapacity) {
		t.Fatal("source-count quota ignored", err)
	}
	small, err := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "account-archives"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer small.Close()
	if _, err = small.Save(ctx, retainedTestID, "123", a, 0); !errors.Is(err, ErrRetainedCapacity) || small.ledger.claims[retainedTestID] {
		t.Fatal("byte quota partially published or spent source", err)
	}
}

func TestRetainedArchiveExpiryRemovalAndSpentIDs(t *testing.T) {
	for _, mode := range []string{"expired", "removed"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: exact source bound to its owner.
			ctx := context.Background()
			a := retainedFixture(t)
			dir := filepath.Join(t.TempDir(), "account-archives")
			s, err := NewRetainedArchiveStore(dir, 8<<20)
			if err != nil {
				t.Fatal(err)
			}
			m, err := s.Save(ctx, retainedTestID, "123", a, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			// Act: foreign removal rejected; owner removal or expiry consumes source.
			if err = s.Remove(ctx, retainedTestID, "124"); !errors.Is(err, ErrRetainedConflict) {
				t.Fatal("foreign removal accepted", err)
			}
			if mode == "expired" {
				until, _ := time.Parse(time.RFC3339Nano, m.ExpiresAt)
				s.ledger.now = func() time.Time { return until }
				if _, _, err = s.Read(ctx, retainedTestID, "123"); !errors.Is(err, ErrRetainedExpired) {
					t.Fatal("expiry ignored", err)
				}
			} else {
				if s.Remove(ctx, retainedTestID, "123") != nil || s.Remove(ctx, retainedTestID, "123") != nil {
					t.Fatal("owner removal failed")
				}
			}
			s.Close()
			s, err = NewRetainedArchiveStore(dir, 8<<20)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			// Assert: restart cannot renew a removed/expired UUID.
			if _, err = s.Save(ctx, retainedTestID, "123", a, time.Hour); err == nil || !s.ledger.claims[retainedTestID] {
				t.Fatal("spent UUID republished")
			}
		})
	}
}

func TestRetainedArchiveTamperingKeyLossAndUnsafeArtifacts(t *testing.T) {
	for _, mode := range []string{"ciphertext", "missing-key", "symlink", "permissions", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: close a valid persisted source before modifying local artifacts.
			ctx := context.Background()
			a := retainedFixture(t)
			dir := filepath.Join(t.TempDir(), "account-archives")
			s, err := NewRetainedArchiveStore(dir, 8<<20)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Save(ctx, retainedTestID, "123", a, 0); err != nil {
				t.Fatal(err)
			}
			s.Close()
			name, _ := retainedName(retainedTestID)
			path := filepath.Join(dir, name)
			switch mode {
			case "ciphertext":
				data, _ := os.ReadFile(path)
				data[len(data)-1] ^= 1
				if os.WriteFile(path, data, 0600) != nil {
					t.Fatal("tamper setup")
				}
			case "missing-key":
				if os.Remove(filepath.Join(dir, snapshotKeyName)) != nil {
					t.Fatal("key removal setup")
				}
			case "symlink":
				if os.Remove(path) != nil || os.Symlink("/dev/null", path) != nil {
					t.Fatal("symlink setup")
				}
			case "permissions":
				if os.Chmod(path, 0644) != nil {
					t.Fatal("permissions setup")
				}
			case "unknown":
				if os.WriteFile(filepath.Join(dir, "unknown"), []byte("private"), 0600) != nil {
					t.Fatal("unknown setup")
				}
			}
			// Act / Assert: reopening fails without exposing any prefix or rotating keys.
			if got, err := NewRetainedArchiveStore(dir, 8<<20); err == nil {
				got.Close()
				t.Fatal("unsafe archive reopened")
			}
			if mode == "missing-key" {
				if _, err = os.Stat(filepath.Join(dir, snapshotKeyName)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing key silently regenerated")
				}
			}
		})
	}
}

func TestRetainedArchiveCancelledAndClosedOperations(t *testing.T) {
	// Arrange: valid source and store, but cancelled caller.
	a := retainedFixture(t)
	s, err := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "account-archives"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act / Assert: no save/claim; operations after close fail without locking forever.
	if _, err = s.Save(ctx, retainedTestID, "123", a, 0); err == nil || s.ledger.claims[retainedTestID] {
		t.Fatal("cancelled save published")
	}
	s.Close()
	if _, _, err = s.Read(context.Background(), retainedTestID, "123"); err == nil {
		t.Fatal("closed read accepted")
	}
	if _, err = s.Save(context.Background(), retainedTestID, "123", a, 0); err == nil {
		t.Fatal("closed save accepted")
	}
}

func TestRetainedArchiveBoundOwnerCannotRepublishToAnotherAccount(t *testing.T) {
	// Arrange: an authenticated retained read, with the original owner bound inside ciphertext.
	ctx := context.Background()
	a := retainedFixture(t)
	s, e := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "account-archives"), 8<<20)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.Save(ctx, retainedTestID, "123", a, 0); e != nil {
		t.Fatal(e)
	}
	read, _, e := s.Read(ctx, retainedTestID, "123")
	if e != nil {
		t.Fatal(e)
	}
	defer read.Clear()
	// Act: try to rebind those bytes to another owner under a fresh UUID.
	_, e = s.Save(ctx, "00000000-0000-4000-8000-000000000002", "124", read, 0)
	// Assert: no new source or durable claim is published.
	if e == nil || s.ledger.claims["00000000-0000-4000-8000-000000000002"] {
		t.Fatal("authenticated archive rebound")
	}
}
