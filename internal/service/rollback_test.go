package service

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestRollbackCommandRequiresStoppedServiceAndReadsOnly(t *testing.T) {
	// Arrange: owner command targets a temporary database, never authenticates.
	ctx := context.Background()
	dir := t.TempDir()
	s, err := storage.Open(ctx, filepath.Join(dir, "messages.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	unlock, err := local.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	// Act / Assert: running service exclusion is enforced with the same account lock.
	if err := checkLegacyRollbackCommand(ctx, dir, &output); err == nil {
		t.Fatal("running service accepted")
	}
	unlock()
	if err := checkLegacyRollbackCommand(ctx, dir, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"compatible":true`)) {
		t.Fatal("missing safe rollback report")
	}
}
