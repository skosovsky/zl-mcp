package service

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
)

// runPreparedMobileOffer is an internal port, not a tool or CLI dispatch method.
// Production current.API is the existing collector sessionGuard.
func (p *membershipPort) runPreparedMobileOffer(parent context.Context, id string) (domain.MobileBackupOffer, error) {
	if parent == nil {
		return domain.MobileBackupOffer{}, mobilebackup.ErrOfferExecution
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if p.lifecycle != nil {
		stop := context.AfterFunc(p.lifecycle, cancel)
		defer stop()
		if p.lifecycle.Err() != nil {
			cancel()
		}
	}
	if ctx.Err() != nil {
		return domain.MobileBackupOffer{}, mobilebackup.ErrOfferExecution
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if ctx.Err() != nil || p.current == nil {
		return domain.MobileBackupOffer{}, mobilebackup.ErrOfferExecution
	}
	source, ok := p.current.API.(domain.MobileBackupSource)
	if !ok {
		return domain.MobileBackupOffer{}, mobilebackup.ErrOfferExecution
	}
	return mobilebackup.RunPreparedOffer(ctx, p.store, source, id)
}

// fetchPreparedMobileArchive keeps session ownership across the entire staged fetch.
// There is deliberately no public/trusted CLI route until live acceptance.
func (p *membershipPort) fetchPreparedMobileArchive(parent context.Context, id string, downloader *mobilebackup.Downloader) (mobilebackup.SelectedArchive, error) {
	return p.consumePreparedMobileArchive(parent, id, downloader, nil)
}

func (p *membershipPort) consumePreparedMobileArchive(parent context.Context, id string, downloader *mobilebackup.Downloader, consume func(context.Context, mobilebackup.SelectedArchive, domain.MobileBackupRequest, domain.MobileIdentitySource, string) error) (mobilebackup.SelectedArchive, error) {
	if parent == nil || !downloader.Ready() {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if p.lifecycle != nil {
		stop := context.AfterFunc(p.lifecycle, cancel)
		defer stop()
		if p.lifecycle.Err() != nil {
			cancel()
		}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if ctx.Err() != nil || p.current == nil {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	source, offers := p.current.API.(domain.MobileBackupSource)
	mapper, mappings := p.current.API.(domain.MobileIdentitySource)
	owner, scoped := p.current.API.(domain.MobileBackupScope)
	archiveTransport, authenticated := p.current.API.(domain.MobileArchiveTransport)
	if !offers || !mappings || !scoped || !authenticated {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	boundDownloader, e := downloader.WithSession(archiveTransport)
	if e != nil {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	operation, stop, e := owner.MobileBackupContext(ctx)
	if e != nil {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	defer stop()
	attempt, e := p.store.MobileBackupAttempt(operation, id)
	if e != nil || attempt.Request.ArchiveScope != "" || attempt.State != "prepared" {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	identityCheck, e := mobilebackup.IdentityPayload(domain.MobileIdentityRequest{Direct: []string{attempt.Request.ConversationID}})
	if e != nil {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	clear(identityCheck)
	account := p.current.API.AccountID()
	if account == "" {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	offer, e := mobilebackup.RunPreparedOffer(operation, p.store, source, id)
	if e != nil {
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	selected, e := mobilebackup.FetchSelectedArchive(operation, boundDownloader, offer, attempt.Request, mapper)
	if e != nil || operation.Err() != nil || p.current.API.AccountID() != account {
		slog.Warn("mobile_archive_scope_failed", "stage", "FETCH_COMPLETE", "fetch_failed", e != nil, "cancelled", operation.Err() != nil, "owner_changed", p.current.API.AccountID() != account)
		selected.Clear()
		return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
	}
	if consume != nil {
		defer selected.Clear()
		if consume(operation, selected, attempt.Request, mapper, account) != nil || operation.Err() != nil || p.current.API.AccountID() != account {
			return mobilebackup.SelectedArchive{}, mobilebackup.ErrArchive
		}
		return mobilebackup.SelectedArchive{}, nil
	}
	return selected, nil
}

func (p *membershipPort) prepareFirstMobilePage(parent context.Context, id string, downloader *mobilebackup.Downloader, scratch string, nowMS int64, size int) (result mobilebackup.PreparedArchivePage, err error) {
	if !filepath.IsAbs(scratch) || nowMS <= 0 || size < 1 || size > 50 {
		return result, mobilebackup.ErrArchive
	}
	_, err = p.consumePreparedMobileArchive(parent, id, downloader, func(ctx context.Context, selected mobilebackup.SelectedArchive, request domain.MobileBackupRequest, mapper domain.MobileIdentitySource, account string) error {
		var e error
		result, e = mobilebackup.ReadPreparedArchivePage(ctx, selected, request, account, scratch, nowMS, size, nil, mapper)
		return e
	})
	if err != nil {
		result.Clear()
	}
	return result, err
}

// walkPreparedMobilePages retains the same archive/session for all bounded pages.
// There is no public route; a future importer must atomically persist page/checkpoint.
func (p *membershipPort) walkPreparedMobilePages(parent context.Context, id string, downloader *mobilebackup.Downloader, scratch string, nowMS int64, size, maxPages int, visit func(context.Context, mobilebackup.PreparedArchivePage) error) (result mobilebackup.ArchiveWalkSummary, err error) {
	if !filepath.IsAbs(scratch) || nowMS <= 0 || size < 1 || size > 50 || maxPages < 1 || maxPages > 100 || visit == nil {
		return result, mobilebackup.ErrArchive
	}
	_, err = p.consumePreparedMobileArchive(parent, id, downloader, func(ctx context.Context, selected mobilebackup.SelectedArchive, request domain.MobileBackupRequest, mapper domain.MobileIdentitySource, account string) error {
		var e error
		result, e = mobilebackup.WalkPreparedArchive(ctx, selected, request, account, scratch, nowMS, size, maxPages, mapper, visit)
		return e
	})
	if err != nil {
		result = mobilebackup.ArchiveWalkSummary{}
	}
	return result, err
}
