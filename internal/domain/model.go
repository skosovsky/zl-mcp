package domain

import "time"

type Group struct {
	ID                string `json:"group_id"`
	Name              string `json:"name"`
	MemberCount       *int   `json:"member_count"`
	CollectionEnabled bool   `json:"collection_enabled"`
}
type Message struct {
	Direction        string         `json:"-"`
	FirstIncoming    *bool          `json:"-"`
	QuoteMetadata    *QuoteMetadata `json:"-"`
	ConversationName *string        `json:"-"`
	// Conversation is the internal typed identity. Legacy JSON remains group-only;
	// conversation tools/events use their dedicated response contracts.
	Conversation    ConversationRef `json:"-"`
	GroupID         string          `json:"group_id"`
	ID              string          `json:"message_id"`
	SenderID        string          `json:"sender_id"`
	SenderName      *string         `json:"sender_name"`
	SentAt          time.Time       `json:"sent_at"`
	Text            string          `json:"text"`
	ReplyTo         *string         `json:"reply_to_message_id"`
	AttachmentTypes []string        `json:"attachment_types"`
	Source          string          `json:"source"`
	TextTruncated   bool            `json:"text_truncated"`
	TextResourceURI *string         `json:"text_resource_uri"`
}
type SearchHit struct {
	Conversation  ConversationRef `json:"-"`
	GroupID       string          `json:"group_id"`
	ID            string          `json:"message_id"`
	SenderID      string          `json:"sender_id"`
	GroupName     *string         `json:"group_name"`
	SenderName    *string         `json:"sender_name"`
	SentAt        time.Time       `json:"sent_at"`
	Excerpt       string          `json:"excerpt"`
	TextTruncated bool            `json:"text_truncated"`
}
type NextAction struct {
	Tool        *string        `json:"tool"`
	Arguments   map[string]any `json:"arguments"`
	Instruction string         `json:"instruction"`
}
type Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Retryable  bool           `json:"retryable"`
	NextAction NextAction     `json:"next_action"`
	Details    map[string]any `json:"details"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func Invalid(message string) error {
	return &Error{Code: "INVALID_ARGUMENT", Message: message, NextAction: NextAction{Instruction: "Correct the arguments using the tool input schema."}, Details: map[string]any{}}
}

type Gap struct {
	From   time.Time  `json:"from"`
	To     *time.Time `json:"to"`
	Reason string     `json:"reason"`
}
type Coverage struct {
	GroupID             string     `json:"group_id"`
	CollectionStartedAt *time.Time `json:"collection_started_at"`
	EarliestStoredAt    *time.Time `json:"earliest_stored_at"`
	LatestStoredAt      *time.Time `json:"latest_stored_at"`
	KnownGaps           []Gap      `json:"known_gaps"`
	HistoryComplete     bool       `json:"history_complete"`
	RetentionDays       *int       `json:"retention_days"`
}

// Invite contains only domain metadata; upstream protocol types never cross this port.
type Invite struct {
	Group            Group
	Description      *string
	ApprovalRequired bool
}

// ResponseTooLarge distinguishes a valid request whose result cannot fit from
// invalid arguments or a failure of the local database.
func ResponseTooLarge(message, instruction string) error {
	return &Error{Code: "RESPONSE_TOO_LARGE", Message: message, NextAction: NextAction{Instruction: instruction}, Details: map[string]any{}}
}
