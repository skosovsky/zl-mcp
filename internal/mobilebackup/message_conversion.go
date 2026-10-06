package mobilebackup

import (
	"context"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type ConvertedArchivePage struct {
	Records                                                           []domain.ExpiringHistoryRecord `json:"-"`
	Expired, UnsupportedContent, UnresolvedQuotes, UnresolvedMentions int                            `json:"-"`
}

func (ConvertedArchivePage) String() string   { return "converted mobile archive page [redacted]" }
func (ConvertedArchivePage) GoString() string { return "converted mobile archive page [redacted]" }
func (p *ConvertedArchivePage) Clear() {
	if p != nil {
		for i := range p.Records {
			p.Records[i] = domain.ExpiringHistoryRecord{}
		}
		*p = ConvertedArchivePage{}
	}
}

// ConvertPreparedArchivePage borrows already mapped candidates; it performs no import.
func ConvertPreparedArchivePage(ctx context.Context, page PreparedArchivePage, request domain.MobileBackupRequest, account string, nowMS int64) (result ConvertedArchivePage, err error) {
	if page.SourceWAL || page.SourceControls > 0 {
		return result, ErrSQLite
	}
	return convertArchiveCandidates(ctx, page, request, account, nowMS)
}

// ArchivePageInspection exposes counts only, never records eligible for storage.
type ArchivePageInspection struct {
	TextCandidates, Expired, UnsupportedContent, UnresolvedQuotes, UnresolvedMentions int
	BlockReasons                                                                      []string
}

// InspectPreparedArchivePage separates source safety gates from invalid records.
// Temporary records are cleared even when the source cannot be imported.
func InspectPreparedArchivePage(ctx context.Context, page PreparedArchivePage, request domain.MobileBackupRequest, account string, nowMS int64) (ArchivePageInspection, error) {
	converted, err := convertArchiveCandidates(ctx, page, request, account, nowMS)
	defer converted.Clear()
	if err != nil {
		return ArchivePageInspection{}, err
	}
	result := ArchivePageInspection{TextCandidates: len(converted.Records), Expired: converted.Expired, UnsupportedContent: converted.UnsupportedContent, UnresolvedQuotes: converted.UnresolvedQuotes, UnresolvedMentions: converted.UnresolvedMentions, BlockReasons: []string{}}
	if page.SourceWAL {
		result.BlockReasons = append(result.BlockReasons, "unverified_wal_snapshot")
	}
	if page.SourceControls > 0 {
		result.BlockReasons = append(result.BlockReasons, "unverified_source_controls")
	}
	return result, nil
}

func convertArchiveCandidates(ctx context.Context, page PreparedArchivePage, request domain.MobileBackupRequest, account string, nowMS int64) (result ConvertedArchivePage, err error) {
	if ctx == nil || ctx.Err() != nil || !canonicalIdentity(account) || nowMS <= 0 || len(page.Candidates.Rows) > 50 || page.SourceControls < 0 {
		return result, ErrSQLite
	}
	r, e := request.Normalize()
	if e != nil || !canonicalIdentity(r.ConversationID) || len(page.Candidates.Rows) > r.MaxMessages || page.requestID != r.RequestID || page.fingerprint != r.Fingerprint() || page.account != account {
		return result, ErrSQLite
	}
	since, _ := time.Parse(time.RFC3339Nano, r.Since)
	until, _ := time.Parse(time.RFC3339Nano, r.Until)
	from, to, validWindow := millisecondWindow(since, until)
	if !validWindow {
		return result, ErrSQLite
	}
	total := 0
	// Whole-page validation precedes classification and output construction.
	for _, row := range page.Candidates.Rows {
		total += len(row.Row.Text) + len(row.Row.BinNet)
		kind, known := mobilePayloadKind(row.Row.Type)
		expiry, declared, e := MessageExpiryMS(row.Row.TimestampMS, row.Row.TTL)
		direction := "incoming"
		if row.SenderID == account {
			direction = "outgoing"
		}
		if ctx.Err() != nil || total > 8<<20 || !known || kind != row.Kind || !canonicalIdentity(row.Row.MessageID) || !canonicalIdentity(row.Row.ClientID) || !canonicalIdentity(row.SenderID) || row.Direction != direction || row.Row.Status <= 0 || row.Row.TimestampMS < from || row.Row.TimestampMS >= to || !utf8.ValidString(row.Row.Text) || len(row.Row.Text) > 1<<20 || e != nil || expiry != row.ExpiresMS || declared != row.ExpiryDeclared || r.ConversationType == domain.ConversationDirect && row.SenderID != account && row.SenderID != r.ConversationID {
			return result, ErrSQLite
		}
	}
	defer func() {
		if err != nil {
			result.Clear()
		}
	}()
	for _, row := range page.Candidates.Rows {
		if ctx.Err() != nil {
			return result, ErrSQLite
		}
		if row.ExpiryDeclared && row.ExpiresMS <= nowMS {
			result.Expired++
			continue
		}
		attachment := len(row.Metadata.Attachments) > 0
		for _, tag := range row.Metadata.UnsupportedTags {
			if tag == 6 {
				attachment = true
			}
		}
		if row.Kind != "webchat" || attachment {
			result.UnsupportedContent++
			continue
		}
		if row.Metadata.Quote != nil && !row.QuoteExpired {
			result.UnresolvedQuotes++
		}
		result.UnresolvedMentions += len(row.Metadata.Mentions)
		message := domain.Message{Conversation: r.Ref(), ID: row.Row.MessageID, SenderID: row.SenderID, SentAt: time.UnixMilli(row.Row.TimestampMS).UTC(), Text: row.Row.Text, Direction: row.Direction, AttachmentTypes: []string{}, QuoteMetadata: &domain.QuoteMetadata{ClientMessageID: row.Row.ClientID, MessageType: "webchat", Timestamp: strconv.FormatInt(row.Row.TimestampMS, 10), TTL: int(row.Row.TTL)}}
		if int64(message.QuoteMetadata.TTL) != row.Row.TTL {
			return result, ErrSQLite
		}
		result.Records = append(result.Records, domain.ExpiringHistoryRecord{Message: message, ExpiresAtMS: row.ExpiresMS})
	}
	if ctx.Err() != nil {
		return result, ErrSQLite
	}
	return result, nil
}
