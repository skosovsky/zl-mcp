package mobilebackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArchiveInventoryAuthenticatesOwnerAndSurvivesRestart(t *testing.T) {
	// Arrange: one authenticated synthetic source, a permanent library and no network.
	ctx := context.Background()
	cache, err := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "cache"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	_, err = cache.Save(ctx, retainedTestID, "123", retainedFixture(t), 0)
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
	if _, err = library.Sources(ctx, ""); err == nil {
		t.Fatal("empty owner accepted")
	}
	receipt, err := library.PreserveFrom(ctx, cache, retainedTestID, binding)
	if err != nil {
		t.Fatal(err)
	}
	// Act: read inventory, restart the permanent library, then read it again.
	first, err := library.Sources(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	library.Close()
	library, err = NewArchiveLibraryStore(path, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	second, err := library.Sources(ctx, binding)
	// Assert: original receipt remains identical; foreign owners get no partial inventory.
	if err != nil || !reflect.DeepEqual(first, []PreservationReceipt{receipt}) || !reflect.DeepEqual(first, second) {
		t.Fatal("inventory changed", err)
	}
	foreign := sha256.Sum256([]byte("124"))
	if values, err := library.Sources(ctx, hex.EncodeToString(foreign[:])); err == nil || values != nil {
		t.Fatal("foreign source disclosed")
	}
}

func TestArchiveTokensBindPurposeAndPersistentLibraryKey(t *testing.T) {
	// Arrange: private synthetic read position and an independent library.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "library")
	library, err := NewArchiveLibraryStore(path, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	token, err := library.SealArchiveToken(ctx, "cursor", []byte("synthetic private position"))
	if err != nil {
		t.Fatal(err)
	}
	library.Close()
	library, err = NewArchiveLibraryStore(path, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	// Act: reopen valid cursor, substitute purpose, tamper ciphertext and cross libraries.
	body, err := library.OpenArchiveToken(ctx, "cursor", token)
	// Assert: only the original purpose/key authenticates; cancellation yields no bytes.
	if err != nil || string(body) != "synthetic private position" {
		t.Fatal("restart invalidated token", err)
	}
	clear(body)
	if body, err = library.OpenArchiveToken(ctx, "resource", token); err == nil || body != nil {
		t.Fatal("purpose substitution accepted")
	}
	modified := []byte(token)
	if modified[0] == 'A' {
		modified[0] = 'B'
	} else {
		modified[0] = 'A'
	}
	if body, err = library.OpenArchiveToken(ctx, "cursor", string(modified)); err == nil || body != nil {
		t.Fatal("tampered token accepted")
	}
	other, err := NewArchiveLibraryStore(filepath.Join(t.TempDir(), "other"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if body, err = other.OpenArchiveToken(ctx, "cursor", token); err == nil || body != nil {
		t.Fatal("foreign library accepted token")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if body, err = library.OpenArchiveToken(cancelled, "cursor", token); err == nil || body != nil {
		t.Fatal("cancelled read returned data")
	}
	if _, err = library.SealArchiveToken(ctx, "unknown", []byte("position")); err == nil {
		t.Fatal("unknown purpose accepted")
	}
}
