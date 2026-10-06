package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

var (
	ErrHistoryConflict = errors.New("history request conflicts with original arguments")
	ErrHistoryBusy     = errors.New("conversation already has an active history operation")
	ErrHistoryCapacity = errors.New("history operation ledger capacity reached")
	ErrHistoryState    = errors.New("history operation state or revision changed")
)

const HistoryWorkBudget = 120 * time.Second
const MobileHistoryWorkBudget = 420 * time.Second

func historyWorkBudget(op HistoryOperation) time.Duration {
	if op.Status.Source == domain.HistorySourceMobileArchive {
		return MobileHistoryWorkBudget
	}
	return HistoryWorkBudget
}

type HistoryOperation struct {
	Status           domain.HistoryImportStatus
	Cursor           string
	Revision         int64
	WorkDuration     time.Duration
	ReservedDuration time.Duration
	accountKey       string
}

func (s *Store) migrateHistoryOperations(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS history_operations(
 operation_id TEXT PRIMARY KEY,request_id TEXT UNIQUE NOT NULL,account_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 conversation_type TEXT NOT NULL,conversation_id TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('queued','running','paused','completed','partial','cancelled','failed','unsupported')),
 cursor TEXT NOT NULL,revision INTEGER NOT NULL DEFAULT 0,work_ns INTEGER NOT NULL DEFAULT 0,reserved_ns INTEGER NOT NULL DEFAULT 0,payload TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS history_active_conversation ON history_operations(conversation_type,conversation_id)
 WHERE state IN ('queued','running','paused');
 CREATE TABLE IF NOT EXISTS history_cursors(operation_id TEXT NOT NULL REFERENCES history_operations(operation_id),cursor TEXT NOT NULL,PRIMARY KEY(operation_id,cursor));
 INSERT OR IGNORE INTO schema_migrations VALUES(8);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type historyReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func historyAccount(ctx context.Context, q historyReader) (string, error) {
	var account string
	if err := q.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='account'").Scan(&account); err != nil {
		return "", err
	}
	return account, nil
}

func loadHistoryOperation(ctx context.Context, q historyReader, id string) (HistoryOperation, error) {
	var op HistoryOperation
	var body string
	err := q.QueryRowContext(ctx, "SELECT payload,cursor,revision,work_ns,reserved_ns,account_key FROM history_operations WHERE operation_id=?", id).Scan(&body, &op.Cursor, &op.Revision, &op.WorkDuration, &op.ReservedDuration, &op.accountKey)
	if err != nil {
		return op, err
	}
	if err = json.Unmarshal([]byte(body), &op.Status); err != nil {
		return op, err
	}
	account, err := historyAccount(ctx, q)
	if err != nil {
		return op, err
	}
	if account != op.accountKey {
		return HistoryOperation{}, subscriptionPermission("History operation belongs to a different account.")
	}
	return op, nil
}

func saveHistoryOperation(ctx context.Context, tx *sql.Tx, op *HistoryOperation) error {
	op.Status.UpdatedAt = now()
	body, err := json.Marshal(op.Status)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE history_operations SET state=?,cursor=?,revision=revision+1,work_ns=?,reserved_ns=?,payload=? WHERE operation_id=? AND revision=? AND account_key=?", op.Status.State, op.Cursor, int64(op.WorkDuration), int64(op.ReservedDuration), string(body), op.Status.OperationID, op.Revision, op.accountKey)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrHistoryState
	}
	op.Revision++
	return nil
}

func commitHistoryResult(tx *sql.Tx, op HistoryOperation) (HistoryOperation, error) {
	if err := tx.Commit(); err != nil {
		return HistoryOperation{}, err
	}
	return op, nil
}

func (s *Store) PrepareHistoryOperation(ctx context.Context, request domain.HistoryImportRequest) (HistoryOperation, error) {
	return s.prepareHistoryOperation(ctx, request, false)
}

// PrepareMobileHistoryOperation is an internal driver port, not a public source selector.
func (s *Store) PrepareMobileHistoryOperation(ctx context.Context, request domain.HistoryImportRequest) (HistoryOperation, error) {
	return s.prepareHistoryOperation(ctx, request, true)
}

