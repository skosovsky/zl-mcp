package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestReadIdentityPreservesEmptyPolicyBytes(t *testing.T) {
	// Arrange: the historical empty selected policy hashes an array, not null.
	ctx := context.Background()
	store, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	expected := sha256.Sum256([]byte(`{"All":false,"Selected":[]}`))
	// Act.
	_, revision, err := store.ReadIdentity(ctx)
	// Assert: field-name/default changes must not silently change persisted read authority.
	if err != nil || revision != hex.EncodeToString(expected[:]) {
		t.Fatalf("identity changed: %s, %v", revision, err)
	}
}
