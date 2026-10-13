// Package shared contains bounded read DTOs shared by conversation capabilities.
package shared

import (
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type ConversationMessage struct {
	ConversationType string    `json:"conversation_type"`
	ConversationID   string    `json:"conversation_id"`
	ID               string    `json:"message_id"`
	SenderID         string    `json:"sender_id"`
	SenderName       *string   `json:"sender_name"`
	SentAt           time.Time `json:"sent_at"`
	Text             string    `json:"text"`
	ReplyTo          *string   `json:"reply_to_message_id"`
	AttachmentTypes  []string  `json:"attachment_types"`
	Source           string    `json:"source"`
	TextTruncated    bool      `json:"text_truncated"`
	TextResourceURI  *string   `json:"text_resource_uri"`
}

type ConversationHit struct {
	ConversationType string    `json:"conversation_type"`
	ConversationID   string    `json:"conversation_id"`
	ConversationName *string   `json:"conversation_name"`
	ID               string    `json:"message_id"`
	SenderID         string    `json:"sender_id"`
	SenderName       *string   `json:"sender_name"`
	SentAt           time.Time `json:"sent_at"`
	Excerpt          string    `json:"excerpt"`
	TextTruncated    bool      `json:"text_truncated"`
}

type ConversationCoverage struct {
	ConversationType    string       `json:"conversation_type"`
	ConversationID      string       `json:"conversation_id"`
	CollectionStartedAt *time.Time   `json:"collection_started_at"`
	EarliestStoredAt    *time.Time   `json:"earliest_stored_at"`
	LatestStoredAt      *time.Time   `json:"latest_stored_at"`
	KnownGaps           []domain.Gap `json:"known_gaps"`
	HistoryComplete     bool         `json:"history_complete"`
	RetentionDays       *int         `json:"retention_days"`
}

func Coverage(ref domain.ConversationRef, c domain.Coverage) ConversationCoverage {
	return ConversationCoverage{
		ConversationType:    ref.Type,
		ConversationID:      ref.ID,
		CollectionStartedAt: c.CollectionStartedAt,
		EarliestStoredAt:    c.EarliestStoredAt,
		LatestStoredAt:      c.LatestStoredAt,
		KnownGaps:           c.KnownGaps,
		HistoryComplete:     c.HistoryComplete,
		RetentionDays:       c.RetentionDays,
	}
}
func Hit(h domain.SearchHit) ConversationHit {
	return ConversationHit{
		ConversationType: h.Conversation.Type,
		ConversationID:   h.Conversation.ID,
		ConversationName: h.GroupName,
		ID:               h.ID,
		SenderID:         h.SenderID,
		SenderName:       h.SenderName,
		SentAt:           h.SentAt,
		Excerpt:          h.Excerpt,
		TextTruncated:    h.TextTruncated,
	}
}
