package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func validateMobileRecallTargets(op HistoryOperation, source domain.MobileHistorySnapshot, recalls []domain.MobileHistoryRecall, requirePeriod bool) error {
	since, _ := time.Parse(time.RFC3339Nano, op.Status.Since)
	until, _ := time.Parse(time.RFC3339Nano, op.Status.Until)
	seen := map[string]bool{}
	for _, recall := range recalls {
		id, e := strconv.ParseUint(recall.MessageID, 10, 64)
		at := time.UnixMilli(recall.RecordAtMS)
		hash := sha256.Sum256([]byte(recall.SenderID))
		if e != nil || id == 0 || strconv.FormatUint(id, 10) != recall.MessageID || seen[recall.MessageID] || !source.HasRange || recall.Conversation != op.Status.Ref() || recall.Conversation.Type != domain.ConversationDirect || recall.SenderID == "" || hex.EncodeToString(hash[:]) != op.accountKey || recall.RecordAtMS < source.EarliestMS || recall.RecordAtMS > source.LatestMS || requirePeriod && (at.Before(since) || !at.Before(until)) {
			return domain.Invalid("Invalid mobile history recall target.")
		}
		seen[recall.MessageID] = true
	}
	return nil
}

func saveMobileSourceCheckpoint(ctx context.Context, tx *sql.Tx, id string, source domain.MobileHistorySnapshot, next *domain.MobileHistoryPosition) error {
	var ts, rowid any
	if next != nil {
		ts, rowid = next.TimestampMS, next.RowID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO history_mobile_sources(operation_id,snapshot_id,digest,created_ms,expires_ms,source_rows,period_rows,invalid_timestamp_rows,has_range,earliest_ms,latest_ms,next_timestamp_ms,next_rowid) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(operation_id) DO UPDATE SET next_timestamp_ms=excluded.next_timestamp_ms,next_rowid=excluded.next_rowid`, id, source.ID, source.Digest[:], source.CreatedMS, source.ExpiresMS, source.SourceRows, source.PeriodRows, source.InvalidTimestampRows, source.HasRange, source.EarliestMS, source.LatestMS, ts, rowid)
	return err
}

func (s *Store) commitMobileControlPreludeTx(ctx context.Context, tx *sql.Tx, op HistoryOperation, page domain.MobileHistoryPage, elapsed time.Duration, started time.Time) (HistoryOperation, error) {
	source := page.Snapshot
	if !source.ControlPreludeComplete || source.ControlRows < 1 || source.ControlRows > 5000 || len(page.Recalls) != source.ControlRows || len(page.Records) != 0 || page.Counts != (domain.MobileHistoryCounts{}) || !page.HasMore || page.Next != nil || op.Status.PagesObserved != 0 || op.Status.RecordsObserved != 0 || op.Status.MobileCoverage != nil {
		return HistoryOperation{}, ErrMobileHistorySource
	}
	if _, err := loadMobileHistoryCheckpoint(ctx, tx, op.Status.OperationID); !errors.Is(err, sql.ErrNoRows) {
		if err != nil {
			return HistoryOperation{}, err
		}
		return HistoryOperation{}, ErrMobileHistorySource
	}
	if err := validateMobileRecallTargets(op, source, page.Recalls, false); err != nil {
		return HistoryOperation{}, err
	}
	if op.WorkDuration < 0 || op.WorkDuration > MobileHistoryWorkBudget || elapsed > MobileHistoryWorkBudget-op.WorkDuration {
		return HistoryOperation{}, domain.Invalid("Mobile history prelude exceeds work budget.")
	}
	for _, recall := range page.Recalls {
		if err := s.deleteObservedMessageTx(ctx, tx, recall.Conversation, recall.MessageID); err != nil {
			return HistoryOperation{}, err
		}
	}
	if err := saveMobileSourceCheckpoint(ctx, tx, op.Status.OperationID, source, nil); err != nil {
		return HistoryOperation{}, err
	}
	op.Status.MobileCoverage = &domain.MobileHistoryCoverage{SourceRows: source.SourceRows, PeriodRows: source.PeriodRows, InvalidTimestampRows: source.InvalidTimestampRows, SourceControls: source.ControlRows, SourceRecalls: len(page.Recalls), ControlPreludeComplete: true}
	op.WorkDuration += elapsed + time.Since(started)
	op.ReservedDuration = 0
	if time.Now().UnixMilli() >= source.ExpiresMS {
		return HistoryOperation{}, ErrMobileHistorySource
	}
	if op.WorkDuration > MobileHistoryWorkBudget {
		return HistoryOperation{}, domain.Invalid("Mobile history prelude exceeds work budget.")
	}
	if op.WorkDuration == MobileHistoryWorkBudget {
		historyStopped(&op, "partial", "time_limit")
	}
	if err := saveHistoryOperation(ctx, tx, &op); err != nil {
		return HistoryOperation{}, err
	}
	return commitHistoryResult(tx, op)
}
