package domain

import (
	"context"
	"errors"
)

var ErrHistoryUnsupported = errors.New("history source unsupported for this conversation")
var ErrHistoryInvalidPage = errors.New("invalid history source page")

// HistorySourceFailure carries only a numeric protocol code across the adapter
// boundary. Error deliberately omits upstream text, URLs and response bodies.
type HistorySourceFailure struct {
	APICode *int
	cause   error
}

func NewHistorySourceFailure(code *int, cause error) *HistorySourceFailure {
	f := &HistorySourceFailure{cause: cause}
	if code != nil {
		value := *code
		f.APICode = &value
	}
	return f
}

func (f *HistorySourceFailure) Error() string { return "history source API failure" }
func (f *HistorySourceFailure) Unwrap() error { return f.cause }

// HistoryPage carries read-only source evidence. Messages intentionally have no
// persistence source: an import policy must choose their ingestion semantics.
type HistoryPage struct {
	PhaseRequired        bool
	LimitedSnapshot      bool
	Messages             []Message
	HasMore              *bool
	Cursor               *string
	IsFiltered           *bool
	IsFilteredByPhase    *bool
	IsFilteredByTimeJoin *bool
	IsOld                *bool
	JoinTimestampMillis  *string
}

type HistorySource interface {
	HistoryPage(context.Context, ConversationRef, string, int) (HistoryPage, error)
}

// PhasedHistorySource uses the previous page's durable source phase. The first
// request is recent; a missing phase after a page must not be guessed.
type PhasedHistorySource interface {
	HistoryPageWithPhase(context.Context, ConversationRef, string, *bool, int) (HistoryPage, error)
	HistoryPhaseRequired() bool
}

// HistoryPageFailure exposes only a closed set of normalization reasons.
type HistoryPageFailure struct{ reason string }

func NewHistoryPageFailure(message string) *HistoryPageFailure {
	reason := "unknown"
	switch message {
	case "invalid group history shape":
		reason = "invalid_group_history_shape"
	case "invalid group history records":
		reason = "invalid_group_history_records"
	case "invalid group history hasMore":
		reason = "invalid_group_history_hasMore"
	case "invalid group history isFiltered":
		reason = "invalid_group_history_isFiltered"
	case "invalid group history isFilteredByPhase":
		reason = "invalid_group_history_isFilteredByPhase"
	case "invalid group history isFilteredByTimeJoin":
		reason = "invalid_group_history_isFilteredByTimeJoin"
	case "invalid group history isOld":
		reason = "invalid_group_history_isOld"

	case "invalid preload last message identifier":
		reason = "invalid_preload_last_message_identifier"
	case "invalid preload message identifier actionId":
		reason = "invalid_preload_message_identifier_actionId"
	case "invalid preload message identifier msgId":
		reason = "invalid_preload_message_identifier_msgId"
	case "invalid preload message identifier cliMsgId":
		reason = "invalid_preload_message_identifier_cliMsgId"
	case "invalid preload message identifier uidFrom":
		reason = "invalid_preload_message_identifier_uidFrom"
	case "invalid preload message identifier idTo":
		reason = "invalid_preload_message_identifier_idTo"
	case "invalid preload message identifier ts":
		reason = "invalid_preload_message_identifier_ts"
	case "invalid preload message identifier userId":
		reason = "invalid_preload_message_identifier_userId"
	case "invalid preload message identifier realMsgId":
		reason = "invalid_preload_message_identifier_realMsgId"
	case "ambiguous preload outgoing peer":
		reason = "ambiguous_preload_outgoing_peer"
	case "duplicate preload dialogue identity":
		reason = "duplicate_preload_dialogue_identity"
	case "history page exceeds normalization budget":
		reason = "history_page_exceeds_normalization_budget"
	case "history record does not match requested conversation":
		reason = "history_record_does_not_match_requested_conversation"
	case "history source returned internal error":
		reason = "history_source_returned_internal_error"
	case "history text exceeds supported size":
		reason = "history_text_exceeds_supported_size"
	case "invalid decimal metadata":
		reason = "invalid_decimal_metadata"
	case "invalid history join timestamp":
		reason = "invalid_history_join_timestamp"
	case "invalid history message":
		reason = "invalid_history_message"
	case "invalid history message shape":
		reason = "invalid_history_message_shape"
	case "invalid history numeric identifier":
		reason = "invalid_history_numeric_identifier"
	case "invalid history page records":
		reason = "invalid_history_page_records"
	case "invalid preload dialogue identity":
		reason = "invalid_preload_dialogue_identity"
	case "invalid preload identifier":
		reason = "invalid_preload_identifier"
	case "invalid preload message":
		reason = "invalid_preload_message"
	case "invalid preload message conversation":
		reason = "invalid_preload_message_conversation"
	case "invalid preload message fields":
		reason = "invalid_preload_message_fields"
	case "invalid preload message metadata":
		reason = "invalid_preload_message_metadata"
	case "invalid preload message shape":
		reason = "invalid_preload_message_shape"
	case "invalid preload metadata":
		reason = "invalid_preload_metadata"
	case "invalid preload numeric identifier":
		reason = "invalid_preload_numeric_identifier"
	case "invalid preload page bounds":
		reason = "invalid_preload_page_bounds"
	case "missing preload account binding":
		reason = "missing_preload_account_binding"
	case "preload message belongs to a foreign account":
		reason = "preload_message_belongs_to_a_foreign_account"
	case "preload normalization exceeds bounds":
		reason = "preload_normalization_exceeds_bounds"
	case "preload record exceeds bounds":
		reason = "preload_record_exceeds_bounds"
	case "unknown preload dialogue classification":
		reason = "unknown_preload_dialogue_classification"
	case "unsupported history message fields":
		reason = "unsupported_history_message_fields"
	case "unsupported history message metadata":
		reason = "unsupported_history_message_metadata"
	}
	return &HistoryPageFailure{reason: reason}
}
func (f *HistoryPageFailure) Reason() string { return f.reason }
func (f *HistoryPageFailure) Error() string  { return "invalid history source page: " + f.reason }
func (f *HistoryPageFailure) Unwrap() error  { return ErrHistoryInvalidPage }

// PreloadHistorySource selects one bounded available snapshot, never deeper history.
type PreloadHistorySource interface {
	PreloadHistoryPage(context.Context, ConversationRef, int) (HistoryPage, error)
}