func (s *Store) prepareHistoryOperation(ctx context.Context, request domain.HistoryImportRequest, mobile bool) (HistoryOperation, error) {
	if mobile {
		if request.Source != "" && request.Source != domain.HistorySourceMobileArchive {
			return HistoryOperation{}, domain.Invalid("Invalid mobile history source.")
		}
		request.Source = ""
	}
	r, err := request.Normalize()
	if err != nil {
		return HistoryOperation{}, err
	}
	if mobile {
		r.Source = domain.HistorySourceMobileArchive
	}
	if !s.AllowsConversation(r.Ref()) {
		return HistoryOperation{}, subscriptionPermission("Conversation is outside collection policy.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryOperation{}, err
	}
	defer tx.Rollback()
	account, err := historyAccount(ctx, tx)
	if err != nil {
		return HistoryOperation{}, err
	}
	var id, fingerprint string
	err = tx.QueryRowContext(ctx, "SELECT operation_id,fingerprint FROM history_operations WHERE request_id=?", r.RequestID).Scan(&id, &fingerprint)
	if err == nil {
		if fingerprint != r.Fingerprint() {
			return HistoryOperation{}, ErrHistoryConflict
		}
		return loadHistoryOperation(ctx, tx, id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return HistoryOperation{}, err
	}
	var total, active, busy int
	if err = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(state IN ('queued','running','paused')),0) FROM history_operations").Scan(&total, &active); err != nil {
		return HistoryOperation{}, err
	}
	if total >= 100000 || active >= 100 {
		return HistoryOperation{}, ErrHistoryCapacity
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM history_operations WHERE conversation_type=? AND conversation_id=? AND state IN ('queued','running','paused')", r.ConversationType, r.ConversationID).Scan(&busy); err != nil {
		return HistoryOperation{}, err
	}
	if busy > 0 {
		return HistoryOperation{}, ErrHistoryBusy
	}
	stamp := now()
	status := domain.HistoryImportStatus{HistoryImportRequest: r, OperationID: uuid.NewString(), NotificationPolicy: "none", SourceKind: "group_cloud", State: "queued", CreatedAt: stamp, UpdatedAt: stamp}
	if mobile {
		status.SourceKind = domain.HistorySourceMobileArchive
	} else if r.Source == "conversation_preload" {
		status.SourceKind = "conversation_preload"
	} else if r.ConversationType == domain.ConversationDirect {
		status.State, status.SourceKind = "unsupported", "unsupported"
		reason := "source_unsupported"
		status.StopReason = &reason
	}
	body, err := json.Marshal(status)
	if err != nil {
		return HistoryOperation{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO history_operations(operation_id,request_id,account_key,fingerprint,conversation_type,conversation_id,state,cursor,payload) VALUES(?,?,?,?,?,?,?,'0',?)", status.OperationID, r.RequestID, account, r.Fingerprint(), r.ConversationType, r.ConversationID, status.State, string(body)); err != nil {
		return HistoryOperation{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO history_cursors VALUES(?,'0')", status.OperationID); err != nil {
		return HistoryOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return HistoryOperation{}, err
	}
	return HistoryOperation{Status: status, Cursor: "0", accountKey: account}, nil
}

func (s *Store) HistoryOperation(ctx context.Context, id string) (HistoryOperation, error) {
	op, err := loadHistoryOperation(ctx, s.DB, id)
	if err == nil && !s.AllowsConversation(op.Status.Ref()) {
		err = subscriptionPermission("Conversation is outside collection policy.")
	}
	return op, err
}

func historyStopped(op *HistoryOperation, state, reason string) {
	op.Status.State = state
	op.Status.StopReason = &reason
}

func (s *Store) ClaimHistoryOperation(ctx context.Context, id string, revision int64) (HistoryOperation, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryOperation{}, err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return op, err
	}
	if op.Revision != revision || (op.Status.State != "queued" && op.Status.State != "paused") {
		return op, ErrHistoryState
	}
	if !s.AllowsConversation(op.Status.Ref()) {
		historyStopped(&op, "cancelled", "access_revoked")
	} else {
		op.Status.State, op.Status.StopReason = "running", nil
	}
	if err = saveHistoryOperation(ctx, tx, &op); err != nil {
		return op, err
	}
	return commitHistoryResult(tx, op)
}

func (s *Store) CancelHistoryOperation(ctx context.Context, id string) (HistoryOperation, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryOperation{}, err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return op, err
	}
	if op.Status.State == "queued" || op.Status.State == "running" || op.Status.State == "paused" {
		historyStopped(&op, "cancelled", "cancelled")
		if err = saveHistoryOperation(ctx, tx, &op); err != nil {
			return op, err
		}
	}
	return commitHistoryResult(tx, op)
}

