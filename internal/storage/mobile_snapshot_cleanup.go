package storage

import (
	"context"
	"encoding/hex"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// MobileHistorySnapshotTerminal authorizes local removal only. Collection
// revocation does not prevent cleanup, and no request/session authority is granted.
func (s *Store) MobileHistorySnapshotTerminal(ctx context.Context, requestID, fingerprint, accountDigest string) (bool, error) {
	if _, err := uuid.Parse(requestID); err != nil {
		return false, nil
	}
	for _, value := range []string{fingerprint, accountDigest} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			return false, nil
		}
	}
	account, err := historyAccount(ctx, s.DB)
	if err != nil {
		return false, err
	}
	if account != accountDigest {
		return false, nil
	}
	var terminal bool
	err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM history_operations o
 JOIN history_mobile_attempts l ON l.operation_id=o.operation_id
 JOIN mobile_backup_attempts a ON a.operation_id=l.attempt_id
 WHERE o.request_id=? AND a.request_id=o.request_id
 AND o.account_key=? AND a.account_key=o.account_key AND a.fingerprint=?
 AND coalesce(json_extract(o.payload,'$.source'),'')=?
 AND o.state IN ('completed','partial','cancelled','failed','unsupported'))`,
		requestID, account, fingerprint, domain.HistorySourceMobileArchive).Scan(&terminal)
	return terminal, err
}
