package service

import (
	"context"
	"errors"
	"os"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
)

func (p *membershipPort) WithHistorySession(parent context.Context, visit func(context.Context, historyimport.MobileHistorySession) error) error {
	downloader, err := mobilebackup.NewDownloader([]string{"trans-bin.zaloapp.com"})
	if err != nil {
		return historyimport.ErrMobileHistorySourceUnavailable
	}
	return p.withHistorySession(parent, downloader, visit)
}

func (p *membershipPort) withHistorySession(parent context.Context, downloader *mobilebackup.Downloader, visit func(context.Context, historyimport.MobileHistorySession) error) error {
	if parent == nil || visit == nil {
		return domain.ErrHistoryUnsupported
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
		return ctx.Err()
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.current == nil {
		return domain.ErrAuthenticationRequired
	}
	if p.snapshots == nil {
		return domain.ErrHistoryUnsupported
	}
	api := p.current.API
	source, offers := api.(domain.MobileBackupSource)
	mapper, mappings := api.(domain.MobileIdentitySource)
	owner, scoped := api.(domain.MobileBackupScope)
	transport, authenticated := api.(domain.MobileArchiveTransport)
	if !offers || !mappings || !scoped || !authenticated {
		return domain.ErrHistoryUnsupported
	}
	bound, err := downloader.WithSession(transport)
	if err != nil {
		return domain.ErrHistoryUnsupported
	}
	operation, stop, err := owner.MobileBackupContext(ctx)
	if err != nil {
		return err
	}
	defer stop()
	account := api.AccountID()
	check, err := mobilebackup.IdentityPayload(domain.MobileIdentityRequest{Direct: []string{account}})
	clear(check)
	if err != nil {
		return domain.ErrAuthenticationRequired
	}
	scratch, err := os.MkdirTemp("", "zl-mobile-history-")
	if err != nil {
		return historyimport.ErrMobileHistorySourceUnavailable
	}
	defer os.RemoveAll(scratch)
	if err = p.snapshots.Cleanup(operation); err != nil {
		return historyimport.ErrMobileHistorySourceUnavailable
	}
	session := &mobileHistorySession{port: p, source: source, mapper: mapper, downloader: bound, account: account, scratch: scratch, valid: func() bool { return operation.Err() == nil && api.AccountID() == account }}
	err = visit(operation, session)
	if parent.Err() != nil {
		return parent.Err()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !session.valid() {
		return domain.ErrAuthenticationRequired
	}
	return err
}

type mobileHistorySession struct {
	port             *membershipPort
	source           domain.MobileBackupSource
	mapper           domain.MobileIdentitySource
	downloader       *mobilebackup.Downloader
	account, scratch string
	valid            func() bool
}

func (*mobileHistorySession) String() string   { return "mobile history session [redacted]" }
func (*mobileHistorySession) GoString() string { return "mobile history session [redacted]" }

func (s *mobileHistorySession) ReceiveOffer(ctx context.Context, id string) (domain.MobileBackupOffer, error) {
	if !s.valid() {
		return domain.MobileBackupOffer{}, domain.ErrAuthenticationRequired
	}
	offer, err := mobilebackup.RunPreparedOffer(ctx, s.port.store, s.source, id)
	if !s.valid() {
		return domain.MobileBackupOffer{}, domain.ErrAuthenticationRequired
	}
	return offer, err
}

func (s *mobileHistorySession) SaveOffer(ctx context.Context, request domain.MobileBackupRequest, offer domain.MobileBackupOffer) error {
	if !s.valid() || offer.SessionAccountID != "" && offer.SessionAccountID != s.account {
		return domain.ErrAuthenticationRequired
	}
	selected, err := mobilebackup.FetchSelectedArchive(ctx, s.downloader, offer, request, s.mapper)
	defer selected.Clear()
	if !s.valid() {
		return domain.ErrAuthenticationRequired
	}
	if err != nil {
		return historyimport.ErrMobileHistorySourceUnavailable
	}
	if err = s.port.snapshots.Save(ctx, selected, request, s.account); err != nil {
		return historyimport.ErrMobileHistorySourceUnavailable
	}
	if !s.valid() {
		return domain.ErrAuthenticationRequired
	}
	return nil
}

func (s *mobileHistorySession) ReadHistoryPage(ctx context.Context, request domain.MobileBackupRequest, size, examined int, previous *domain.MobileHistorySnapshot, after *domain.MobileHistoryPosition) (domain.MobileHistoryPage, error) {
	if !s.valid() {
		return domain.MobileHistoryPage{}, domain.ErrAuthenticationRequired
	}
	page, err := s.port.snapshots.ReadHistoryPage(ctx, request, s.account, s.scratch, size, examined, previous, after, s.mapper)
	if !s.valid() {
		page.Clear()
		return domain.MobileHistoryPage{}, domain.ErrAuthenticationRequired
	}
	if err != nil {
		page.Clear()
	}
	switch {
	case errors.Is(err, mobilebackup.ErrSnapshotSourceUnsupported):
		return domain.MobileHistoryPage{}, domain.ErrHistoryUnsupported
	case errors.Is(err, mobilebackup.ErrSQLite):
		return domain.MobileHistoryPage{}, domain.ErrHistoryInvalidPage
	case err != nil:
		return domain.MobileHistoryPage{}, historyimport.ErrMobileHistorySourceUnavailable
	}
	return page, nil
}
