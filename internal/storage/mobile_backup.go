package storage

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

var ErrMobileBackupConflict = errors.New("mobile backup request conflicts with original arguments")
var ErrMobileBackupBusy = errors.New("mobile backup attempt already active")
var ErrMobileBackupState = errors.New("mobile backup state or revision changed")
var ErrMobileBackupCapacity = errors.New("mobile backup ledger capacity reached")

type MobileBackupAttempt struct {
	Request     domain.MobileBackupRequest `json:"request"`
	OperationID string                     `json:"operation_id"`
	State       string                     `json:"state"`
	Revision    int64                      `json:"revision"`
	CreatedAt   string                     `json:"created_at"`
	UpdatedAt   string                     `json:"updated_at"`
	PublicKey   string                     `json:"-"`
	account     string
}

func (MobileBackupAttempt) String() string   { return "mobile backup attempt [redacted]" }
func (MobileBackupAttempt) GoString() string { return "mobile backup attempt [redacted]" }

func (s *Store) migrateMobileBackup(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS mobile_backup_attempts(
 operation_id TEXT PRIMARY KEY,request_id TEXT UNIQUE NOT NULL,account_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('prepared','dispatching','waiting_for_confirmation','request_result_unknown','mobile_restoring','waiting_for_backup','offer_ready','failed','cancelled','interrupted')),
 public_key TEXT NOT NULL DEFAULT '',revision INTEGER NOT NULL DEFAULT 0,request TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS mobile_backup_active_account ON mobile_backup_attempts(account_key)
 WHERE state IN ('prepared','dispatching','waiting_for_confirmation','request_result_unknown','mobile_restoring','waiting_for_backup');
 INSERT OR IGNORE INTO schema_migrations VALUES(9);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) loadMobileBackup(ctx context.Context, q historyReader, id string) (MobileBackupAttempt, error) {
	var a MobileBackupAttempt
	var body string
	err := q.QueryRowContext(ctx, "SELECT operation_id,account_key,state,public_key,revision,request,created_at,updated_at FROM mobile_backup_attempts WHERE operation_id=?", id).Scan(&a.OperationID, &a.account, &a.State, &a.PublicKey, &a.Revision, &body, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	if json.Unmarshal([]byte(body), &a.Request) != nil {
		return MobileBackupAttempt{}, ErrMobileBackupState
	}
	account, err := historyAccount(ctx, q)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if account != a.account || (a.Request.ArchiveScope != "account" && !s.AllowsConversation(a.Request.Ref())) {
		return MobileBackupAttempt{}, subscriptionPermission("Mobile backup attempt is outside account or collection policy.")
	}
	return a, nil
}
func (s *Store) MobileBackupAttempt(ctx context.Context, id string) (MobileBackupAttempt, error) {
	return s.loadMobileBackup(ctx, s.DB, id)
}

func (s *Store) PrepareMobileBackup(ctx context.Context, request domain.MobileBackupRequest) (MobileBackupAttempt, error) {
	r, err := request.Normalize()
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if r.ArchiveScope != "account" && !s.AllowsConversation(r.Ref()) {
		return MobileBackupAttempt{}, subscriptionPermission("Conversation is outside collection policy.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	defer tx.Rollback()
	a, err := s.prepareMobileBackupTx(ctx, tx, r, false)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if err = tx.Commit(); err != nil {
		return MobileBackupAttempt{}, err
	}
	return a, nil
}

func (s *Store) prepareMobileBackupTx(ctx context.Context, tx *sql.Tx, r domain.MobileBackupRequest, rejectExisting bool) (MobileBackupAttempt, error) {
	account, err := historyAccount(ctx, tx)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	var id, fingerprint string
	err = tx.QueryRowContext(ctx, "SELECT operation_id,fingerprint FROM mobile_backup_attempts WHERE request_id=?", r.RequestID).Scan(&id, &fingerprint)
	if err == nil {
		if rejectExisting {
			return MobileBackupAttempt{}, ErrMobileBackupConflict
		}
		original, err := s.loadMobileBackup(ctx, tx, id)
		if err != nil {
			return MobileBackupAttempt{}, err
		}
		if fingerprint != r.Fingerprint() {
			return MobileBackupAttempt{}, ErrMobileBackupConflict
		}
		return original, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MobileBackupAttempt{}, err
	}
	var total, active int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM mobile_backup_attempts").Scan(&total); err != nil {
		return MobileBackupAttempt{}, err
	}
	if total >= 10000 {
		return MobileBackupAttempt{}, ErrMobileBackupCapacity
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM mobile_backup_attempts WHERE account_key=? AND state IN ('prepared','dispatching','waiting_for_confirmation','request_result_unknown','mobile_restoring','waiting_for_backup')", account).Scan(&active); err != nil {
		return MobileBackupAttempt{}, err
	}
	if active > 0 {
		return MobileBackupAttempt{}, ErrMobileBackupBusy
	}
	stamp := now()
	a := MobileBackupAttempt{Request: r, OperationID: uuid.NewString(), State: "prepared", CreatedAt: stamp, UpdatedAt: stamp, account: account}
	body, err := json.Marshal(r)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO mobile_backup_attempts(operation_id,request_id,account_key,fingerprint,state,request,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", a.OperationID, r.RequestID, account, r.Fingerprint(), a.State, string(body), stamp, stamp)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	return a, nil
}

func (s *Store) DispatchMobileBackup(ctx context.Context, id string, revision int64, public string) (MobileBackupAttempt, error) {
	if public == "" || len(public) > 1024 {
		return MobileBackupAttempt{}, ErrMobileBackupState
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(public)
	if err != nil || len(public) > 1024 || len(decoded) == 0 || base64.StdEncoding.EncodeToString(decoded) != public {
		return MobileBackupAttempt{}, ErrMobileBackupState
	}
	key, err := x509.ParsePKIXPublicKey(decoded)
	if err != nil {
		return MobileBackupAttempt{}, ErrMobileBackupState
	}
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok || rsaKey.N.BitLen() != 2048 || rsaKey.E != 65537 {
		return MobileBackupAttempt{}, ErrMobileBackupState
	}
	return s.transitionMobileBackup(ctx, id, revision, "dispatching", public)
}
func (s *Store) ProgressMobileBackup(ctx context.Context, id string, revision int64, state string) (MobileBackupAttempt, error) {
	return s.transitionMobileBackup(ctx, id, revision, state, "")
}

func mobileBackupTransition(from, to string) bool {
	if to == "failed" || to == "cancelled" || to == "interrupted" {
		return from == "prepared" || from == "dispatching" || from == "waiting_for_confirmation" || from == "request_result_unknown" || from == "mobile_restoring" || from == "waiting_for_backup"
	}
	if from == "prepared" {
		return to == "dispatching"
	}
	if from == "dispatching" {
		return to == "waiting_for_confirmation" || to == "request_result_unknown"
	}
	if from == "waiting_for_confirmation" || from == "request_result_unknown" || from == "mobile_restoring" || from == "waiting_for_backup" {
		return to == "mobile_restoring" || to == "waiting_for_backup" || to == "offer_ready"
	}
	return false
}
func (s *Store) transitionMobileBackup(ctx context.Context, id string, revision int64, state, public string) (MobileBackupAttempt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	defer tx.Rollback()
	a, err := s.loadMobileBackup(ctx, tx, id)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if a.Revision != revision || !mobileBackupTransition(a.State, state) || state == "dispatching" && public == "" {
		return MobileBackupAttempt{}, ErrMobileBackupState
	}
	if state == "dispatching" {
		if err = s.checkMobileHistoryDispatch(ctx, tx, id); err != nil {
			return MobileBackupAttempt{}, err
		}
	}
	if public != "" {
		a.PublicKey = public
	}
	a.State = state
	a.UpdatedAt = now()
	result, err := tx.ExecContext(ctx, "UPDATE mobile_backup_attempts SET state=?,public_key=?,revision=revision+1,updated_at=? WHERE operation_id=? AND revision=? AND account_key=?", a.State, a.PublicKey, a.UpdatedAt, id, revision, a.account)
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return MobileBackupAttempt{}, err
	}
	if n != 1 {
		return MobileBackupAttempt{}, ErrMobileBackupState
	}
	a.Revision++
	if err = tx.Commit(); err != nil {
		return MobileBackupAttempt{}, err
	}
	return a, nil
}

// RecoverMobileBackupAttempts is called explicitly before an operation worker starts.
// It never performs network work or makes interrupted UUIDs dispatchable again.
func (s *Store) RecoverMobileBackupAttempts(ctx context.Context) error {
	account, err := historyAccount(ctx, s.DB)
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if e := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM mobile_backup_attempts").Scan(&count); e != nil {
			return e
		}
		if count == 0 {
			return nil
		}
		return ErrMobileBackupState
	}
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "UPDATE mobile_backup_attempts SET state='interrupted',revision=revision+1,updated_at=? WHERE account_key=? AND state IN ('prepared','dispatching','waiting_for_confirmation','request_result_unknown','mobile_restoring','waiting_for_backup')", now(), account)
	return err
}

// MobileBackupObserver binds serial adapter callbacks to the durable revision.
// It does not dispatch, retry or recover an operation. Callers must finish failed
// attempts explicitly; restart recovery preserves ambiguous dispatches.
func (s *Store) MobileBackupObserver(ctx context.Context, id string) (*domain.MobileBackupObserver, error) {
	attempt, err := s.MobileBackupAttempt(ctx, id)
	if err != nil {
		return nil, err
	}
	if attempt.State != "prepared" {
		return nil, ErrMobileBackupState
	}
	var mu sync.Mutex
	return &domain.MobileBackupObserver{
		BeforeDispatch: func(public string) error {
			mu.Lock()
			defer mu.Unlock()
			next, err := s.DispatchMobileBackup(ctx, id, attempt.Revision, public)
			if err == nil {
				attempt = next
			}
			return err
		},
		Progress: func(state string) error {
			mu.Lock()
			defer mu.Unlock()
			next, err := s.ProgressMobileBackup(ctx, id, attempt.Revision, state)
			if err == nil {
				attempt = next
			}
			return err
		},
	}, nil
}
