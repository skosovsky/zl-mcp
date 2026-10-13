package mobilebackup

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

//nolint:recvcheck // Value String/GoString must redact formatted values; pointer methods mutate or clear owned state.
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
func ConvertPreparedArchivePage(
	ctx context.Context,
	page PreparedArchivePage,
	request domain.MobileBackupRequest,
	account string,
	nowMS int64,
) (ConvertedArchivePage, error) {
	var result ConvertedArchivePage
	if page.SourceWAL || !archiveControlsClassified(page) {
		return result, ErrSQLite
	}
	return convertArchiveCandidates(ctx, page, request, account, nowMS)
}

// ArchivePageInspection exposes counts only, never records eligible for storage.
type ArchivePageInspection struct {
	TextCandidates            int            `json:"TextCandidates"`
	Expired                   int            `json:"Expired"`
	UnsupportedContent        int            `json:"UnsupportedContent"`
	UnresolvedQuotes          int            `json:"UnresolvedQuotes"`
	UnresolvedMentions        int            `json:"UnresolvedMentions"`
	OwnRecallCandidates       int            `json:"OwnRecallCandidates"`
	BlockReasons              []string       `json:"BlockReasons"`
	UnsupportedContentReasons map[string]int `json:"UnsupportedContentReasons"`
	UnsupportedContentKinds   map[string]int `json:"UnsupportedContentKinds"`
}

