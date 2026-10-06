package mobilebackup

import (
	"context"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// Counting the crypto operation tests actual archive reads, not polling timing.
type cleanupCountingAEAD struct {
	cipher.AEAD
	opens int
}

func (a *cleanupCountingAEAD) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	a.opens++
	return a.AEAD.Open(dst, nonce, ciphertext, additionalData)
}

func TestSnapshotTerminalCleanupHintsCannotAuthorizeRemoval(t *testing.T) {
	// Arrange: a valid private image and an instrumented authenticated decoder.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1<<20)
	selected, request := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	if err := s.Save(ctx, selected, request, "10"); err != nil {
		t.Fatal(err)
	}
	counter := &cleanupCountingAEAD{AEAD: s.aead}
	s.aead = counter
	active := func(context.Context, string, string, string) (bool, error) { return false, nil }
	if err := s.CleanupTerminal(ctx, active); err != nil {
		t.Fatal(err)
	}
	// Act: repeated idle polls must not reread the full encrypted image.
	for i := 0; i < 3; i++ {
		if err := s.CleanupTerminal(ctx, active); err != nil {
			t.Fatal(err)
		}
	}
	if counter.opens != 1 {
		t.Fatal("idle cleanup repeatedly decrypted archive", counter.opens)
	}
	// Replace authenticated bytes after hint creation; hints must not grant deletion.
	name, _ := snapshotName(request.RequestID)
	file := filepath.Join(dir, name)
	bytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	bytes[len(bytes)-1] ^= 1
	if err := os.WriteFile(file, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	err = s.CleanupTerminal(ctx, func(context.Context, string, string, string) (bool, error) { return true, nil })
	// Assert: current bytes were authenticated again and rejected without removal.
	if !errors.Is(err, ErrSnapshot) || counter.opens != 2 {
		t.Fatal("hint granted removal without authentication", err, counter.opens)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("changed source was removed", err)
	}
}

func TestSnapshotTerminalCleanupRechecksJournalAfterHint(t *testing.T) {
	// Arrange: authenticate one retained image to populate the idle hint.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1<<20)
	selected, request := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	if err := s.Save(ctx, selected, request, "10"); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupTerminal(ctx, func(context.Context, string, string, string) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	calls := 0
	// Act: the hint lookup says terminal; the authoritative second lookup refuses.
	err := s.CleanupTerminal(ctx, func(context.Context, string, string, string) (bool, error) { calls++; return calls == 1, nil })
	// Assert: only the current authenticated binding can authorize removal.
	if err != nil || calls != 2 {
		t.Fatal("journal was not rechecked", err, calls)
	}
	got, err := s.Read(ctx, request, "10")
	defer got.Clear()
	if err != nil {
		t.Fatal("hint alone removed retained source", err)
	}
}

func TestSnapshotTerminalCleanupExpiresCachedImage(t *testing.T) {
	// Arrange: a cached nonterminal image with a controlled clock.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1<<20)
	now := time.Now()
	s.now = func() time.Time { return now }
	selected, request := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	if err := s.Save(ctx, selected, request, "10"); err != nil {
		t.Fatal(err)
	}
	active := func(context.Context, string, string, string) (bool, error) { return false, nil }
	counter := &cleanupCountingAEAD{AEAD: s.aead}
	s.aead = counter
	if err := s.CleanupTerminal(ctx, active); err != nil {
		t.Fatal(err)
	}
	// Act: expiry must override the cached nonterminal hint, with fresh authentication.
	now = now.Add(snapshotLifetime)
	if err := s.CleanupTerminal(ctx, active); err != nil {
		t.Fatal(err)
	}
	// Assert: no TTL extension and no removal based solely on a cached timestamp.
	name, _ := snapshotName(request.RequestID)
	if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) || counter.opens != 2 {
		t.Fatal("cached image did not expire safely", err, counter.opens)
	}
}
