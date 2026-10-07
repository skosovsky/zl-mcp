package mobilebackup

import (
	"context"
	"errors"
	"log/slog"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type SelectedArchive struct {
	requestID                                      string
	ref                                            domain.ConversationRef
	requestFingerprint                             string
	snapshotCreatedMS, snapshotExpiresMS           int64
	File                                           ArchiveFile `json:"-"`
	CiphertextBytes, ContainerBytes, TrailingBytes uint64      `json:"-"`
}

func (SelectedArchive) String() string   { return "mobile backup selected archive [redacted]" }
func (SelectedArchive) GoString() string { return "mobile backup selected archive [redacted]" }
func (a *SelectedArchive) Clear() {
	if a != nil {
		clear(a.File.Data)
		*a = SelectedArchive{}
	}
}
func FetchSelectedArchive(ctx context.Context, d *Downloader, offer domain.MobileBackupOffer, r domain.MobileBackupRequest, mapper domain.MobileIdentitySource) (SelectedArchive, error) {
	if ctx == nil || ctx.Err() != nil || d == nil || mapper == nil {
		return SelectedArchive{}, ErrArchive
	}
	normalized, e := r.Normalize()
	if e != nil || !canonicalIdentity(normalized.ConversationID) {
		return SelectedArchive{}, ErrArchive
	}
	account, e := FetchAccountArchive(ctx, d, offer, normalized.MaxArchiveBytes, mapper)
	if e != nil {
		return SelectedArchive{}, e
	}
	defer account.Clear()
	archive, pairs := account.archive, account.pairs
	slog.Info("mobile_archive_stage", "request_id", normalized.RequestID, "stage", "selection")
	index, e := SelectArchiveIndex(ctx, archive, pairs, normalized.Ref())
	if e != nil {
		reason := "INVALID_MAPPING"
		if errors.Is(e, ErrSelectedConversationUnavailable) {
			reason = "SELECTED_CONVERSATION_UNAVAILABLE"
		}
		if ctx.Err() != nil {
			reason = "CANCELLED"
		}
		slog.Warn("mobile_archive_selection_failed", "request_id", normalized.RequestID, "reason", reason, "direct_files", account.directFiles(), "group_files", account.groupFiles())
		return SelectedArchive{}, ErrArchive
	}
	if ctx.Err() != nil {
		return SelectedArchive{}, ErrArchive
	}
	selected := SelectedArchive{ref: normalized.Ref(), requestID: normalized.RequestID, requestFingerprint: normalized.Fingerprint(), File: archive.Files[index], CiphertextBytes: account.ciphertextBytes, ContainerBytes: account.containerBytes, TrailingBytes: account.trailingBytes}
	account.archive.Files[index] = ArchiveFile{}
	return selected, nil
}
