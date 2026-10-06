package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) migrateMobileHistoryAcquisition(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS history_mobile_attempts(
 operation_id TEXT PRIMARY KEY REFERENCES history_operations(operation_id),
 attempt_id TEXT UNIQUE NOT NULL REFERENCES mobile_backup_attempts(operation_id),
 dispatch_revision INTEGER NOT NULL CHECK(dispatch_revision>=0));
 INSERT OR IGNORE INTO schema_migrations VALUES(14);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// PrepareMobileHistoryAcquisition binds exactly one attempt before dispatch.
// Restart/retry returns that same attempt, never a replacement phone request.
func (s *Store) PrepareMobileHistoryAcquisition(ctx context.Context, id string, revision int64, maxArchiveBytes int64) (MobileBackupAttempt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if op.Revision != revision || op.Status.State != "running" || op.Status.Source != domain.HistorySourceMobileArchive {
		return MobileBackupAttempt{}, ErrHistoryState
	}
	if !s.AllowsConversation(op.Status.Ref()) {
		return MobileBackupAttempt{}, subscriptionPermission("Conversation is outside collection policy.")
	}
	r, err := (domain.MobileBackupRequest{RequestID: op.Status.RequestID, ConversationType: op.Status.ConversationType, ConversationID: op.Status.ConversationID, Since: op.Status.Since, Until: op.Status.Until, MaxMessages: op.Status.MaxMessages, MaxArchiveBytes: maxArchiveBytes}).Normalize()
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	var attemptID string
	err = tx.QueryRowContext(ctx, "SELECT attempt_id FROM history_mobile_attempts WHERE operation_id=?", id).Scan(&attemptID)
	if err == nil {
		attempt, e := s.loadMobileBackup(ctx, tx, attemptID)
		if e != nil {
			return MobileBackupAttempt{}, e
		}
		if attempt.Request.RequestID != r.RequestID || attempt.Request.Fingerprint() != r.Fingerprint() {
			return MobileBackupAttempt{}, ErrMobileBackupConflict
		}
		return attempt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MobileBackupAttempt{}, err
	}
	if op.ReservedDuration <= 0 || op.ReservedDuration > 180*time.Second {
		return MobileBackupAttempt{}, ErrHistoryState
	}
	attempt, err := s.prepareMobileBackupTx(ctx, tx, r, true)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO history_mobile_attempts(operation_id,attempt_id,dispatch_revision) VALUES(?,?,?)", id, attempt.OperationID, revision)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if err = tx.Commit(); err != nil {
		return MobileBackupAttempt{}, err
	}
	return attempt, nil
}

func (s *Store) checkMobileHistoryDispatch(ctx context.Context, tx *sql.Tx, attemptID string) error {
	var id string
	var revision int64
	err := tx.QueryRowContext(ctx, "SELECT operation_id,dispatch_revision FROM history_mobile_attempts WHERE attempt_id=?", attemptID).Scan(&id, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // Existing owner-only diagnostic attempt, not a history acquisition.
	}
	if err != nil {
		return err
	}
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return err
	}
	if op.Status.Source != domain.HistorySourceMobileArchive || op.Status.State != "running" || op.Revision != revision || op.ReservedDuration <= 0 || op.ReservedDuration > 180*time.Second {
		return ErrHistoryState
	}
	if !s.AllowsConversation(op.Status.Ref()) {
		return subscriptionPermission("Conversation is outside collection policy.")
	}
	return nil
}
