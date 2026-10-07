package mobilebackup

import (
	"context"
	"maps"
	"time"
)

// AccountFileCoverage is a bounded owner-only diagnostic, not a message page.
// File ordinals are stable within one authenticated source; no peer IDs or text
// are serialized. A scan limit never becomes a complete-history assertion.
type AccountFileCoverage struct {
	FileIndex         int                         `json:"file_index"`
	ConversationType  string                      `json:"conversation_type"`
	Status            string                      `json:"status"`
	SourceRows        int64                       `json:"source_rows"`
	PeriodRows        int64                       `json:"period_rows"`
	InvalidTimestamps int64                       `json:"invalid_timestamp_rows"`
	EarliestAt        *string                     `json:"source_earliest_at"`
	LatestAt          *string                     `json:"source_latest_at"`
	WALMode           bool                        `json:"wal_mode"`
	Examined          int                         `json:"examined"`
	RejectedReasons   map[string]int              `json:"rejected_row_reasons,omitempty"`
	Rejected          int                         `json:"rejected"`
	SourceControls    int                         `json:"source_controls"`
	SampleHasMore     bool                        `json:"sample_has_more"`
	Types             map[string]int              `json:"sample_content_kinds"`
	Metadata          *AccountMetadataDiagnostics `json:"metadata_diagnostics,omitempty"`
}

// InspectCoverage reads at most 25 files and 50 rows per file. All source bytes
// and mappings were authenticated before selection. SQLite schema rejection is
// reported per file, without exposing source paths or raw values.
func (a AccountArchive) InspectCoverage(ctx context.Context, scratch string, since, until time.Time, offset, limit int) ([]AccountFileCoverage, bool, error) {
	return a.InspectCoverageWithMetadata(ctx, scratch, since, until, offset, limit, false)
}

func (a AccountArchive) InspectCoverageWithMetadata(ctx context.Context, scratch string, since, until time.Time, offset, limit int, includeMetadata bool) ([]AccountFileCoverage, bool, error) {
	if ctx == nil || ctx.Err() != nil || offset < 0 || offset > len(a.archive.Files) || limit < 1 || limit > 25 {
		return nil, false, ErrArchive
	}
	if _, _, valid := millisecondWindow(since, until); !valid {
		return nil, false, ErrSQLite
	}
	end := min(offset+limit, len(a.archive.Files))
	result := make([]AccountFileCoverage, 0, end-offset)
	for index := offset; index < end; index++ {
		file := a.archive.Files[index]
		refType := "direct"
		for _, pair := range a.pairs {
			if pair.Group && file.Name == "group_"+pair.Plain+".db" {
				refType = "group"
			}
		}
		item := AccountFileCoverage{FileIndex: index, ConversationType: refType, Status: "available", Types: map[string]int{}}
		batch, err := ReadSQLiteRows(ctx, file, scratch, since, until, 50)
		if ctx.Err() != nil {
			batch.Clear()
			return nil, false, ErrArchive
		}
		if err != nil {
			item.Status = "unreadable_sqlite"
		} else {
			item.SourceRows, item.PeriodRows, item.InvalidTimestamps = batch.Coverage.SourceRows, batch.Coverage.PeriodRows, batch.Coverage.InvalidTimestamps
			item.RejectedReasons = maps.Clone(batch.RejectedReasons)
			item.WALMode, item.Examined, item.Rejected, item.SourceControls, item.SampleHasMore = batch.WALMode, batch.Examined, batch.Rejected, batch.SourceControls, batch.HasMore
			if batch.Coverage.HasRange {
				first, last := time.UnixMilli(batch.Coverage.EarliestMS).UTC().Format(time.RFC3339Nano), time.UnixMilli(batch.Coverage.LatestMS).UTC().Format(time.RFC3339Nano)
				item.EarliestAt, item.LatestAt = &first, &last
			}
			if includeMetadata {
				var metadataErr error
				item.Metadata, metadataErr = inspectAccountMetadata(ctx, batch.Rows)
				if metadataErr != nil {
					batch.Clear()
					return nil, false, metadataErr
				}
			}
			for _, row := range batch.Rows {
				kind, known := mobilePayloadKind(row.Type)
				if !known {
					kind = "unsupported"
				}
				item.Types[kind]++
			}
		}
		batch.Clear()
		result = append(result, item)
	}
	return result, end < len(a.archive.Files), nil
}
