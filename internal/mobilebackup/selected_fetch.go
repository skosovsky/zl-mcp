package mobilebackup

import (
	"bytes"
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type SelectedArchive struct {
	requestID                                      string
	ref                                            domain.ConversationRef
	requestFingerprint                             string
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
	encrypted, e := d.Fetch(ctx, offer.URL, offer.FileSize, uint64(normalized.MaxArchiveBytes))
	if e != nil {
		return SelectedArchive{}, ErrArchive
	}
	defer clear(encrypted)
	archive, e := ReadFormat1Archive(ctx, bytes.NewReader(encrypted), offer.KeyText, uint64(normalized.MaxArchiveBytes), MaxTotalBytes)
	if e != nil {
		return SelectedArchive{}, ErrArchive
	}
	defer archive.Clear()
	request, e := ArchiveIdentityRequest(ctx, archive.Archive)
	if e != nil {
		return SelectedArchive{}, ErrArchive
	}
	pairs, e := mapper.MapMobileBackupIdentities(ctx, request)
	defer clear(pairs)
	if e != nil || ctx.Err() != nil {
		return SelectedArchive{}, ErrArchive
	}
	index, e := SelectArchiveIndex(ctx, archive.Archive, pairs, normalized.Ref())
	if e != nil {
		return SelectedArchive{}, ErrArchive
	}
	if ctx.Err() != nil {
		return SelectedArchive{}, ErrArchive
	}
	selected := SelectedArchive{ref: normalized.Ref(), requestID: normalized.RequestID, requestFingerprint: normalized.Fingerprint(), File: archive.Archive.Files[index], CiphertextBytes: archive.CiphertextBytes, ContainerBytes: archive.ContainerBytes, TrailingBytes: archive.TrailingBytes}
	archive.Archive.Files[index] = ArchiveFile{}
	return selected, nil
}
