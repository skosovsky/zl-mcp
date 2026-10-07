package storage

import (
	"context"
	"encoding/hex"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ArchiveAccountKey returns only the local immutable account binding. Offline
// owner archive reads do not need a live Zalo session or credentials.
func (s *Store) ArchiveAccountKey(ctx context.Context) (string, error) {
	key, err := historyAccount(ctx, s.DB)
	decoded, e := hex.DecodeString(key)
	if err != nil || e != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != key {
		return "", domain.Invalid("Archive account binding is unavailable.")
	}
	return key, nil
}
