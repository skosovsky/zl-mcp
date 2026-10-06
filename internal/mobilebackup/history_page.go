package mobilebackup

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

var ErrSnapshotSourceUnsupported = errors.New("selected mobile snapshot source is unsupported")

// ReadHistoryPage resumes a trusted journal checkpoint from the same encrypted
// snapshot. It never downloads, requests the phone, writes the corpus or emits Events.
// The caller supplies only durable progress from the operation/source journal.
func (s *SnapshotStore) ReadHistoryPage(ctx context.Context, request domain.MobileBackupRequest, account, scratch string, size, examined int, previous *domain.MobileHistorySnapshot, after *domain.MobileHistoryPosition, mapper domain.MobileIdentitySource) (result domain.MobileHistoryPage, err error) {
	r, err := request.Normalize()
	if err != nil || examined < 0 || examined >= r.MaxMessages || (examined == 0) != (previous == nil) || (examined == 0) != (after == nil) {
		return result, ErrSnapshot
	}
	selected, err := s.Read(ctx, r, account)
	if err != nil {
		return result, err
	}
	defer selected.Clear()
	nowMS := time.Now().UnixMilli()
	if selected.snapshotCreatedMS <= 0 || selected.snapshotExpiresMS <= nowMS {
		return result, ErrSnapshot
	}
	digest := sha256.Sum256(selected.File.Data)
	if previous != nil && (previous.ID != r.RequestID || previous.Digest != digest || previous.CreatedMS != selected.snapshotCreatedMS || previous.ExpiresMS != selected.snapshotExpiresMS) {
		return result, ErrSnapshotConflict
	}
	var cursor *PreparedArchiveCursor
	if after != nil {
		since, _ := time.Parse(time.RFC3339Nano, r.Since)
		until, _ := time.Parse(time.RFC3339Nano, r.Until)
		from, to, valid := millisecondWindow(since, until)
		if !valid {
			return result, ErrSnapshot
		}
		cursor = &PreparedArchiveCursor{requestID: r.RequestID, fingerprint: r.Fingerprint(), account: account, examined: examined, sqlite: SQLiteCursor{Digest: digest, Name: selected.File.Name, SinceMS: from, UntilMS: to, TimestampMS: after.TimestampMS, RowID: after.RowID}}
	}
	page, err := ReadPreparedArchivePage(ctx, selected, r, account, scratch, nowMS, size, cursor, mapper)
	if err != nil {
		return result, err
	}
	defer page.Clear()
	source := domain.MobileHistorySnapshot{ID: r.RequestID, Digest: digest, CreatedMS: selected.snapshotCreatedMS, ExpiresMS: selected.snapshotExpiresMS, SourceRows: page.Coverage.SourceRows, PeriodRows: page.Coverage.PeriodRows, InvalidTimestampRows: page.Coverage.InvalidTimestamps, HasRange: page.Coverage.HasRange, EarliestMS: page.Coverage.EarliestMS, LatestMS: page.Coverage.LatestMS, WALMode: page.SourceWAL, ControlRows: page.SourceControls}
	if previous != nil && *previous != source {
		return result, ErrSnapshotConflict
	}
	if page.SourceWAL || page.SourceControls > 0 {
		return result, ErrSnapshotSourceUnsupported
	}
	converted, err := ConvertPreparedArchivePage(ctx, page, r, account, nowMS)
	if err != nil {
		return result, err
	}
	defer converted.Clear()
	if ctx.Err() != nil || time.Now().UnixMilli() >= source.ExpiresMS {
		return result, ErrSnapshot
	}
	result.Snapshot, result.HasMore = source, page.SourceHasMore
	result.Counts = domain.MobileHistoryCounts{Examined: page.Examined, Rejected: page.Rejected, Expired: page.ExpiredMessages + converted.Expired, UnsupportedTypes: page.Candidates.UnsupportedTypes, UnsupportedContent: converted.UnsupportedContent, MissingMetadata: page.Candidates.MissingMetadata, InvalidMetadata: page.Candidates.InvalidMetadata, DeferredControls: page.Candidates.DeferredControls, UnknownMetadataFields: page.Candidates.UnsupportedMetadataFields, ExpiredQuotes: page.ExpiredQuotes, UnresolvedQuotes: converted.UnresolvedQuotes, UnresolvedMentions: converted.UnresolvedMentions}
	if page.Next != nil {
		result.Next = &domain.MobileHistoryPosition{TimestampMS: page.Next.sqlite.TimestampMS, RowID: page.Next.sqlite.RowID}
	}
	result.Records, converted.Records = converted.Records, nil
	return result, nil
}
