package mobilebackup

import (
	"context"
	"path/filepath"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ArchiveWalkSummary contains only counts, never archive identities or message data.
type ArchiveWalkSummary struct {
	Pages, Examined, Rejected, Candidates, ExpiredMessages, ExpiredQuotes                           int
	DeferredControls, UnsupportedTypes, MissingMetadata, InvalidMetadata, UnsupportedMetadataFields int
	SourceHasMore                                                                                   bool
	StopReason                                                                                      string
}

// WalkPreparedArchive borrows archive bytes and lends each page until visit returns.
// It performs no persistence and never downloads or dispatches another offer.
func WalkPreparedArchive(parent context.Context, archive SelectedArchive, request domain.MobileBackupRequest, account, scratch string, nowMS int64, size, maxPages int, mapper domain.MobileIdentitySource, visit func(context.Context, PreparedArchivePage) error) (summary ArchiveWalkSummary, err error) {
	if parent == nil || parent.Err() != nil || !filepath.IsAbs(scratch) || !canonicalIdentity(account) || nowMS <= 0 || size < 1 || size > 50 || maxPages < 1 || maxPages > 100 || mapper == nil || visit == nil {
		return summary, ErrSQLite
	}
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	defer func() {
		if err != nil {
			summary = ArchiveWalkSummary{}
		}
	}()
	var cursor *PreparedArchiveCursor
	for {
		page, e := ReadPreparedArchivePage(ctx, archive, request, account, scratch, nowMS, size, cursor, mapper)
		if e != nil {
			return summary, ErrSQLite
		}
		var next *PreparedArchiveCursor
		if page.Next != nil {
			copied := *page.Next
			next = &copied
		}
		counts := ArchiveWalkSummary{Pages: 1, Examined: page.Examined, Rejected: page.Rejected, Candidates: len(page.Candidates.Rows), ExpiredMessages: page.ExpiredMessages, ExpiredQuotes: page.ExpiredQuotes, DeferredControls: page.Candidates.DeferredControls, UnsupportedTypes: page.Candidates.UnsupportedTypes, MissingMetadata: page.Candidates.MissingMetadata, InvalidMetadata: page.Candidates.InvalidMetadata, UnsupportedMetadataFields: page.Candidates.UnsupportedMetadataFields, SourceHasMore: page.SourceHasMore}
		exhausted := page.BudgetExhausted
		e = visit(ctx, page)
		page.Clear()
		if e != nil || ctx.Err() != nil {
			return summary, ErrSQLite
		}
		summary.Pages += counts.Pages
		summary.Examined += counts.Examined
		summary.Rejected += counts.Rejected
		summary.Candidates += counts.Candidates
		summary.ExpiredMessages += counts.ExpiredMessages
		summary.ExpiredQuotes += counts.ExpiredQuotes
		summary.DeferredControls += counts.DeferredControls
		summary.UnsupportedTypes += counts.UnsupportedTypes
		summary.MissingMetadata += counts.MissingMetadata
		summary.InvalidMetadata += counts.InvalidMetadata
		summary.UnsupportedMetadataFields += counts.UnsupportedMetadataFields
		summary.SourceHasMore = counts.SourceHasMore
		switch {
		case !counts.SourceHasMore:
			summary.StopReason = "available_archive_exhausted"
			return summary, nil
		case exhausted:
			summary.StopReason = "message_limit"
			return summary, nil
		case summary.Pages >= maxPages:
			summary.StopReason = "page_limit"
			return summary, nil
		case next == nil:
			return summary, ErrSQLite
		}
		cursor = next
	}
}
