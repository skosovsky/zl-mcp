package eventaccess

import (
	"context"
	"encoding/json"
)

// Subscription represents either retained event profile explicitly. Optional
// profile fields preserve legacy omission; timestamps retain their wire spelling.
type Subscription struct {
	SubscriptionID    string  `json:"subscription_id"`
	EventName         string  `json:"event_name"`
	Scope             string  `json:"scope"`
	ConversationType  *string `json:"conversation_type"`
	ConversationID    *string `json:"conversation_id"`
	Direction         string  `json:"direction"`
	FirstIncomingOnly bool    `json:"first_incoming_only"`
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
	if d.SchemaVersion != nil {
		return json.Marshal(struct {
			Message

			ConversationID   *string `json:"conversation_id"`
			ConversationType *string `json:"conversation_type"`
			ConversationName *string `json:"conversation_name"`
			SchemaVersion    *int    `json:"schema_version"`
			Direction        *string `json:"direction"`
			FirstIncoming    *bool   `json:"first_incoming"`
		}{d.Message, d.ConversationID, d.ConversationType, d.ConversationName, d.SchemaVersion, d.Direction, d.FirstIncoming})
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
	Event         *Event  `json:"event"`
	DeliveryState string  `json:"delivery_state"`
	GapReason     *string `json:"gap_reason"`
	Receipt       string  `json:"receipt"`
}
type Page struct {
	SubscriptionID    string  `json:"subscription_id"`
	Events            []Entry `json:"events"`
	HasMore           bool    `json:"has_more"`
	BlockedOnDelivery bool    `json:"blocked_on_delivery"`
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
