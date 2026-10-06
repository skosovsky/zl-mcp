package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

var ErrMobileHistorySource = errors.New("mobile history source unavailable or changed")

// MobileHistoryCheckpoint is a private immutable source and exact next position.
type MobileHistoryCheckpoint struct {
	Snapshot domain.MobileHistorySnapshot  `json:"-"`
	Next     *domain.MobileHistoryPosition `json:"-"`
}

func (MobileHistoryCheckpoint) String() string   { return "mobile history checkpoint [redacted]" }
func (MobileHistoryCheckpoint) GoString() string { return "mobile history checkpoint [redacted]" }

func (s *Store) migrateMobileHistory(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS history_mobile_sources(
 operation_id TEXT PRIMARY KEY REFERENCES history_operations(operation_id),
 snapshot_id TEXT NOT NULL,digest BLOB NOT NULL CHECK(length(digest)=32),
 created_ms INTEGER NOT NULL,expires_ms INTEGER NOT NULL,
 source_rows INTEGER NOT NULL,period_rows INTEGER NOT NULL,invalid_timestamp_rows INTEGER NOT NULL,
 has_range INTEGER NOT NULL,earliest_ms INTEGER NOT NULL,latest_ms INTEGER NOT NULL,
 next_timestamp_ms INTEGER,next_rowid INTEGER,
 CHECK((next_timestamp_ms IS NULL)=(next_rowid IS NULL)));
 INSERT OR IGNORE INTO schema_migrations VALUES(13);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func loadMobileHistoryCheckpoint(ctx context.Context, q historyReader, id string) (MobileHistoryCheckpoint, error) {
	var c MobileHistoryCheckpoint
	var digest []byte
	var ts, rowid sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT snapshot_id,digest,created_ms,expires_ms,source_rows,period_rows,invalid_timestamp_rows,has_range,earliest_ms,latest_ms,next_timestamp_ms,next_rowid FROM history_mobile_sources WHERE operation_id=?`, id).Scan(&c.Snapshot.ID, &digest, &c.Snapshot.CreatedMS, &c.Snapshot.ExpiresMS, &c.Snapshot.SourceRows, &c.Snapshot.PeriodRows, &c.Snapshot.InvalidTimestampRows, &c.Snapshot.HasRange, &c.Snapshot.EarliestMS, &c.Snapshot.LatestMS, &ts, &rowid)
	if err != nil {
		return c, err
	}
	if len(digest) != 32 || ts.Valid != rowid.Valid {
		return MobileHistoryCheckpoint{}, ErrMobileHistorySource
	}
	copy(c.Snapshot.Digest[:], digest)
	if ts.Valid {
		c.Next = &domain.MobileHistoryPosition{TimestampMS: ts.Int64, RowID: rowid.Int64}
	}
	return c, nil
}

func (s *Store) MobileHistoryCheckpoint(ctx context.Context, id string) (MobileHistoryCheckpoint, error) {
	op, err := s.HistoryOperation(ctx, id)
	if err != nil {
		return MobileHistoryCheckpoint{}, err
	}
	if op.Status.Source != domain.HistorySourceMobileArchive {
		return MobileHistoryCheckpoint{}, ErrHistoryState
	}
	return loadMobileHistoryCheckpoint(ctx, s.DB, id)
}

func validMobileSource(source domain.MobileHistorySnapshot, op HistoryOperation, nowMS int64) bool {
	if source.ID != op.Status.RequestID || source.Digest == ([32]byte{}) || source.CreatedMS <= 0 || source.CreatedMS > nowMS || source.ExpiresMS-source.CreatedMS != int64(15*time.Minute/time.Millisecond) || source.ExpiresMS <= nowMS || source.WALMode || source.ControlRows != 0 {
		return false
	}
	if source.SourceRows < 0 || source.SourceRows > 256<<20 || source.PeriodRows < 0 || source.PeriodRows > source.SourceRows-source.InvalidTimestampRows || source.InvalidTimestampRows < 0 || source.InvalidTimestampRows > source.SourceRows {
		return false
	}
	if source.HasRange {
		return source.EarliestMS >= 0 && source.LatestMS >= source.EarliestMS && source.LatestMS <= 253402300799999 && source.SourceRows > source.InvalidTimestampRows
	}
	return source.EarliestMS == 0 && source.LatestMS == 0 && source.PeriodRows == 0 && source.SourceRows == source.InvalidTimestampRows
}

func validMobileCounts(c domain.MobileHistoryCounts, records, limit int) bool {
	if c.Examined < 0 || c.Examined > limit || records > c.Examined {
		return false
	}
	total := records
	for _, n := range []int{c.Rejected, c.Expired, c.UnsupportedTypes, c.UnsupportedContent, c.MissingMetadata, c.InvalidMetadata, c.DeferredControls} {
		if n < 0 || n > c.Examined {
			return false
		}
		total += n
	}
	if total != c.Examined {
		return false
	}
	for _, n := range []int{c.ExpiredQuotes, c.UnresolvedQuotes} {
		if n < 0 || n > c.Examined {
			return false
		}
	}
	return c.UnresolvedMentions >= 0 && c.UnresolvedMentions <= c.Examined*1024 && c.UnknownMetadataFields >= 0 && c.UnknownMetadataFields <= c.Examined*32768
}

func addMobileCounts(a, b domain.MobileHistoryCounts) domain.MobileHistoryCounts {
	return domain.MobileHistoryCounts{Examined: a.Examined + b.Examined, Rejected: a.Rejected + b.Rejected, Expired: a.Expired + b.Expired, UnsupportedTypes: a.UnsupportedTypes + b.UnsupportedTypes, UnsupportedContent: a.UnsupportedContent + b.UnsupportedContent, MissingMetadata: a.MissingMetadata + b.MissingMetadata, InvalidMetadata: a.InvalidMetadata + b.InvalidMetadata, DeferredControls: a.DeferredControls + b.DeferredControls, UnknownMetadataFields: a.UnknownMetadataFields + b.UnknownMetadataFields, ExpiredQuotes: a.ExpiredQuotes + b.ExpiredQuotes, UnresolvedQuotes: a.UnresolvedQuotes + b.UnresolvedQuotes, UnresolvedMentions: a.UnresolvedMentions + b.UnresolvedMentions}
}
func mobileHasGaps(c domain.MobileHistoryCoverage) bool {
	g := c.MobileHistoryCounts
	return c.InvalidTimestampRows > 0 || g.Rejected > 0 || g.Expired > 0 || g.UnsupportedTypes > 0 || g.UnsupportedContent > 0 || g.MissingMetadata > 0 || g.InvalidMetadata > 0 || g.DeferredControls > 0 || g.UnknownMetadataFields > 0 || g.ExpiredQuotes > 0 || g.UnresolvedQuotes > 0 || g.UnresolvedMentions > 0
}
func mobilePositionAfter(a, b domain.MobileHistoryPosition) bool {
	return a.TimestampMS > b.TimestampMS || a.TimestampMS == b.TimestampMS && a.RowID > b.RowID
}

// CommitMobileHistoryPage commits one source page and its exact binding/checkpoint.
// No phone, Events or public source selection is performed by this internal port.
func (s *Store) CommitMobileHistoryPage(ctx context.Context, id string, revision int64, page domain.MobileHistoryPage, elapsed time.Duration) (HistoryOperation, error) {
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
	if op.Revision != revision || op.Status.State != "running" || op.Status.Source != domain.HistorySourceMobileArchive {
		return HistoryOperation{}, ErrHistoryState
	}
	if !s.AllowsConversation(op.Status.Ref()) {
		historyStopped(&op, "cancelled", "access_revoked")
		if err = saveHistoryOperation(ctx, tx, &op); err != nil {
			return HistoryOperation{}, err
		}
		return commitHistoryResult(tx, op)
	}
	if elapsed < 0 || elapsed > MobileHistoryWorkBudget || op.Status.PagesObserved >= op.Status.MaxPages || !validMobileSource(page.Snapshot, op, time.Now().UnixMilli()) {
		return HistoryOperation{}, ErrMobileHistorySource
	}
	limit := min(op.Status.PageSize, op.Status.MaxMessages-op.Status.RecordsObserved)
	if !validMobileCounts(page.Counts, len(page.Records), limit) || int64(op.Status.RecordsObserved+page.Counts.Examined) > page.Snapshot.PeriodRows {
		return HistoryOperation{}, domain.Invalid("Invalid mobile history source counts.")
	}
	if err = s.validateExpiringHistoryPage(op.Status.Ref(), page.Records); err != nil {
		return HistoryOperation{}, err
	}
	since, _ := time.Parse(time.RFC3339Nano, op.Status.Since)
	until, _ := time.Parse(time.RFC3339Nano, op.Status.Until)
	for _, r := range page.Records {
		if r.Message.SentAt.Before(since) || !r.Message.SentAt.Before(until) || r.Message.SentAt.UnixMilli() < page.Snapshot.EarliestMS || r.Message.SentAt.UnixMilli() > page.Snapshot.LatestMS {
			return HistoryOperation{}, domain.Invalid("Mobile history record is outside the requested interval.")
		}
	}
	examinedTotal := op.Status.RecordsObserved + page.Counts.Examined
	if page.HasMore && int64(examinedTotal) >= page.Snapshot.PeriodRows || !page.HasMore && int64(examinedTotal) != page.Snapshot.PeriodRows {
		return HistoryOperation{}, domain.Invalid("Mobile source exhaustion contradicts row counts.")
	}
	limitReached := examinedTotal >= op.Status.MaxMessages || op.Status.PagesObserved+1 >= op.Status.MaxPages
	previous, e := loadMobileHistoryCheckpoint(ctx, tx, id)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return HistoryOperation{}, e
	}
	if e == nil && previous.Snapshot != page.Snapshot {
		return HistoryOperation{}, ErrMobileHistorySource
	}
	if e != nil && (op.Status.PagesObserved != 0 || op.Status.MobileCoverage != nil) {
		return HistoryOperation{}, ErrMobileHistorySource
	}
	if page.HasMore {
		if page.Counts.Examined == 0 || page.Next == nil && !limitReached {
			return HistoryOperation{}, domain.Invalid("Invalid mobile history continuation.")
		}
		if page.Next != nil && (time.UnixMilli(page.Next.TimestampMS).Before(since) || !time.UnixMilli(page.Next.TimestampMS).Before(until) || page.Next.TimestampMS < page.Snapshot.EarliestMS || page.Next.TimestampMS > page.Snapshot.LatestMS || previous.Next != nil && !mobilePositionAfter(*page.Next, *previous.Next)) {
			return HistoryOperation{}, domain.Invalid("Invalid mobile history continuation.")
		}
	} else if page.Next != nil {
		return HistoryOperation{}, domain.Invalid("Terminal mobile history page has a continuation.")
	}
	for _, r := range page.Records {
		if previous.Next != nil && r.Message.SentAt.Before(time.UnixMilli(previous.Next.TimestampMS)) || page.Next != nil && r.Message.SentAt.After(time.UnixMilli(page.Next.TimestampMS)) {
			return HistoryOperation{}, domain.Invalid("Mobile history record does not match its source page.")
		}
	}
	if op.WorkDuration < 0 || op.WorkDuration > MobileHistoryWorkBudget || elapsed > MobileHistoryWorkBudget-op.WorkDuration {
		return HistoryOperation{}, domain.Invalid("Mobile history page exceeds work budget.")
	}
	var messageSeq int64
	if err = tx.QueryRowContext(ctx, "SELECT coalesce(max(seq),0) FROM messages").Scan(&messageSeq); err != nil {
		return HistoryOperation{}, err
	}
	counts, err := s.putExpiringHistoryPageTx(ctx, tx, page.Records)
	if err != nil {
		return HistoryOperation{}, err
	}
	op.Status.InsertedCount += counts.Inserted
	op.Status.DuplicateCount += counts.Duplicates
	op.Status.RecordsObserved += page.Counts.Examined
	op.Status.PagesObserved++
	if counts.Inserted > 0 {
		var first, last string
		if err = tx.QueryRowContext(ctx, "SELECT min(sent_at),max(sent_at) FROM messages WHERE seq>?", messageSeq).Scan(&first, &last); err != nil {
			return HistoryOperation{}, err
		}
		if op.Status.EarliestImportedAt == nil || earlierHistoryTime(first, *op.Status.EarliestImportedAt) {
			op.Status.EarliestImportedAt = &first
		}
		if op.Status.LatestImportedAt == nil || earlierHistoryTime(*op.Status.LatestImportedAt, last) {
			op.Status.LatestImportedAt = &last
		}
	}
	coverage := domain.MobileHistoryCoverage{SourceRows: page.Snapshot.SourceRows, PeriodRows: page.Snapshot.PeriodRows, InvalidTimestampRows: page.Snapshot.InvalidTimestampRows}
	if op.Status.MobileCoverage != nil {
		coverage.MobileHistoryCounts = op.Status.MobileCoverage.MobileHistoryCounts
	}
	coverage.MobileHistoryCounts = addMobileCounts(coverage.MobileHistoryCounts, page.Counts)
	op.Status.MobileCoverage = &coverage
	more := page.HasMore
	op.Status.SourceHasMore = &more
	op.WorkDuration += elapsed
	op.ReservedDuration = 0
	switch {
	case !page.HasMore:
		if mobileHasGaps(coverage) {
			historyStopped(&op, "partial", "source_gaps")
		} else {
			historyStopped(&op, "completed", "available_source_exhausted")
		}
	case op.Status.PagesObserved >= op.Status.MaxPages:
		historyStopped(&op, "partial", "page_limit")
	case op.Status.RecordsObserved >= op.Status.MaxMessages:
		historyStopped(&op, "partial", "message_limit")
	case op.WorkDuration >= MobileHistoryWorkBudget:
		historyStopped(&op, "partial", "time_limit")
	}
	var ts, rowid any
	if page.Next != nil {
		ts, rowid = page.Next.TimestampMS, page.Next.RowID
	}
	source := page.Snapshot
	_, err = tx.ExecContext(ctx, `INSERT INTO history_mobile_sources(operation_id,snapshot_id,digest,created_ms,expires_ms,source_rows,period_rows,invalid_timestamp_rows,has_range,earliest_ms,latest_ms,next_timestamp_ms,next_rowid) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(operation_id) DO UPDATE SET next_timestamp_ms=excluded.next_timestamp_ms,next_rowid=excluded.next_rowid`, id, source.ID, source.Digest[:], source.CreatedMS, source.ExpiresMS, source.SourceRows, source.PeriodRows, source.InvalidTimestampRows, source.HasRange, source.EarliestMS, source.LatestMS, ts, rowid)
	if err != nil {
		return HistoryOperation{}, err
	}
	if err = s.expireHistoryTx(ctx, tx, time.Now()); err != nil {
		return HistoryOperation{}, err
	}
	if time.Now().UnixMilli() >= source.ExpiresMS {
		return HistoryOperation{}, ErrMobileHistorySource
	}
	// Include transaction work before publishing either records or progress.
	op.WorkDuration += time.Since(started)
	if op.WorkDuration > MobileHistoryWorkBudget {
		return HistoryOperation{}, domain.Invalid("Mobile history page exceeds work budget.")
	}
	if err = saveHistoryOperation(ctx, tx, &op); err != nil {
		return HistoryOperation{}, err
	}
	return commitHistoryResult(tx, op)
}
