package mobilebackup

import (
	"bytes"
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
	slog.Info("mobile_archive_stage", "request_id", normalized.RequestID, "stage", "download")
	encrypted, e := d.Fetch(ctx, offer.URL, offer.FileSize, uint64(normalized.MaxArchiveBytes))
	if e != nil {
		return SelectedArchive{}, ErrArchive
	}
	defer clear(encrypted)
	slog.Info("mobile_archive_stage", "request_id", normalized.RequestID, "stage", "decrypt_container")
	archive, e := ReadFormat1Archive(ctx, bytes.NewReader(encrypted), offer.KeyText, uint64(normalized.MaxArchiveBytes), MaxTotalBytes)
	if e != nil {
		return SelectedArchive{}, ErrArchive
	}
	defer archive.Clear()
	slog.Info("mobile_archive_stage", "request_id", normalized.RequestID, "stage", "file_index")
	request, e := ArchiveIdentityRequest(ctx, archive.Archive)
	if e != nil {
		return SelectedArchive{}, ErrArchive
	}
	slog.Info("mobile_archive_stage", "request_id", normalized.RequestID, "stage", "identity_mapping")
	pairs, e := mapper.MapMobileBackupIdentities(ctx, request)
	defer clear(pairs)
	if e != nil || ctx.Err() != nil {
		return SelectedArchive{}, ErrArchive
	}
	slog.Info("mobile_archive_stage", "request_id", normalized.RequestID, "stage", "selection")
	index, e := SelectArchiveIndex(ctx, archive.Archive, pairs, normalized.Ref())
	if e != nil {
		reason := "INVALID_MAPPING"
		if errors.Is(e, ErrSelectedConversationUnavailable) {
			reason = "SELECTED_CONVERSATION_UNAVAILABLE"
		}
		if ctx.Err() != nil {
			reason = "CANCELLED"
		}
		slog.Warn("mobile_archive_selection_failed", "request_id", normalized.RequestID, "reason", reason, "direct_files", len(request.Direct), "group_files", len(request.Groups))
		return SelectedArchive{}, ErrArchive
	}
	if ctx.Err() != nil {
		return SelectedArchive{}, ErrArchive
	}
	selected := SelectedArchive{ref: normalized.Ref(), requestID: normalized.RequestID, requestFingerprint: normalized.Fingerprint(), File: archive.Archive.Files[index], CiphertextBytes: archive.CiphertextBytes, ContainerBytes: archive.ContainerBytes, TrailingBytes: archive.TrailingBytes}
	archive.Archive.Files[index] = ArchiveFile{}
	return selected, nil
}
