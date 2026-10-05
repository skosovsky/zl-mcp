package mobilebackup

import (
	"context"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type PreparedArchiveCursor struct {
	requestID, fingerprint, account string
	SourceControls                  int `json:"-"`
	examined                        int
	sqlite                          SQLiteCursor
}

func (PreparedArchiveCursor) String() string   { return "mobile backup prepared cursor [redacted]" }
func (PreparedArchiveCursor) GoString() string { return "mobile backup prepared cursor [redacted]" }

type PreparedArchivePage struct {
	SourceControls                                     int `json:"-"`
	requestID, fingerprint, account                    string
	Candidates                                         PreparedRowPage        `json:"-"`
	Examined, Rejected, ExpiredMessages, ExpiredQuotes int                    `json:"-"`
	SourceHasMore, BudgetExhausted                     bool                   `json:"-"`
	Next                                               *PreparedArchiveCursor `json:"-"`
}

func (PreparedArchivePage) String() string   { return "mobile backup prepared archive page [redacted]" }
func (PreparedArchivePage) GoString() string { return "mobile backup prepared archive page [redacted]" }
func (p *PreparedArchivePage) Clear() {
	if p != nil {
		p.Candidates.Clear()
		if p.Next != nil {
			*p.Next = PreparedArchiveCursor{}
		}
		*p = PreparedArchivePage{}
	}
}

func ReadPreparedArchivePage(ctx context.Context, archive SelectedArchive, r domain.MobileBackupRequest, account, scratch string, nowMS int64, size int, after *PreparedArchiveCursor, mapper domain.MobileIdentitySource) (result PreparedArchivePage, err error) {
	if ctx == nil || ctx.Err() != nil || nowMS <= 0 || size < 1 || size > 50 || !canonicalIdentity(account) || mapper == nil {
		return result, ErrSQLite
	}
	normalized, e := r.Normalize()
	if e != nil || archive.ref != normalized.Ref() || archive.requestID != normalized.RequestID || archive.requestFingerprint == "" || archive.requestFingerprint != normalized.Fingerprint() {
		return result, ErrSQLite
	}
	examined := 0
	var sqliteAfter *SQLiteCursor
	if after != nil {
		if after.requestID != normalized.RequestID || after.fingerprint != normalized.Fingerprint() || after.account != account || after.examined <= 0 || after.examined >= normalized.MaxMessages {
			return result, ErrSQLite
		}
		examined = after.examined
		sqliteAfter = &after.sqlite
	}
	remaining := normalized.MaxMessages - examined
	if size > remaining {
		size = remaining
	}
	since, _ := time.Parse(time.RFC3339Nano, normalized.Since)
	until, _ := time.Parse(time.RFC3339Nano, normalized.Until)
	batch, e := ReadSQLitePage(ctx, archive.File, scratch, since, until, size, sqliteAfter)
	if e != nil {
		return result, ErrSQLite
	}
	defer batch.Clear()
	defer func() {
		if err != nil {
			result.Clear()
		}
	}()
	result.SourceControls = batch.SourceControls
	result.requestID, result.fingerprint, result.account = normalized.RequestID, normalized.Fingerprint(), account
	result.Candidates, e = PrepareRowPage(ctx, batch.Rows, normalized, account, mapper)
	if e != nil {
		return result, ErrSQLite
	}
	result.ExpiredMessages, result.ExpiredQuotes, e = result.Candidates.ApplyExpiry(nowMS)
	if e != nil || ctx.Err() != nil {
		return result, ErrSQLite
	}
	result.Examined, result.Rejected = result.Candidates.Examined+batch.Rejected, batch.Rejected
	result.SourceHasMore = batch.HasMore
	examined += result.Examined
	result.BudgetExhausted = batch.HasMore && examined >= normalized.MaxMessages
	if batch.HasMore && !result.BudgetExhausted {
		if batch.Next == nil || result.Examined == 0 {
			return result, ErrSQLite
		}
		result.Next = &PreparedArchiveCursor{requestID: normalized.RequestID, fingerprint: normalized.Fingerprint(), account: account, examined: examined, sqlite: *batch.Next}
	}
	return result, nil
}
