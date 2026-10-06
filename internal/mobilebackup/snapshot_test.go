package mobilebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func snapshotFixture(t *testing.T) (SelectedArchive, domain.MobileBackupRequest) {
	t.Helper()
	r, err := selectedFetchRequest().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		_, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", "13", "14", "private-synthetic-snapshot-message-marker", time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC).UnixMilli(), 0, 0, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
	})
	return SelectedArchive{requestID: r.RequestID, requestFingerprint: r.Fingerprint(), ref: r.Ref(), File: ArchiveFile{Name: "902.db", Data: data}}, r
}

func openSnapshotStore(t *testing.T, dir string, budget int64) *SnapshotStore {
	t.Helper()
	s, err := NewSnapshotStore(dir, budget)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSnapshotRestartBindingPrivacyAndIdempotency(t *testing.T) {
	// Arrange: the exact selected source and a private store, with no corpus or session.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1<<20)
	selected, r := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	// Act: publish, retry without re-encryption, then reopen like a service restart.
	if err := s.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	name, _ := snapshotName(r.RequestID)
	sealed, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	retry, _ := os.ReadFile(filepath.Join(dir, name))
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openSnapshotStore(t, dir, 1<<20)
	got, err := s.Read(ctx, r, "10")
	defer got.Clear()
	// Assert: source bytes/binding survive, ciphertext and TTL remain unchanged.
	if err != nil || !bytes.Equal(got.File.Data, selected.File.Data) || got.File.Name != selected.File.Name || got.ref != r.Ref() || got.requestFingerprint != r.Fingerprint() || !bytes.Equal(sealed, retry) {
		t.Fatal("snapshot restart or repeat changed source", err)
	}
	if bytes.Contains(sealed, []byte("private-synthetic-snapshot-message-marker")) || bytes.Contains(sealed, []byte("SQLite format 3")) {
		t.Fatal("plaintext source persisted")
	}
	for _, value := range []any{s, *s} {
		if fmt.Sprintf("%#v", value) != "mobile archive snapshot store [redacted]" {
			t.Fatal("store formatting exposed state")
		}
		encoded, _ := json.Marshal(value)
		if string(encoded) != "{}" {
			t.Fatal("store serialized private state")
		}
	}
	for _, file := range []string{name, snapshotKeyName, snapshotClaimsName} {
		info, e := os.Stat(filepath.Join(dir, file))
		if e != nil || info.Mode().Perm() != 0600 {
			t.Fatal("artifact permissions", e)
		}
	}
}

func TestSnapshotRejectsChangedAccountRequestAndSource(t *testing.T) {
	// Arrange.
	s := openSnapshotStore(t, filepath.Join(t.TempDir(), "snapshots"), 1<<20)
	selected, r := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	if err := s.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	// Act / Assert: neither references nor known UUIDs authorize foreign binding.
	for _, account := range []string{"11", "01"} {
		got, err := s.Read(ctx, r, account)
		if err == nil || len(got.File.Data) != 0 {
			t.Fatal("account escaped snapshot binding")
		}
	}
	changed := r
	changed.Since = "2026-09-02T00:00:00Z"
	got, err := s.Read(ctx, changed, "10")
	if !errors.Is(err, ErrSnapshotConflict) || len(got.File.Data) != 0 {
		t.Fatal("changed selection accepted")
	}
	copyOfSource := selected
	copyOfSource.File.Data = append([]byte(nil), selected.File.Data...)
	defer copyOfSource.Clear()
	copyOfSource.File.Data[len(copyOfSource.File.Data)-1] ^= 1
	if err = s.Save(ctx, copyOfSource, r, "10"); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatal("source replaced under same request", err)
	}
	if err = s.Remove(ctx, r, "11"); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatal("foreign account removed snapshot")
	}
	got, err = s.Read(ctx, r, "10")
	defer got.Clear()
	if err != nil || !bytes.Equal(got.File.Data, selected.File.Data) {
		t.Fatal("binding failures damaged source", err)
	}
}

