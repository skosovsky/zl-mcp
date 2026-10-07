package mobilebackup

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// AccountArchive owns all decoded files. It is private acquisition evidence,
// not a complete history claim or a serializable tool result.
type AccountArchive struct {
	ownerAccount                                   string
	retainedSourceID                               string
	archive                                        PlainArchive
	pairs                                          []IdentityPair
	ciphertextBytes, containerBytes, trailingBytes uint64
}

// OwnerAccountID is available only on authenticated retained reads. It is a
// message-identity input for offline inspection, never a credential or receipt.
func (a AccountArchive) OwnerAccountID() string { return a.ownerAccount }

func (AccountArchive) String() string                 { return "mobile account archive [redacted]" }
func (AccountArchive) GoString() string               { return "mobile account archive [redacted]" }
func (a AccountArchive) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }
func (a *AccountArchive) Clear() {
	if a == nil {
		return
	}
	a.archive.Clear()
	clear(a.pairs)
	*a = AccountArchive{}
}
func (a AccountArchive) groupFiles() int {
	n := 0
	for _, p := range a.pairs {
		if p.Group {
			n++
		}
	}
	return n
}
func (a AccountArchive) directFiles() int { return len(a.pairs) - a.groupFiles() }

// FetchAccountArchive never selects a conversation or filters by a date.
func FetchAccountArchive(ctx context.Context, d *Downloader, offer domain.MobileBackupOffer, byteLimit int64, mapper domain.MobileIdentitySource) (AccountArchive, error) {
	if ctx == nil || ctx.Err() != nil || d == nil || mapper == nil || byteLimit < 1 || byteLimit > 512<<20 {
		return AccountArchive{}, ErrArchive
	}
	encrypted, err := d.Fetch(ctx, offer.URL, offer.FileSize, uint64(byteLimit))
	if err != nil {
		return AccountArchive{}, ErrArchive
	}
	defer clear(encrypted)
	decoded, err := ReadFormat1Archive(ctx, bytes.NewReader(encrypted), offer.KeyText, uint64(byteLimit), MaxTotalBytes)
	if err != nil {
		return AccountArchive{}, ErrArchive
	}
	defer decoded.Clear()
	request, err := ArchiveIdentityRequest(ctx, decoded.Archive)
	if err != nil {
		return AccountArchive{}, ErrArchive
	}
	pairs, err := mapper.MapMobileBackupIdentities(ctx, request)
	defer clear(pairs)
	if err != nil || ctx.Err() != nil || len(pairs) == 0 {
		return AccountArchive{}, ErrArchive
	}
	result, err := ownAccountArchive(ctx, &decoded, pairs)
	if err != nil {
		return AccountArchive{}, ErrArchive
	}
	slog.Info("mobile_account_archive_acquired", "files", len(result.pairs), "direct_files", result.directFiles(), "group_files", result.groupFiles())
	return result, nil
}

// Transfers the decoded buffers only after the complete mapping is verified.
func ownAccountArchive(ctx context.Context, decoded *Format1Archive, pairs []IdentityPair) (AccountArchive, error) {
	if decoded == nil || len(pairs) == 0 {
		return AccountArchive{}, ErrArchive
	}
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: pairs[0].Session}
	if pairs[0].Group {
		ref.Type = domain.ConversationGroup
	}
	// The selected-index validator checks the entire mapping, not just this ref.
	if _, err := SelectArchiveIndex(ctx, decoded.Archive, pairs, ref); err != nil {
		return AccountArchive{}, ErrArchive
	}
	result := AccountArchive{archive: decoded.Archive, pairs: append([]IdentityPair(nil), pairs...), ciphertextBytes: decoded.CiphertextBytes, containerBytes: decoded.ContainerBytes, trailingBytes: decoded.TrailingBytes}
	decoded.Archive = PlainArchive{}

	if ctx.Err() != nil {
		result.Clear()
		return AccountArchive{}, ErrArchive
	}
	return result, nil
}
