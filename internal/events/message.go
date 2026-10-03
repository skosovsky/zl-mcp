// Package events implements the MCP Events application contract independently of
// collector and deployment infrastructure.
package events

import (
	"encoding/json"
	"errors"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

const MessageCreated = domain.LegacyMessageCreated
const ConversationMessageCreated = domain.ConversationMessageCreated

type MessageData struct {
	GroupID         string    `json:"group_id"`
	MessageID       string    `json:"message_id"`
	SenderID        string    `json:"sender_id"`
	SenderName      *string   `json:"sender_name"`
	SentAt          time.Time `json:"sent_at"`
	Text            string    `json:"text"`
	TextTruncated   bool      `json:"text_truncated"`
	TextResourceURI *string   `json:"text_resource_uri"`
}

type MessageEvent struct {
	EventID   string      `json:"eventId"`
	Name      string      `json:"name"`
	Timestamp time.Time   `json:"timestamp"`
	Data      MessageData `json:"data"`
	Cursor    *string     `json:"cursor"`
}

type MessageEncoder struct {
	schema             *jsonschema.Schema
	conversationSchema *jsonschema.Schema
	textLimit          int
}

func NewMessageEncoder() (*MessageEncoder, error) {
	doc, err := contracts.Document("zalo_message_created", "payload")
	if err != nil {
		return nil, err
	}
	properties := doc["properties"].(map[string]any)
	text := properties["text"].(map[string]any)
	schema, err := contracts.Compile("mcp_event", "delivery")
	if err != nil {
		return nil, err
	}
	conversationSchema, err := contracts.Compile("conversation_event", "delivery")
	if err != nil {
		return nil, err
	}
	return &MessageEncoder{schema: schema, conversationSchema: conversationSchema, textLimit: int(text["maxLength"].(float64))}, nil
}

// Encode uses the journal's stable ID. The returned bytes are persisted and signed
// unchanged across delivery attempts; signing time is separate from Timestamp.
func (e *MessageEncoder) Encode(eventID string, message domain.Message) ([]byte, error) {
	if !utf8.ValidString(message.Text) {
		return nil, errors.New("event message text is not valid UTF-8")
	}
	if message.Ref().Type != domain.ConversationGroup {
		return nil, errors.New("legacy event requires group message")
	}
	message.GroupID = message.Ref().ID
	data := MessageData{GroupID: message.GroupID, MessageID: message.ID, SenderID: message.SenderID, SenderName: message.SenderName, SentAt: message.SentAt.UTC(), Text: message.Text}
	runes := []rune(message.Text)
	if len(runes) > e.textLimit {
		data.Text = string(runes[:e.textLimit])
		data.TextTruncated = true
		uri := "zalo://groups/" + url.PathEscape(message.GroupID) + "/messages/" + url.PathEscape(message.ID)
		data.TextResourceURI = &uri
	}
	wire, err := json.Marshal(MessageEvent{EventID: eventID, Name: MessageCreated, Timestamp: message.SentAt.UTC(), Data: data})
	if err != nil || len(wire) > 256<<10 {
		return nil, errors.New("event exceeds delivery body limit")
	}
	var value any
	if json.Unmarshal(wire, &value) != nil || e.schema.Validate(value) != nil {
		return nil, errors.New("message does not satisfy event contract")
	}
	return wire, nil
}

func (e *MessageEncoder) EncodeProfile(profile, id string, m domain.Message) ([]byte, error) {
	if profile == MessageCreated {
		return e.Encode(id, m)
	}
	if profile != ConversationMessageCreated || !m.Ref().Valid() || !utf8.ValidString(m.Text) {
		return nil, errors.New("invalid conversation event")
	}
	text := m.Text
	runes := []rune(text)
	truncated := len(runes) > e.textLimit
	var uri *string
	if truncated {
		text = string(runes[:e.textLimit])
		v := "zalo://conversations/" + m.Ref().Type + "/" + url.PathEscape(m.Ref().ID) + "/messages/" + url.PathEscape(m.ID)
		uri = &v
	}
	data := map[string]any{"schema_version": 1, "conversation_type": m.Ref().Type, "conversation_id": m.Ref().ID, "conversation_name": m.ConversationName, "message_id": m.ID, "sender_id": m.SenderID, "sender_name": m.SenderName, "sent_at": m.SentAt.UTC(), "text": text, "text_truncated": truncated, "text_resource_uri": uri}
	body, err := json.Marshal(map[string]any{"eventId": id, "name": profile, "timestamp": m.SentAt.UTC(), "data": data, "cursor": nil})
	if err != nil || len(body) > 256<<10 {
		return nil, errors.New("event exceeds delivery body limit")
	}
	var value any
	if json.Unmarshal(body, &value) != nil || e.conversationSchema.Validate(value) != nil {
		return nil, errors.New("conversation message does not satisfy event contract")
	}
	return body, nil
}
