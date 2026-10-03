package domain

// ConversationRef keeps otherwise colliding upstream ID namespaces separate.
// IDs are opaque strings. A direct ID always identifies the peer, not sender.
type ConversationRef struct {
	Type string `json:"conversation_type" toml:"type"`
	ID   string `json:"conversation_id" toml:"id"`
}

const (
	ConversationDirect           = "direct"
	ConversationGroup            = "group"
	LegacyMessageCreated         = "zalo.message.created"
	ConversationMessageCreated   = "zalo.conversation.message.created"
	ConversationMessageCreatedV2 = "zalo.conversation.message.created.v2"
)

func (r ConversationRef) Valid() bool {
	return (r.Type == ConversationDirect || r.Type == ConversationGroup) && r.ID != ""
}

func (m Message) Ref() ConversationRef {
	if m.Conversation.Type != "" || m.Conversation.ID != "" {
		return m.Conversation
	}
	return ConversationRef{Type: ConversationGroup, ID: m.GroupID}
}

// CollectionPolicy is shared by ingestion, reads and subscription validation.
// A zero policy collects nothing; all mode also admits newly discovered peers.
type CollectionPolicy struct {
	All      bool
	Selected map[ConversationRef]bool
}

func (p CollectionPolicy) Allows(r ConversationRef) bool {
	return r.Valid() && (p.All || p.Selected[r])
}
