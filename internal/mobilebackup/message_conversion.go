package mobilebackup

import (
	"context"
	"maps"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type ConvertedArchivePage struct {
	Records                                                           []domain.ExpiringHistoryRecord `json:"-"`
	Recalls                                                           []domain.MobileHistoryRecall   `json:"-"`
	Expired, UnsupportedContent, UnresolvedQuotes, UnresolvedMentions int                            `json:"-"`
	UnsupportedContentReasons, UnsupportedContentKinds                map[string]int                 `json:"-"`
}

func (ConvertedArchivePage) String() string   { return "converted mobile archive page [redacted]" }
func (ConvertedArchivePage) GoString() string { return "converted mobile archive page [redacted]" }
func (p *ConvertedArchivePage) Clear() {
	if p != nil {
		for i := range p.Records {
			p.Records[i] = domain.ExpiringHistoryRecord{}
		}
		clear(p.Recalls)
		clear(p.UnsupportedContentReasons)
		clear(p.UnsupportedContentKinds)
		*p = ConvertedArchivePage{}
	}
}

// ConvertPreparedArchivePage borrows already mapped candidates; it performs no import.
func ConvertPreparedArchivePage(ctx context.Context, page PreparedArchivePage, request domain.MobileBackupRequest, account string, nowMS int64) (result ConvertedArchivePage, err error) {
	if page.SourceWAL || !archiveControlsClassified(page) {
		return result, ErrSQLite
	}
	return convertArchiveCandidates(ctx, page, request, account, nowMS)
}

// ArchivePageInspection exposes counts only, never records eligible for storage.
type ArchivePageInspection struct {
	TextCandidates, Expired, UnsupportedContent, UnresolvedQuotes, UnresolvedMentions int
	OwnRecallCandidates                                                               int
	BlockReasons                                                                      []string
	UnsupportedContentReasons, UnsupportedContentKinds                                map[string]int
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
	result.OwnRecallCandidates = len(page.Candidates.OwnRecalls)
	result.UnsupportedContentReasons = maps.Clone(converted.UnsupportedContentReasons)
	result.UnsupportedContentKinds = maps.Clone(converted.UnsupportedContentKinds)
	if page.SourceWAL {
		result.BlockReasons = append(result.BlockReasons, "unverified_wal_snapshot")
	}
	if !archiveControlsClassified(page) {
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
	if len(page.Candidates.OwnRecalls) > page.Candidates.DeferredControls || len(page.Candidates.OwnRecalls) > page.Candidates.Examined || len(page.Candidates.OwnRecalls) > page.SourceControls || len(page.Candidates.OwnRecalls) > min(50, r.MaxMessages) {
		return result, ErrSQLite
	}
	seenRecalls := map[string]bool{}
	for _, recall := range page.Candidates.OwnRecalls {
		if ctx.Err() != nil || !canonicalIdentity(recall.MessageID) || seenRecalls[recall.MessageID] || recall.Conversation != r.Ref() || recall.Conversation.Type != domain.ConversationDirect || recall.SenderID != account || recall.RecordAtMS < from || recall.RecordAtMS >= to {
			return result, ErrSQLite
		}
		seenRecalls[recall.MessageID] = true
	}
	total := 0
	// Whole-page validation precedes classification and output construction.
	for _, row := range page.Candidates.Rows {
		total += len(row.Row.Text) + len(row.Row.BinNet)
		projected, _, _, valid := archiveText(row)
		if !valid {
			return result, ErrSQLite
		}
		if projected != row.Row.Text {
			total += len(projected)
		}
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
		text, rich, supported, _ := archiveText(row)
		if !supported {
			result.UnsupportedContent++
			if result.UnsupportedContentReasons == nil {
				result.UnsupportedContentReasons = map[string]int{}
				result.UnsupportedContentKinds = map[string]int{}
			}
			result.UnsupportedContentReasons[unsupportedArchiveContentReason(row)]++
			result.UnsupportedContentKinds[row.Kind]++
			continue
		}
		if row.Metadata.Quote != nil && !row.QuoteExpired {
			result.UnresolvedQuotes++
		}
		result.UnresolvedMentions += len(row.Metadata.Mentions)
		message := domain.Message{Conversation: r.Ref(), ID: row.Row.MessageID, SenderID: row.SenderID, SentAt: time.UnixMilli(row.Row.TimestampMS).UTC(), Text: text, Direction: row.Direction, AttachmentTypes: []string{}, QuoteMetadata: &domain.QuoteMetadata{ClientMessageID: row.Row.ClientID, MessageType: "webchat", Timestamp: strconv.FormatInt(row.Row.TimestampMS, 10), TTL: int(row.Row.TTL)}}
		if rich {
			message.AttachmentTypes = []string{"rtf"}
		}
		if int64(message.QuoteMetadata.TTL) != row.Row.TTL {
			return result, ErrSQLite
		}
		result.Records = append(result.Records, domain.ExpiringHistoryRecord{Message: message, ExpiresAtMS: row.ExpiresMS})
	}
	if ctx.Err() != nil {
		return result, ErrSQLite
	}
	result.Recalls = append([]domain.MobileHistoryRecall(nil), page.Candidates.OwnRecalls...)
	return result, nil
}

// Called only for validated, unexpired rows rejected by archiveText. Never
// include private attachment values in the diagnostic category.
func unsupportedArchiveContentReason(row PreparedRow) string {
	if row.Kind != "webchat" {
		return "non_text_kind"
	}
	for _, tag := range row.Metadata.UnsupportedTags {
		if tag == 6 {
			return "unparsed_attachment"
		}
	}
	if len(row.Metadata.Attachments) > 1 {
		return "multiple_attachments"
	}
	return "unsupported_attachment_action"
}

// Only a complete first-page classification or its immutable accepted checkpoint
// can prove there is no unseen suppressing control elsewhere in this source.
func archiveControlsClassified(page PreparedArchivePage) bool {
	return page.SourceControls >= 0 && (page.controlsVerified || page.SourceControls == len(page.Candidates.OwnRecalls))
}

// archiveText follows the installed MSG_TEXT title-or-MsgContent rule. It does
// not render styling or interpret arbitrary attachment params/URLs.
func archiveText(row PreparedRow) (text string, rich, supported, valid bool) {
	text, valid = row.Row.Text, true
	if row.Kind != "webchat" {
		return
	}
	for _, tag := range row.Metadata.UnsupportedTags {
		if tag == 6 {
			return
		}
	}
	if len(row.Metadata.Attachments) == 0 {
		supported = true
		return
	}
	if len(row.Metadata.Attachments) != 1 {
		return
	}
	a := row.Metadata.Attachments[0]
	if !utf8.Valid(a.Action.Bytes) || !utf8.Valid(a.Title.Bytes) || len(a.Title.Bytes) > 1<<20 {
		valid = false
		return
	}
	if !a.Action.Present || len(a.Action.Bytes) == 0 {
		supported = true
		return
	}
	if string(a.Action.Bytes) != "rtf" {
		return
	}
	if a.Title.Present && len(a.Title.Bytes) > 0 {
		text = string(a.Title.Bytes)
	}
	rich, supported = true, true
	return
}
