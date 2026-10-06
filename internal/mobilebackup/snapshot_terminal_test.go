package mobilebackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotTerminalCleanupRetainsSpentIdentity(t *testing.T) {
	// Arrange: authenticated source, private binding and local owner predicate.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1<<20)
	selected, request := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	if err := s.Save(ctx, selected, request, "10"); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("10"))
	owner := hex.EncodeToString(digest[:])
	calls := 0
	eligible := func(_ context.Context, id, fingerprint, account string) (bool, error) {
		calls++
		if id != request.RequestID || fingerprint != request.Fingerprint() || account != owner {
			t.Fatal("cleanup lost authenticated binding")
		}
		return false, nil
	}
	// Act: preserve active/paused source, then remove it when the journal is terminal.
	if err := s.CleanupTerminal(ctx, eligible); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Read(ctx, request, "10"); err != nil {
		t.Fatal(err)
	} else {
		got.Clear()
	}
	if err := s.CleanupTerminal(ctx, func(context.Context, string, string, string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	// Assert: image gone, key/claims retained, retry cannot renew the source.
	name, _ := snapshotName(request.RequestID)
	if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) || calls != 1 {
		t.Fatal("terminal source retained", err)
	}
	if err := s.Save(ctx, selected, request, "10"); !errors.Is(err, ErrSnapshot) {
		t.Fatal("cleanup renewed source", err)
	}
	if err := s.CleanupTerminal(ctx, eligible); err != nil || calls != 1 {
		t.Fatal("repeat cleanup was not idempotent", err)
	}
}

func TestSnapshotTerminalCleanupPredicateFailurePreservesImage(t *testing.T) {
	// Arrange: journal read failure must not turn into permission to remove data.
	s := openSnapshotStore(t, filepath.Join(t.TempDir(), "snapshots"), 1<<20)
	selected, request := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	if err := s.Save(ctx, selected, request, "10"); err != nil {
		t.Fatal(err)
	}
	failed := errors.New("synthetic journal failure")
	// Act.
	err := s.CleanupTerminal(ctx, func(context.Context, string, string, string) (bool, error) { return true, failed })
	// Assert: real error retained and the original bound image is readable.
	if !errors.Is(err, failed) {
		t.Fatal("journal failure masked", err)
	}
	got, err := s.Read(ctx, request, "10")
	defer got.Clear()
	if err != nil {
		t.Fatal("image removed without authority", err)
	}
}
