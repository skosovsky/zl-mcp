package domain

import (
	"errors"
	"strconv"
)

// ErrMessageActionRejected represents confirmed upstream refusal, never a timeout.
var ErrMessageActionRejected = errors.New("message action rejected by upstream")

// MessageActionTarget is resolved by the service from an exact retained message.
// It is private adapter input, never accepted as MCP arguments or returned data.
type MessageActionTarget struct {
	Conversation    ConversationRef
	MessageID       string
	ClientMessageID string
	SenderID        string
	Seen            *SeenMetadata
}

// ValidActionMessageID checks the numeric range required by the pinned native
// write protocol. Other read APIs continue treating message IDs as opaque.
func ValidActionMessageID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

// ReadReceiptResult preserves bounded protocol evidence without asserting a UI
// effect. Missing status and status zero are intentionally distinct.
type ReadReceiptResult struct{ Status *int }

// ErrUnverifiedReadStatus keeps unknown protocol semantics out of accepted state.
var ErrUnverifiedReadStatus = errors.New("read receipt status is not yet verified")
