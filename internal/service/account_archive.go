package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func (p *membershipPort) captureAccountArchive(ctx context.Context, id string, revision int64) (mobilebackup.RetainedArchiveManifest, error) {
	d, err := mobilebackup.NewDownloader([]string{"trans-bin.zaloapp.com"})
	if err != nil {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	return p.captureAccountArchiveWithDownloader(ctx, id, revision, d)
}

func (p *membershipPort) captureAccountArchiveWithDownloader(parent context.Context, id string, revision int64, d *mobilebackup.Downloader) (mobilebackup.RetainedArchiveManifest, error) {
	if parent == nil || p.store == nil || p.archives == nil || !d.Ready() {
		return mobilebackup.RetainedArchiveManifest{}, mobilebackup.ErrRetainedArchive
	}
	ctx, cancel := context.WithTimeout(parent, 420*time.Second)
	defer cancel()
	if p.lifecycle != nil {
		stop := context.AfterFunc(p.lifecycle, cancel)
		defer stop()
		if p.lifecycle.Err() != nil {
			cancel()
		}
	}
	attempt, err := p.store.MobileBackupAttempt(ctx, id)
	if err != nil {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	if attempt.Request.ArchiveScope != "account" {
		return mobilebackup.RetainedArchiveManifest{}, storage.ErrMobileBackupState
	}
	key, err := p.store.ArchiveAccountKey(ctx)
	if err != nil {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	saved, manifest, err := p.archives.ReadBound(ctx, attempt.Request.RequestID, key)
	saved.Clear()
	if err == nil {
		return manifest, nil
	}
	if !errors.Is(err, mobilebackup.ErrRetainedAbsent) {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	if attempt.State != "prepared" || attempt.Revision != revision {
		return mobilebackup.RetainedArchiveManifest{}, storage.ErrMobileBackupState
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.current == nil || ctx.Err() != nil {
		return mobilebackup.RetainedArchiveManifest{}, mobilebackup.ErrOfferExecution
	}
	api := p.current.API
	source, offers := api.(domain.MobileBackupSource)
	mapper, mappings := api.(domain.MobileIdentitySource)
	owner, scoped := api.(domain.MobileBackupScope)
	transport, authenticated := api.(domain.MobileArchiveTransport)
	if !offers || !mappings || !scoped || !authenticated {
		return mobilebackup.RetainedArchiveManifest{}, mobilebackup.ErrOfferExecution
	}
	account := api.AccountID()
	hash := sha256.Sum256([]byte(account))
	if hex.EncodeToString(hash[:]) != key {
		return mobilebackup.RetainedArchiveManifest{}, mobilebackup.ErrRetainedConflict
	}
	operation, stop, err := owner.MobileBackupContext(ctx)
	if err != nil {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	defer stop()
	bound, err := d.WithSession(transport)
	if err != nil {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	offer, err := mobilebackup.RunPreparedOffer(operation, p.store, source, id)
	if err != nil {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	archive, err := mobilebackup.FetchAccountArchive(operation, bound, offer, attempt.Request.MaxArchiveBytes, mapper)
	defer archive.Clear()
	if err != nil || operation.Err() != nil || api.AccountID() != account {
		return mobilebackup.RetainedArchiveManifest{}, mobilebackup.ErrRetainedArchive
	}
	return p.archives.Save(operation, attempt.Request.RequestID, account, archive, time.Duration(attempt.Request.RetentionHours)*time.Hour)
}

func (p *membershipPort) accountArchiveStatus(ctx context.Context, id string) (mobilebackup.RetainedArchiveManifest, error) {
	if p.store == nil || p.archives == nil {
		return mobilebackup.RetainedArchiveManifest{}, mobilebackup.ErrRetainedArchive
	}
	key, err := p.store.ArchiveAccountKey(ctx)
	if err != nil {
		return mobilebackup.RetainedArchiveManifest{}, err
	}
	archive, manifest, err := p.archives.ReadBound(ctx, id, key)
	archive.Clear()
	return manifest, err
}

func (p *membershipPort) inspectAccountArchive(ctx context.Context, id, sinceText, untilText string, offset, limit int) (map[string]any, error) {
	if p.store == nil || p.archives == nil {
		return nil, mobilebackup.ErrRetainedArchive
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	since, err := time.Parse(time.RFC3339Nano, sinceText)
	if err != nil {
		return nil, domain.Invalid("Invalid archive interval.")
	}
	until, err := time.Parse(time.RFC3339Nano, untilText)
	if err != nil || !since.Before(until) {
		return nil, domain.Invalid("Invalid archive interval.")
	}
	key, err := p.store.ArchiveAccountKey(ctx)
	if err != nil {
		return nil, err
	}
	archive, manifest, err := p.archives.ReadBound(ctx, id, key)
	defer archive.Clear()
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 25
	}
	scratch, err := os.MkdirTemp(p.stateDir, "account-inspection-")
	if err != nil {
		return nil, mobilebackup.ErrRetainedArchive
	}
	defer os.RemoveAll(scratch)
	files, more, err := archive.InspectCoverage(ctx, scratch, since, until, offset, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"manifest": manifest, "since": since.UTC().Format(time.RFC3339Nano), "until": until.UTC().Format(time.RFC3339Nano), "offset": offset, "files": files, "has_more_files": more, "download_performed": false, "import_performed": false, "history_complete": false}, nil
}

func (p *membershipPort) removeAccountArchive(ctx context.Context, id string) (map[string]any, error) {
	if p.store == nil || p.archives == nil {
		return nil, mobilebackup.ErrRetainedArchive
	}
	key, err := p.store.ArchiveAccountKey(ctx)
	if err != nil {
		return nil, err
	}
	if err = p.archives.RemoveBound(ctx, id, key); err != nil {
		return nil, err
	}
	return map[string]any{"source_id": id, "removed": true, "import_performed": false}, nil
}
