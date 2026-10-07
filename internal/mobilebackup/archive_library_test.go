package mobilebackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func TestArchiveLibrarySurvivesCacheExpiryAndRestart(t *testing.T) {
	// Arrange: a source captured in the past, with an independently controlled cache clock.
	ctx := context.Background()
	fixture := retainedFixture(t)
	cache, err := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "cache"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	captured := time.Now().Add(-48 * time.Hour).Truncate(time.Millisecond)
	cache.ledger.now = func() time.Time { return captured }
	original, err := cache.Save(ctx, retainedTestID, "123", fixture, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library")
	library, err := NewArchiveLibraryStore(path, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("123"))
	binding := hex.EncodeToString(hash[:])
	// Act: preserve while cache is valid, repeat, expire cache and restart library.
	first, err := library.PreserveFrom(ctx, cache, retainedTestID, binding)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := retainedName(retainedTestID)
	before, err := os.ReadFile(filepath.Join(path, name))
	if err != nil {
		t.Fatal(err)
	}
	retry, err := library.PreserveFrom(ctx, cache, retainedTestID, binding)
	if err != nil {
		t.Fatal(err)
	}
	cache.ledger.now = time.Now
	if _, _, err := cache.Read(ctx, retainedTestID, "123"); !errors.Is(err, ErrRetainedExpired) {
		t.Fatal("cache retention changed", err)
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
	status, err := library.PreservationStatus(ctx, retainedTestID, binding)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(path, name))
	// Assert: original provenance/mapping survived; no renewal or rewriting occurred.
	if !reflect.DeepEqual(first, retry) || !reflect.DeepEqual(first, status) || !reflect.DeepEqual(original, manifest) || !bytes.Equal(before, after) || len(read.archive.Files) != 3 || read.pairs[1].Session != "12" || !read.pairs[1].Group || string(read.archive.Files[2].Data) != "synthetic-c" {
		t.Fatal("durable source or provenance changed")
	}
	encoded, _ := json.Marshal(status)
	var value any
	json.Unmarshal(encoded, &value)
	schema, err := contracts.Compile("cli_preserve_account_archive", "output")
	if err != nil || schema.Validate(value) != nil {
		t.Fatal("receipt violates contract", err)
	}
	if _, err := library.Save(ctx, retainedTestID, "123", fixture, 0); err == nil {
		t.Fatal("library admitted direct acquisition")
	}
	foreign := sha256.Sum256([]byte("124"))
	if _, _, err := library.ReadBound(ctx, retainedTestID, hex.EncodeToString(foreign[:])); !errors.Is(err, ErrRetainedConflict) {
		t.Fatal("foreign owner accepted", err)
	}
	if err := library.RemoveBound(ctx, retainedTestID, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := library.PreservationStatus(ctx, retainedTestID, binding); err == nil {
		t.Fatal("removed source revived")
	}
}

func TestArchiveLibraryRejectsTamperingAndCancelledPromotion(t *testing.T) {
	// Arrange: a bound source and empty durable library.
	ctx := context.Background()
	a := retainedFixture(t)
	cache, err := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "cache"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if _, err = cache.Save(ctx, retainedTestID, "123", a, 0); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library")
	library, err := NewArchiveLibraryStore(path, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte("123"))
	binding := hex.EncodeToString(h[:])
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	// Act / Assert: cancellation publishes nothing; later ciphertext corruption fails closed.
	if _, err = library.PreserveFrom(cancelled, cache, retainedTestID, binding); err == nil || library.ledger.claims[retainedTestID] {
		t.Fatal("cancelled promotion published")
	}
	if _, err = library.PreserveFrom(ctx, cache, retainedTestID, binding); err != nil {
		t.Fatal(err)
	}
	library.Close()
	name, _ := retainedName(retainedTestID)
	file := filepath.Join(path, name)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if opened, err := NewArchiveLibraryStore(path, 8<<20); err == nil {
		opened.Close()
		t.Fatal("tampered library reopened")
	}
}