func TestSnapshotExpiryRemovalAndSpentUUIDSurviveRestart(t *testing.T) {
	// Arrange: successful publication has a durable reservation independent of bytes.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1<<20)
	selected, r := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	now := time.Now()
	s.now = func() time.Time { return now }
	if err := s.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	// Act: expire and read, then restart after the source bytes have been removed.
	s.now = func() time.Time { return now.Add(snapshotLifetime) }
	got, err := s.Read(ctx, r, "10")
	if !errors.Is(err, ErrSnapshot) || len(got.File.Data) != 0 {
		t.Fatal("expired source returned")
	}
	name, _ := snapshotName(r.RequestID)
	if _, err = os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expired file retained")
	}
	s.Close()
	s = openSnapshotStore(t, dir, 1<<20)
	// Assert: saving an expired UUID cannot renew its lifetime after restart.
	if err = s.Save(ctx, selected, r, "10"); !errors.Is(err, ErrSnapshot) {
		t.Fatal("spent UUID recreated source", err)
	}
	fresh := r
	fresh.RequestID = uuid.NewString()
	selected.requestID = fresh.RequestID
	if err = s.Save(ctx, selected, fresh, "10"); err != nil {
		t.Fatal("new request unavailable", err)
	}
	if err = s.Remove(ctx, fresh, "10"); err != nil {
		t.Fatal(err)
	}
	if err = s.Save(ctx, selected, fresh, "10"); !errors.Is(err, ErrSnapshot) {
		t.Fatal("removed snapshot recreated")
	}
}

func TestSnapshotCorruptionAndMissingKeyFailWithoutRotation(t *testing.T) {
	for _, mode := range []string{"ciphertext", "key", "missing-key", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: valid encrypted snapshot, with an independently retained key image.
			dir := filepath.Join(t.TempDir(), "snapshots")
			s := openSnapshotStore(t, dir, 1<<20)
			selected, r := snapshotFixture(t)
			defer selected.Clear()
			ctx := context.Background()
			if err := s.Save(ctx, selected, r, "10"); err != nil {
				t.Fatal(err)
			}
			key, _ := os.ReadFile(filepath.Join(dir, snapshotKeyName))
			defer clear(key)
			s.Close()
			name, _ := snapshotName(r.RequestID)
			// Act: corrupt an owned artifact or replace it by an outside symlink.
			switch mode {
			case "ciphertext":
				path := filepath.Join(dir, name)
				data, _ := os.ReadFile(path)
				data[len(data)-1] ^= 1
				os.WriteFile(path, data, 0600)
			case "key":
				os.WriteFile(filepath.Join(dir, snapshotKeyName), []byte("broken"), 0600)
			case "missing-key":
				os.Remove(filepath.Join(dir, snapshotKeyName))
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				os.WriteFile(outside, []byte("unrelated private data"), 0600)
				os.Remove(filepath.Join(dir, name))
				if err := os.Symlink(outside, filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
			got, err := NewSnapshotStore(dir, 1<<20)
			// Assert: no constructor success, repair or implicit key rotation.
			if err == nil || got != nil {
				if got != nil {
					got.Close()
				}
				t.Fatal("corrupt source/key accepted")
			}
			if mode == "ciphertext" || mode == "symlink" {
				after, _ := os.ReadFile(filepath.Join(dir, snapshotKeyName))
				defer clear(after)
				if !bytes.Equal(key, after) {
					t.Fatal("key rotated")
				}
			}
			if mode == "missing-key" {
				if _, err = os.Stat(filepath.Join(dir, snapshotKeyName)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing key recreated")
				}
			}
		})
	}
}

