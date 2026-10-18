package domain

import "time"

// MessageActionOperation describes upstream acceptance separately from observed
// Zalo UI state. It contains no native metadata, credentials or message text.
type MessageActionOperation struct {
	UpstreamStatus *int            `json:"upstream_status"`
	Plan           string          `json:"plan"`
	RequestID      string          `json:"request_id"`
	Kind           string          `json:"kind"`
	Conversation   ConversationRef `json:"conversation"`
	Status         string          `json:"status"`
	Stage          string          `json:"stage"`
	CompletedSteps int             `json:"completed_steps"`
	Reason         *string         `json:"reason"`
	UpdatedAt      time.Time       `json:"updated_at"`
	RetrySafe      bool            `json:"retry_safe"`
}
