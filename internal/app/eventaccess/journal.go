package eventaccess

import (
	"context"
	"encoding/json"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Subscription represents either retained event profile explicitly. Optional
// profile fields preserve legacy omission; timestamps retain their wire spelling.
type Subscription struct {
	SubscriptionID         string  `json:"subscription_id"`
	EventName              string  `json:"event_name"`
	Scope                  string  `json:"scope"`
	ConversationType       *string `json:"conversation_type"`
	ConversationID         *string `json:"conversation_id"`
	Direction              string  `json:"direction"`
	FirstIncomingOnly      bool    `json:"first_incoming_only"`
	GenerationReference    string  `json:"generation_reference,omitempty"`
	RecoveryMode           string  `json:"recovery_mode,omitempty"`
	RecoverySubscriptionID *string `json:"recovery_subscription_id,omitempty"`
}
type Subscriptions struct {
	Subscriptions []Subscription `json:"subscriptions"`
}
type Message struct {
	MessageID       string  `json:"message_id"`
	SenderID        string  `json:"sender_id"`
	SenderName      *string `json:"sender_name"`
	SentAt          string  `json:"sent_at"`
	Text            string  `json:"text"`
	TextTruncated   bool    `json:"text_truncated"`
	TextResourceURI *string `json:"text_resource_uri"`
}
type EventData struct {
	Message

	Activity *domain.Activity `json:"-"`

	ContentTruncated   *bool                  `json:"content_truncated,omitempty"`
	ContentResourceURI string                 `json:"content_resource_uri,omitempty"`
	Content            *domain.MessageContent `json:"content,omitempty"`

	GroupID          *string `json:"group_id,omitempty"`
	ConversationID   *string `json:"conversation_id,omitempty"`
	ConversationType *string `json:"conversation_type,omitempty"`
	ConversationName *string `json:"conversation_name,omitempty"`
	SchemaVersion    *int    `json:"schema_version,omitempty"`
	Direction        *string `json:"direction,omitempty"`
	FirstIncoming    *bool   `json:"first_incoming,omitempty"`
}

// MarshalJSON preserves v2 nullable fields that the legacy profile does not contain.
func (d EventData) MarshalJSON() ([]byte, error) {
	if d.Activity != nil {
		return json.Marshal(d.Activity)
	}
	if d.SchemaVersion != nil {
		return json.Marshal(struct {
			Message

			ConversationID     *string                `json:"conversation_id"`
			ConversationType   *string                `json:"conversation_type"`
			ConversationName   *string                `json:"conversation_name"`
			SchemaVersion      *int                   `json:"schema_version"`
			Direction          *string                `json:"direction"`
			FirstIncoming      *bool                  `json:"first_incoming"`
			Content            *domain.MessageContent `json:"content,omitempty"`
			ContentResourceURI string                 `json:"content_resource_uri,omitempty"`
			ContentTruncated   *bool                  `json:"content_truncated,omitempty"`
		}{d.Message, d.ConversationID, d.ConversationType, d.ConversationName, d.SchemaVersion, d.Direction, d.FirstIncoming, d.Content, d.ContentResourceURI, d.ContentTruncated})
	}
	return json.Marshal(struct {
		Message

		GroupID *string `json:"group_id"`
	}{d.Message, d.GroupID})
}

type Event struct {
	EventID   string    `json:"eventId"`
	Name      string    `json:"name"`
	Timestamp string    `json:"timestamp"`
	Data      EventData `json:"data"`
	Cursor    *string   `json:"cursor"`
}
type Entry struct {
	Event             *Event  `json:"event"`
	DeliveryState     string  `json:"delivery_state"`
	GapReason         *string `json:"gap_reason"`
	Receipt           string  `json:"receipt"`
	DeliveryReason    string  `json:"delivery_reason,omitempty"`
	DeliveryReference string  `json:"delivery_reference,omitempty"`
}
type Page struct {
	SubscriptionID    string  `json:"subscription_id"`
	Events            []Entry `json:"events"`
	HasMore           bool    `json:"has_more"`
	BlockedOnDelivery bool    `json:"blocked_on_delivery"`
	RetryAfterMS      *int    `json:"retry_after_ms,omitempty"`
	RecoveryMode      string  `json:"recovery_mode,omitempty"`
	RecoveryState     string  `json:"recovery_state,omitempty"`
	RecoveryReference *string `json:"recovery_reference,omitempty"`
	RecoveryReason    *string `json:"recovery_reason,omitempty"`
}
type JournalPort interface {
	Subscriptions(context.Context, string) (Subscriptions, error)
	ReadEvents(context.Context, string, string, int) (Page, error)
}

func (h *Handler) Subscriptions(ctx context.Context, principal string) (Subscriptions, error) {
	return h.Journal.Subscriptions(ctx, principal)
}
func (h *Handler) ReadEvents(ctx context.Context, principal, id string, limit int) (Page, error) {
	return h.Journal.ReadEvents(ctx, principal, id, limit)
}

// UnmarshalJSON distinguishes the closed activity union from the retained
// message profiles. A reaction/call is never projected through Message fields.
func (d *EventData) UnmarshalJSON(raw []byte) error {
	var discriminator struct {
		Family string `json:"family"`
	}
	if err := json.Unmarshal(raw, &discriminator); err != nil {
		return err
	}
	var empty EventData
	*d = empty
	if discriminator.Family != "" {
		var activity domain.Activity
		if err := json.Unmarshal(raw, &activity); err != nil {
			return err
		}
		if err := activity.Validate(); err != nil {
			return err
		}
		d.Activity = &activity
		return nil
	}
	type plainEventData EventData
	return json.Unmarshal(raw, (*plainEventData)(d))
}