func TestSnapshotCapacityCancellationAndInterruptedCleanup(t *testing.T) {
	// Arrange: a budget too small for one image must not reserve its UUID.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1)
	selected, r := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	// Act / Assert.
	if err := s.Save(ctx, selected, r, "10"); !errors.Is(err, ErrSnapshotCapacity) {
		t.Fatal("byte quota exceeded", err)
	}
	if len(s.claims) != 0 {
		t.Fatal("capacity rejection spent UUID")
	}
	s.budget = 1 << 20
	if !s.lock(ctx) {
		t.Fatal("cannot hold store ownership")
	}
	request, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := s.Read(request, r, "10"); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrSnapshot) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		s.unlock()
		t.Fatal("cancelled read waited for ownership")
	}
	s.unlock()
	if err := s.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	// Arrange: an interrupted private temporary write and an unrelated outside file.
	tmp := uuid.NewString() + ".tmp"
	if err := os.WriteFile(filepath.Join(dir, tmp), []byte("unfinished ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openSnapshotStore(t, dir, 1<<20)
	// Assert: restart removes only the owned temporary artifact; source remains.
	if _, err := os.Stat(filepath.Join(dir, tmp)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted temporary artifact retained")
	}
	got, err := s.Read(ctx, r, "10")
	defer got.Clear()
	if err != nil || len(got.File.Data) == 0 {
		t.Fatal("cleanup damaged snapshot", err)
	}
}

func TestSnapshotCountLimitAndIncompleteReservationSurviveRestart(t *testing.T) {
	// Arrange: fill the configured snapshot count below the byte quota.
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := openSnapshotStore(t, dir, 1<<20)
	selected, r := snapshotFixture(t)
	defer selected.Clear()
	ctx := context.Background()
	for i := 0; i < maxSnapshots; i++ {
		r.RequestID = uuid.NewString()
		selected.requestID = r.RequestID
		if err := s.Save(ctx, selected, r, "10"); err != nil {
			t.Fatal(err)
		}
	}
	fullRequest := r
	fullRequest.RequestID = uuid.NewString()
	selected.requestID = fullRequest.RequestID
	// Act: exceed the count, then simulate a crash after durable reservation.
	if err := s.Save(ctx, selected, fullRequest, "10"); !errors.Is(err, ErrSnapshotCapacity) {
		t.Fatal("snapshot count limit bypassed", err)
	}
	if s.claims[fullRequest.RequestID] {
		t.Fatal("count rejection reserved a request")
	}
	if err := s.claim(fullRequest.RequestID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openSnapshotStore(t, dir, 1<<20)
	// Assert: an interrupted publication cannot recreate or renew its source.
	if err := s.Save(ctx, selected, fullRequest, "10"); !errors.Is(err, ErrSnapshot) {
		t.Fatal("interrupted reservation allowed a replacement source", err)
	}
	got, err := s.Read(ctx, fullRequest, "10")
	if !errors.Is(err, ErrSnapshot) || len(got.File.Data) != 0 {
		t.Fatal("unpublished source became readable")
	}
}

func TestSnapshotRejectsTruncatedClaimsAndUnownedArtifacts(t *testing.T) {
	for _, mode := range []string{"truncated-claims", "duplicate-claims", "unknown-file", "key-permissions"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: a valid store with one reserved source.
			dir := filepath.Join(t.TempDir(), "snapshots")
			s := openSnapshotStore(t, dir, 1<<20)
			selected, r := snapshotFixture(t)
			defer selected.Clear()
			if err := s.Save(context.Background(), selected, r, "10"); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			// Act: damage the reservation boundary or introduce unowned data.
			var err error
			switch mode {
			case "truncated-claims":
				err = os.WriteFile(filepath.Join(dir, snapshotClaimsName), []byte(r.RequestID), 0600)
			case "duplicate-claims":
				err = os.WriteFile(filepath.Join(dir, snapshotClaimsName), []byte(r.RequestID+"\n"+r.RequestID+"\n"), 0600)
			case "unknown-file":
				err = os.WriteFile(filepath.Join(dir, "unrelated-data"), []byte("preserve me"), 0600)
			case "key-permissions":
				err = os.Chmod(filepath.Join(dir, snapshotKeyName), 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := NewSnapshotStore(dir, 1<<20)
			// Assert: no implicit repair; unrelated data survives rejected startup.
			if err == nil || got != nil {
				if got != nil {
					got.Close()
				}
				t.Fatal("invalid ownership boundary accepted")
			}
			if mode == "unknown-file" {
				data, e := os.ReadFile(filepath.Join(dir, "unrelated-data"))
				if e != nil || string(data) != "preserve me" {
					t.Fatal("unrelated file removed", e)
				}
			}
		})
	}
}