func exactHistoryCursor(v string) bool {
	if len(v) == 0 || len(v) > 128 || (len(v) > 1 && v[0] == '0') {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// CommitHistoryOperationPage atomically imports records and advances the saved
// checkpoint. Stale workers cannot persist after cancellation or another claim.
func (s *Store) CommitHistoryOperationPage(ctx context.Context, id string, revision int64, page domain.HistoryPage, elapsed time.Duration) (HistoryOperation, error) {
	return s.commitHistoryOperationPage(ctx, id, revision, page, nil, elapsed)
}

// CommitExpiringHistoryOperationPage derives messages from the trusted records.
// TTL evidence and checkpoint share the operation's ownership/revision transaction.
// This adds no public source and performs no archive request or download.
func (s *Store) CommitExpiringHistoryOperationPage(ctx context.Context, id string, revision int64, page domain.HistoryPage, records []ExpiringHistoryRecord, elapsed time.Duration) (HistoryOperation, error) {
	if len(page.Messages) != 0 {
		return HistoryOperation{}, domain.Invalid("Provide records without a second message list.")
	}
	if len(records) > 50 {
		return HistoryOperation{}, domain.Invalid("History requires at most 50 records.")
	}
	page.Messages = make([]domain.Message, len(records))
	for i, r := range records {
		page.Messages[i] = r.Message
	}
	if records == nil {
		records = []ExpiringHistoryRecord{}
	}
	return s.commitHistoryOperationPage(ctx, id, revision, page, records, elapsed)
}

func (s *Store) commitHistoryOperationPage(ctx context.Context, id string, revision int64, page domain.HistoryPage, records []ExpiringHistoryRecord, elapsed time.Duration) (HistoryOperation, error) {
	started := time.Now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryOperation{}, err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return op, err
	}
	if op.Revision != revision || op.Status.State != "running" || op.Status.Source == domain.HistorySourceMobileArchive {
		return op, ErrHistoryState
	}
	if !s.AllowsConversation(op.Status.Ref()) {
		historyStopped(&op, "cancelled", "access_revoked")
	} else {
		if (op.Status.Source == "conversation_preload") != page.LimitedSnapshot {
			return op, domain.Invalid("History source evidence does not match selected source.")
		}
		if err = s.validateHistoryPage(op.Status.Ref(), page.Messages); err != nil {
			return op, err
		}
		if records != nil {
			if err = s.validateExpiringHistoryPage(op.Status.Ref(), records); err != nil {
				return op, err
			}
		}
		if elapsed < 0 || elapsed > HistoryWorkBudget || len(page.Messages) > op.Status.PageSize || len(page.Messages) > op.Status.MaxMessages-op.Status.RecordsObserved || op.Status.PagesObserved >= op.Status.MaxPages || (page.Cursor != nil && !exactHistoryCursor(*page.Cursor)) || (page.JoinTimestampMillis != nil && !exactHistoryCursor(*page.JoinTimestampMillis)) {
			return op, domain.Invalid("History page exceeds operation bounds or contains invalid continuation evidence.")
		}
		op.WorkDuration += elapsed
		op.ReservedDuration = 0
		if op.WorkDuration > HistoryWorkBudget {
			historyStopped(&op, "partial", "time_limit")
		} else if err = s.commitHistoryRecords(ctx, tx, &op, page, records); err != nil {
			return HistoryOperation{}, err
		}
		op.WorkDuration += time.Since(started)
		if op.WorkDuration >= HistoryWorkBudget && op.Status.State == "running" {
			historyStopped(&op, "partial", "time_limit")
		}
	}
	if records != nil {
		if err = s.expireHistoryTx(ctx, tx, time.Now()); err != nil {
			return HistoryOperation{}, err
		}
	}
	if err = saveHistoryOperation(ctx, tx, &op); err != nil {
		return HistoryOperation{}, err
	}
	return commitHistoryResult(tx, op)
}

func (s *Store) commitHistoryRecords(ctx context.Context, tx *sql.Tx, op *HistoryOperation, page domain.HistoryPage, records []ExpiringHistoryRecord) error {
	status := &op.Status
	since, _ := time.Parse(time.RFC3339Nano, status.Since)
	until, _ := time.Parse(time.RFC3339Nano, status.Until)
	eligible := []domain.Message{}
	eligibleExpiring := []ExpiringHistoryRecord{}
	for i, m := range page.Messages {
		if !m.SentAt.Before(since) && m.SentAt.Before(until) {
			eligible = append(eligible, m)
			if records != nil {
				eligibleExpiring = append(eligibleExpiring, records[i])
			}
		} else {
			status.OutOfIntervalCount++
		}
	}
	var snapshot int64
	if err := tx.QueryRowContext(ctx, "SELECT coalesce(max(seq),0) FROM messages").Scan(&snapshot); err != nil {
		return err
	}
	var counts HistoryPageCounts
	var err error
	if records == nil {
		counts, err = s.putHistoryPageTx(ctx, tx, eligible)
	} else {
		counts, err = s.putExpiringHistoryPageTx(ctx, tx, eligibleExpiring)
	}
	if err != nil {
		return err
	}
	status.InsertedCount += counts.Inserted
	status.DuplicateCount += counts.Duplicates
	status.RecordsObserved += len(page.Messages)
	status.PagesObserved++
	if counts.Inserted > 0 {
		var earliest, latest string
		if err = tx.QueryRowContext(ctx, "SELECT min(sent_at),max(sent_at) FROM messages WHERE seq>?", snapshot).Scan(&earliest, &latest); err != nil {
			return err
		}
		if status.EarliestImportedAt == nil || earlierHistoryTime(earliest, *status.EarliestImportedAt) {
			status.EarliestImportedAt = &earliest
		}
		if status.LatestImportedAt == nil || earlierHistoryTime(*status.LatestImportedAt, latest) {
			status.LatestImportedAt = &latest
		}
	}
	status.SourceHasMore, status.IsFiltered = page.HasMore, page.IsFiltered
	status.IsFilteredByPhase, status.IsFilteredByTimeJoin = page.IsFilteredByPhase, page.IsFilteredByTimeJoin
	status.IsOld, status.JoinTimestampMillis = page.IsOld, page.JoinTimestampMillis
	var seen int
	continuation := ""
	if page.Cursor != nil {
		continuation = *page.Cursor
		if page.PhaseRequired && page.IsOld != nil && *page.IsOld {
			continuation = "old:" + continuation
		}
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM history_cursors WHERE operation_id=? AND cursor=?", status.OperationID, continuation).Scan(&seen); err != nil {
			return err
		}
	}
	positive := func(v *bool) bool { return v != nil && *v }
	switch {
	case page.LimitedSnapshot:
		historyStopped(op, "partial", "source_window_limited")
	case positive(page.IsFiltered) || positive(page.IsFilteredByPhase) || positive(page.IsFilteredByTimeJoin):
		historyStopped(op, "partial", "source_filtered")
	case page.HasMore == nil:
		historyStopped(op, "partial", "missing_continuation")
	case !*page.HasMore:
		historyStopped(op, "completed", "available_source_exhausted")
	case len(page.Messages) == 0:
		historyStopped(op, "partial", "empty_continuing_page")
	case page.Cursor == nil:
		historyStopped(op, "partial", "missing_continuation")
	case page.PhaseRequired && page.IsOld == nil:
		historyStopped(op, "partial", "missing_continuation")
	case seen > 0:
		historyStopped(op, "partial", "repeated_cursor")
	default:
		op.Cursor = *page.Cursor
		if _, err = tx.ExecContext(ctx, "INSERT INTO history_cursors VALUES(?,?)", status.OperationID, continuation); err != nil {
			return err
		}
		switch {
		case status.PagesObserved >= status.MaxPages:
			historyStopped(op, "partial", "page_limit")
		case status.RecordsObserved >= status.MaxMessages:
			historyStopped(op, "partial", "message_limit")
		case op.WorkDuration >= HistoryWorkBudget:
			historyStopped(op, "partial", "time_limit")
		}
	}
	return nil
}