// InspectPreparedArchivePage separates source safety gates from invalid records.
// Temporary records are cleared even when the source cannot be imported.
func InspectPreparedArchivePage(
	ctx context.Context,
	page PreparedArchivePage,
	request domain.MobileBackupRequest,
	account string,
	nowMS int64,
) (ArchivePageInspection, error) {
	converted, err := convertArchiveCandidates(ctx, page, request, account, nowMS)
	defer converted.Clear()
	if err != nil {
		return ArchivePageInspection{}, err
	}
	result := ArchivePageInspection{
		TextCandidates:     len(converted.Records),
		Expired:            converted.Expired,
		UnsupportedContent: converted.UnsupportedContent,
		UnresolvedQuotes:   converted.UnresolvedQuotes,
		UnresolvedMentions: converted.UnresolvedMentions,
		BlockReasons:       []string{},
	}
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

//nolint:nonamedreturns // Deferred cleanup observes the returned result/error to preserve ownership and panic/error semantics.
func convertArchiveCandidates(
	ctx context.Context,
	page PreparedArchivePage,
	request domain.MobileBackupRequest,
	account string,
	nowMS int64,
) (result ConvertedArchivePage, err error) {
	r, from, to, bindingErr := bindArchiveCandidates(ctx, page, request, account, nowMS)
	if bindingErr != nil {
		return result, bindingErr
	}
	if recallErr := validateCandidateRecalls(ctx, page, r, account, from, to); recallErr != nil {
		return result, recallErr
	}
	if validationErr := validateCandidateRows(ctx, page, r, account, from, to); validationErr != nil {
		return result, validationErr
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
			recordUnsupportedCandidate(&result, row)
			continue
		}
		if row.Metadata.Quote != nil && !row.QuoteExpired {
			result.UnresolvedQuotes++
		}
		result.UnresolvedMentions += len(row.Metadata.Mentions)
		message, messageErr := candidateMessage(row, r, text, rich)
		if messageErr != nil {
			return result, messageErr
		}
		result.Records = append(
			result.Records,
			domain.ExpiringHistoryRecord{Message: message, ExpiresAtMS: row.ExpiresMS},
		)
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
	if slices.Contains(row.Metadata.UnsupportedTags, 6) {
		return "unparsed_attachment"
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
func archiveText(row PreparedRow) (string, bool, bool, bool) {
	var text string
	var rich bool
	var supported bool
	var valid bool
	text, valid = row.Row.Text, true
	if row.Kind != "webchat" {
		return text, rich, supported, valid
	}
	if slices.Contains(row.Metadata.UnsupportedTags, 6) {
		return text, rich, supported, valid
	}
	if len(row.Metadata.Attachments) == 0 {
		supported = true
		return text, rich, supported, valid
	}
	if len(row.Metadata.Attachments) != 1 {
		return text, rich, supported, valid
	}
	a := row.Metadata.Attachments[0]
	if !utf8.Valid(a.Action.Bytes) || !utf8.Valid(a.Title.Bytes) || len(a.Title.Bytes) > 1<<20 {
		valid = false
		return text, rich, supported, valid
	}
	if !a.Action.Present || len(a.Action.Bytes) == 0 {
		supported = true
		return text, rich, supported, valid
	}
	if string(a.Action.Bytes) != "rtf" {
		return text, rich, supported, valid
	}
	if a.Title.Present && len(a.Title.Bytes) > 0 {
		text = string(a.Title.Bytes)
	}
	rich, supported = true, true
	return text, rich, supported, valid
}

func bindArchiveCandidates(ctx context.Context, page PreparedArchivePage, request domain.MobileBackupRequest, account string, nowMS int64) (domain.MobileBackupRequest, int64, int64, error) {
	if ctx == nil || ctx.Err() != nil || !canonicalIdentity(account) || nowMS <= 0 || len(page.Candidates.Rows) > 50 ||
		page.SourceControls < 0 {
		return domain.MobileBackupRequest{}, 0, 0, ErrSQLite
	}
	r, e := request.Normalize()
	if e != nil || !canonicalIdentity(r.ConversationID) || len(page.Candidates.Rows) > r.MaxMessages ||
		page.requestID != r.RequestID ||
		page.fingerprint != r.Fingerprint() ||
		page.account != account {
		return domain.MobileBackupRequest{}, 0, 0, ErrSQLite
	}
	since, _ := time.Parse(time.RFC3339Nano, r.Since)
	until, _ := time.Parse(time.RFC3339Nano, r.Until)
	from, to, validWindow := millisecondWindow(since, until)
	if !validWindow {
		return domain.MobileBackupRequest{}, 0, 0, ErrSQLite
	}
	return r, from, to, nil
}

func validateCandidateRecalls(ctx context.Context, page PreparedArchivePage, r domain.MobileBackupRequest, account string, from, to int64) error {
	if len(page.Candidates.OwnRecalls) > page.Candidates.DeferredControls ||
		len(page.Candidates.OwnRecalls) > page.Candidates.Examined ||
		len(page.Candidates.OwnRecalls) > page.SourceControls ||
		len(page.Candidates.OwnRecalls) > min(50, r.MaxMessages) {
		return ErrSQLite
	}
	seenRecalls := map[string]bool{}
	for _, recall := range page.Candidates.OwnRecalls {
		if ctx.Err() != nil || !canonicalIdentity(recall.MessageID) || seenRecalls[recall.MessageID] ||
			recall.Conversation != r.Ref() ||
			recall.Conversation.Type != domain.ConversationDirect ||
			recall.SenderID != account ||
			recall.RecordAtMS < from ||
			recall.RecordAtMS >= to {
			return ErrSQLite
		}
		seenRecalls[recall.MessageID] = true
	}
	return nil
}

func validateCandidateRows(ctx context.Context, page PreparedArchivePage, r domain.MobileBackupRequest, account string, from, to int64) error {
	total := 0
	// Whole-page validation precedes classification and output construction.
	for _, row := range page.Candidates.Rows {
		total += len(row.Row.Text) + len(row.Row.BinNet)
		projected, _, _, valid := archiveText(row)
		if !valid {
			return ErrSQLite
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
		if ctx.Err() != nil || total > 8<<20 || !known || kind != row.Kind || !canonicalIdentity(row.Row.MessageID) ||
			!canonicalIdentity(row.Row.ClientID) ||
			!canonicalIdentity(row.SenderID) ||
			row.Direction != direction ||
			row.Row.Status <= 0 ||
			row.Row.TimestampMS < from ||
			row.Row.TimestampMS >= to ||
			!utf8.ValidString(row.Row.Text) ||
			len(row.Row.Text) > 1<<20 ||
			e != nil ||
			expiry != row.ExpiresMS ||
			declared != row.ExpiryDeclared ||
			r.ConversationType == domain.ConversationDirect && row.SenderID != account &&
				row.SenderID != r.ConversationID {
			return ErrSQLite
		}
	}
	return nil
}

func recordUnsupportedCandidate(result *ConvertedArchivePage, row PreparedRow) {
	result.UnsupportedContent++
	if result.UnsupportedContentReasons == nil {
		result.UnsupportedContentReasons = map[string]int{}
		result.UnsupportedContentKinds = map[string]int{}
	}
	result.UnsupportedContentReasons[unsupportedArchiveContentReason(row)]++
	result.UnsupportedContentKinds[row.Kind]++
}

func candidateMessage(row PreparedRow, r domain.MobileBackupRequest, text string, rich bool) (domain.Message, error) {
	message := domain.Message{
		Conversation:    r.Ref(),
		ID:              row.Row.MessageID,
		SenderID:        row.SenderID,
		SentAt:          time.UnixMilli(row.Row.TimestampMS).UTC(),
		Text:            text,
		Direction:       row.Direction,
		AttachmentTypes: []string{},
		QuoteMetadata: &domain.QuoteMetadata{
			ClientMessageID: row.Row.ClientID,
			MessageType:     "webchat",
			Timestamp:       strconv.FormatInt(row.Row.TimestampMS, 10),
			TTL:             int(row.Row.TTL),
		},
	}
	if rich {
		message.AttachmentTypes = []string{"rtf"}
	}
	if int64(message.QuoteMetadata.TTL) != row.Row.TTL {
		return domain.Message{}, ErrSQLite
	}
	return message, nil
}
