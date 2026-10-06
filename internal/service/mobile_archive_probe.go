package service

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// probeMobileArchive is an owner-only bounded diagnostic, never an MCP tool.
func (p *membershipPort) probeMobileArchive(ctx context.Context, id string, revision int64) (map[string]any, error) {
	return p.probeMobileArchiveComparison(ctx, id, revision, "")
}
func (p *membershipPort) probeMobileArchiveComparison(ctx context.Context, id string, revision int64, sendID string) (map[string]any, error) {
	d, err := mobilebackup.NewDownloader([]string{"trans-bin.zaloapp.com"})
	if err != nil {
		return nil, err
	}
	return p.probeMobileArchiveComparisonWithDownloader(ctx, id, revision, d, sendID)
}

func (p *membershipPort) probeMobileArchiveWithDownloader(ctx context.Context, id string, revision int64, d *mobilebackup.Downloader) (map[string]any, error) {
	return p.probeMobileArchiveComparisonWithDownloader(ctx, id, revision, d, "")
}
func (p *membershipPort) probeMobileArchiveComparisonWithDownloader(ctx context.Context, id string, revision int64, d *mobilebackup.Downloader, sendID string) (map[string]any, error) {
	attempt, err := p.store.MobileBackupAttempt(ctx, id)
	if err != nil {
		return nil, err
	}
	if attempt.State != "prepared" || attempt.Revision != revision {
		return nil, storage.ErrMobileBackupState
	}
	target := ""
	if sendID != "" {
		op, e := p.store.SendStatus(ctx, sendID)
		if e != nil || op.Status != "sent" || op.MessageID == nil || !canonicalDiagnosticID(*op.MessageID) || op.RecipientID != attempt.Request.ConversationID || attempt.Request.ConversationType != domain.ConversationDirect {
			return nil, domain.Invalid("Comparison needs a sent operation for the selected direct conversation.")
		}
		target = *op.MessageID
	}
	var comparison map[string]any
	request, cancel := context.WithTimeout(ctx, 420*time.Second)
	defer cancel()
	scratch, err := os.MkdirTemp("", "zl-mobile-archive-")
	if err != nil {
		return nil, mobilebackup.ErrArchive
	}
	defer os.RemoveAll(scratch)
	now := time.Now().UnixMilli()
	convertible, unsupported, controls := 0, 0, 0
	textCandidates, unresolvedQuotes, unresolvedMentions := 0, 0, 0
	ownRecallCandidates := 0
	blockReasons := []string{}
	blocked := false
	walMode := false
	var coverage mobilebackup.SQLiteCoverage
	var summary mobilebackup.ArchiveWalkSummary
	_, err = p.consumePreparedMobileArchive(request, id, d, func(ctx context.Context, selected mobilebackup.SelectedArchive, selection domain.MobileBackupRequest, mapper domain.MobileIdentitySource, account string) error {
		var e error
		if target != "" {
			since, _ := time.Parse(time.RFC3339Nano, selection.Since)
			until, _ := time.Parse(time.RFC3339Nano, selection.Until)
			batch, readErr := mobilebackup.ReadSQLiteRows(ctx, selected.File, scratch, since, until, min(selection.MaxMessages, 500))
			if readErr != nil {
				return readErr
			}
			comparison = archiveTargetComparison(batch, target)
			batch.Clear()
		}
		summary, e = mobilebackup.WalkPreparedArchive(ctx, selected, selection, account, scratch, now, 50, 100, mapper, func(ctx context.Context, page mobilebackup.PreparedArchivePage) error {
			walMode = walMode || page.SourceWAL
			coverage = page.Coverage
			if page.SourceControls > controls {
				controls = page.SourceControls
			}
			inspection, e := mobilebackup.InspectPreparedArchivePage(ctx, page, attempt.Request, account, now)
			if e != nil {
				slog.Warn("mobile_archive_probe_failed", "operation_id", id, "stage", "CANDIDATE_VALIDATION")
				return e
			}
			if len(inspection.BlockReasons) > 0 {
				blocked = true
				for _, reason := range inspection.BlockReasons {
					found := false
					for _, prior := range blockReasons {
						found = found || prior == reason
					}
					if !found {
						blockReasons = append(blockReasons, reason)
					}
				}
			} else {
				convertible += inspection.TextCandidates
			}
			textCandidates += inspection.TextCandidates
			ownRecallCandidates += inspection.OwnRecallCandidates
			unsupported += inspection.UnsupportedContent
			unresolvedQuotes += inspection.UnresolvedQuotes
			unresolvedMentions += inspection.UnresolvedMentions
			return nil
		})
		return e
	})
	if err != nil {
		slog.Warn("mobile_archive_probe_failed", "operation_id", id)
		return nil, err
	}
	attempt, err = p.store.MobileBackupAttempt(request, id)
	if err != nil {
		return nil, err
	}
	slog.Info("mobile_archive_probe_complete", "operation_id", id, "examined", summary.Examined, "conversion_blocked", blocked)
	var earliest, latest any
	if coverage.HasRange {
		earliest = time.UnixMilli(coverage.EarliestMS).UTC().Format(time.RFC3339Nano)
		latest = time.UnixMilli(coverage.LatestMS).UTC().Format(time.RFC3339Nano)
	}
	response := map[string]any{"attempt": attempt, "download_performed": true, "import_performed": false, "pages": summary.Pages, "examined": summary.Examined, "rejected": summary.Rejected, "candidates": summary.Candidates, "source_controls": controls, "convertible_messages": convertible, "unsupported_content": unsupported, "expired_messages": summary.ExpiredMessages, "deferred_controls": summary.DeferredControls, "unsupported_types": summary.UnsupportedTypes, "missing_metadata": summary.MissingMetadata, "invalid_metadata": summary.InvalidMetadata, "unsupported_metadata_fields": summary.UnsupportedMetadataFields, "source_has_more": summary.SourceHasMore, "stop_reason": summary.StopReason, "conversion_blocked": blocked, "conversion_block_reasons": blockReasons, "text_candidates": textCandidates, "unresolved_quotes": unresolvedQuotes, "unresolved_mentions": unresolvedMentions, "wal_mode": walMode, "source_rows": coverage.SourceRows, "source_period_rows": coverage.PeriodRows, "invalid_timestamp_rows": coverage.InvalidTimestamps, "source_earliest_at": earliest, "source_latest_at": latest}
	if comparison != nil {
		response["comparison"] = comparison
	}
	response["own_recall_candidates"] = ownRecallCandidates
	return response, nil
}

// archiveTargetComparison reports only bounded observations, never private row data.
func archiveTargetComparison(batch mobilebackup.SQLiteBatch, target string) map[string]any {
	matches, controls := 0, 0
	types := map[string]int{}
	statuses := []int64{}
	for _, row := range batch.Rows {
		if row.MessageID != target {
			continue
		}
		matches++
		types[strconv.FormatInt(row.Type, 10)]++
		statuses = append(statuses, row.Status)
		if row.Type == 33 || row.Type == 36 {
			controls++
		}
	}
	return map[string]any{"examined": batch.Examined, "rejected": batch.Rejected, "has_more": batch.HasMore, "matching_records": matches, "matching_controls": controls, "matching_type_counts": types, "matching_statuses": statuses}
}