func earlierHistoryTime(a, b string) bool {
	x, _ := time.Parse(time.RFC3339Nano, a)
	y, _ := time.Parse(time.RFC3339Nano, b)
	return x.Before(y)
}

// RecoverInterruptedHistory runs only under the service account lock. Opening a
// Store for diagnostics does not reset a live worker's revision or state.
func (s *Store) RecoverInterruptedHistory(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	account, err := historyAccount(ctx, tx)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT operation_id FROM history_operations WHERE account_key=? AND state IN ('queued','running','paused')", account)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		op, err := loadHistoryOperation(ctx, tx, id)
		if err != nil {
			return err
		}
		if !s.AllowsConversation(op.Status.Ref()) {
			historyStopped(&op, "cancelled", "access_revoked")
		} else if op.Status.State == "running" {
			// A crash cannot provide an elapsed duration. Charge the durable
			// reservation conservatively so repeated crashes cannot reset limits.
			op.WorkDuration += op.ReservedDuration
			op.ReservedDuration = 0
			if op.WorkDuration >= historyWorkBudget(op) {
				historyStopped(&op, "partial", "time_limit")
			} else {
				op.Status.State, op.Status.StopReason = "queued", nil
			}
		} else {
			continue
		}
		if err = saveHistoryOperation(ctx, tx, &op); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PendingHistoryOperations(ctx context.Context, limit int) ([]HistoryOperation, error) {
	return s.pendingHistoryOperations(ctx, limit, false)
}

func (s *Store) PendingMobileHistoryOperations(ctx context.Context, limit int) ([]HistoryOperation, error) {
	return s.pendingHistoryOperations(ctx, limit, true)
}

func (s *Store) pendingHistoryOperations(ctx context.Context, limit int, mobile bool) ([]HistoryOperation, error) {
	if limit < 1 || limit > 100 {
		return nil, domain.Invalid("History queue limit must be 1–100.")
	}
	account, err := historyAccount(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	comparison := "<>"
	if mobile {
		comparison = "="
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT payload,cursor,revision,work_ns FROM history_operations WHERE account_key=? AND state IN ('queued','paused') AND coalesce(json_extract(payload,'$.source'),'') "+comparison+" ? ORDER BY rowid LIMIT ?", account, domain.HistorySourceMobileArchive, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []HistoryOperation{}
	for rows.Next() {
		op := HistoryOperation{accountKey: account}
		var body string
		if err = rows.Scan(&body, &op.Cursor, &op.Revision, &op.WorkDuration); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &op.Status); err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, rows.Err()
}

func (s *Store) StopHistoryOperation(ctx context.Context, id string, revision int64, state, reason string, elapsed time.Duration) (HistoryOperation, error) {
	valid := (state == "paused" && reason == "auth_required") ||
		(state == "unsupported" && reason == "source_unsupported") ||
		(state == "failed" && (reason == "upstream_unavailable" || reason == "invalid_source_page" || reason == "storage_error")) ||
		(state == "partial" && reason == "time_limit")
	mobileStop := state == "partial" && (reason == "source_unavailable" || reason == "source_gaps")
	if (!valid && !mobileStop) || elapsed < 0 || elapsed > MobileHistoryWorkBudget {
		return HistoryOperation{}, domain.Invalid("Invalid history stop transition.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryOperation{}, err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return op, err
	}
	if op.Revision != revision || op.Status.State != "running" {
		return op, ErrHistoryState
	}
	if elapsed > historyWorkBudget(op) || mobileStop && op.Status.Source != domain.HistorySourceMobileArchive {
		return op, domain.Invalid("Invalid history stop transition.")
	}
	historyStopped(&op, state, reason)
	if state == "unsupported" {
		op.Status.SourceKind = "unsupported"
	}
	op.WorkDuration += elapsed
	op.ReservedDuration = 0
	if err = saveHistoryOperation(ctx, tx, &op); err != nil {
		return op, err
	}
	return commitHistoryResult(tx, op)
}

// ReserveHistoryPage records the maximum in-flight source duration before the
// network call. Normal completion reconciles actual work; crash recovery charges
// the reservation, without charging time spent waiting for authentication.
func (s *Store) ReserveHistoryPage(ctx context.Context, id string, revision int64, budget time.Duration) (HistoryOperation, error) {
	return s.reserveHistoryWork(ctx, id, revision, budget, false)
}

func (s *Store) ReserveMobileHistoryWork(ctx context.Context, id string, revision int64, budget time.Duration) (HistoryOperation, error) {
	return s.reserveHistoryWork(ctx, id, revision, budget, true)
}

func (s *Store) reserveHistoryWork(ctx context.Context, id string, revision int64, budget time.Duration, mobile bool) (HistoryOperation, error) {
	limit := 30 * time.Second
	if mobile {
		limit = 180 * time.Second
	}
	if budget <= 0 || budget > limit {
		return HistoryOperation{}, domain.Invalid("Invalid history source work reservation.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return HistoryOperation{}, err
	}
	defer tx.Rollback()
	op, err := loadHistoryOperation(ctx, tx, id)
	if err != nil {
		return HistoryOperation{}, err
	}
	if op.Revision != revision || op.Status.State != "running" || op.ReservedDuration != 0 || (op.Status.Source == domain.HistorySourceMobileArchive) != mobile {
		return HistoryOperation{}, ErrHistoryState
	}
	if !s.AllowsConversation(op.Status.Ref()) {
		historyStopped(&op, "cancelled", "access_revoked")
	} else {
		if budget > historyWorkBudget(op)-op.WorkDuration {
			return HistoryOperation{}, domain.Invalid("History source reservation exceeds work budget.")
		}
		op.ReservedDuration = budget
	}
	if err = saveHistoryOperation(ctx, tx, &op); err != nil {
		return HistoryOperation{}, err
	}
	return commitHistoryResult(tx, op)
}
