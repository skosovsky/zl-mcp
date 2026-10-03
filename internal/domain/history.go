package domain

import (
	"context"
	"errors"
)

var ErrHistoryUnsupported = errors.New("history source unsupported for this conversation")

// HistoryPage carries read-only source evidence. Messages intentionally have no
// persistence source: an import policy must choose their ingestion semantics.
type HistoryPage struct {
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
