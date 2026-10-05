package domain

import (
	"context"
	"errors"
)

var ErrConversationPreloadUnsupported = errors.New("conversation preload source unsupported")

// PreloadEntry is observed dialogue metadata, never a friendship/Strangers fact.
type PreloadEntry struct {
	Conversation  ConversationRef
	Name          *string
	LastMessageID *string
}

// PreloadSnapshot is bounded available source evidence, not an inbox manifest.
// Messages deliberately have no persistence source. False availability indicates
// absence of the corresponding source category, not source exhaustion.
type PreloadSnapshot struct {
	Entries                 []PreloadEntry
	Messages                []Message
	DirectMessagesAvailable bool
	GroupMessagesAvailable  bool
	UnsupportedMessageCount int
}

type ConversationPreloadSource interface {
	ConversationPreload(context.Context) (PreloadSnapshot, error)
}
