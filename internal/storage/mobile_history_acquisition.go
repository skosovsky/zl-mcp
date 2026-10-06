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

// MobileHistoryAcquisition reads the permanent link without creating an attempt.
func (s *Store) MobileHistoryAcquisition(ctx context.Context, id string) (MobileBackupAttempt, error) {
	op, err := s.HistoryOperation(ctx, id)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if op.Status.Source != domain.HistorySourceMobileArchive {
		return MobileBackupAttempt{}, ErrHistoryState
	}
	var attemptID string
	if err = s.DB.QueryRowContext(ctx, "SELECT attempt_id FROM history_mobile_attempts WHERE operation_id=?", id).Scan(&attemptID); err != nil {
		return MobileBackupAttempt{}, err
	}
	return s.MobileBackupAttempt(ctx, attemptID)
}

// CompleteMobileHistoryWork reconciles one non-page stage without changing source
// counts or committing records. A page reconciles work in its own atomic commit.
func (s *Store) CompleteMobileHistoryWork(ctx context.Context, id string, revision int64, elapsed time.Duration) (HistoryOperation, error) {
	if elapsed < 0 || elapsed > MobileHistoryWorkBudget {
		return HistoryOperation{}, domain.Invalid("Invalid mobile history stage duration.")
	}
	started := time.Now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryOperation{}, err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return HistoryOperation{}, err
	}
	if op.Status.Source != domain.HistorySourceMobileArchive || op.Status.State != "running" || op.Revision != revision || op.ReservedDuration <= 0 || op.WorkDuration < 0 || op.WorkDuration > MobileHistoryWorkBudget {
		return HistoryOperation{}, ErrHistoryState
	}
	if !s.AllowsConversation(op.Status.Ref()) {
		historyStopped(&op, "cancelled", "access_revoked")
	} else {
		remaining := MobileHistoryWorkBudget - op.WorkDuration
		used := elapsed + time.Since(started)
		if remaining <= 0 || used >= remaining {
			op.WorkDuration = MobileHistoryWorkBudget
			historyStopped(&op, "partial", "time_limit")
		} else {
			op.WorkDuration += used
		}
	}
	op.ReservedDuration = 0
	if err = saveHistoryOperation(ctx, tx, &op); err != nil {
		return HistoryOperation{}, err
	}
	return commitHistoryResult(tx, op)
}

// CloseMobileHistoryAcquisition releases only an inactive linked history attempt.
// This account-owned cleanup grants no access, dispatch or retry and is allowed
// after scope revocation. Existing terminal acquisition evidence is preserved.
func (s *Store) CloseMobileHistoryAcquisition(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return err
	}
	if op.Status.Source != domain.HistorySourceMobileArchive || op.Status.State == "queued" || op.Status.State == "running" {
		return ErrHistoryState
	}
	state := "interrupted"
	if op.Status.State == "cancelled" {
		state = "cancelled"
	}
	_, err = tx.ExecContext(ctx, `UPDATE mobile_backup_attempts SET state=?,revision=revision+1,updated_at=?
 WHERE account_key=? AND operation_id=(SELECT attempt_id FROM history_mobile_attempts WHERE operation_id=?)
 AND state IN ('prepared','dispatching','waiting_for_confirmation','request_result_unknown','mobile_restoring','waiting_for_backup')`, state, now(), op.accountKey, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
