package events

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestMessageEventPreservesIDsAndProvidesFullTextReference(t *testing.T) {
	// Arrange: non-ASCII text and identifiers containing reserved URI characters.
	encoder, err := NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{2048, 2049} {
		message := domain.Message{GroupID: "group/one", ID: "message?#two", SenderID: "author", SentAt: time.Date(2026, 10, 2, 10, 0, 0, 0, time.FixedZone("local", 3*3600)), Text: strings.Repeat("界", size)}
		// Act.
		wire, err := encoder.Encode("stable-event-id", message)
		if err != nil {
			t.Fatal(err)
		}
		var event MessageEvent
		if err := json.Unmarshal(wire, &event); err != nil {
			t.Fatal(err)
		}
		retryWire, err := encoder.Encode("stable-event-id", message)
		// Assert: original record remains intact, wire is stable and UTC.
		if err != nil || !bytes.Equal(wire, retryWire) || event.Data.GroupID != message.GroupID || event.Data.MessageID != message.ID || len([]rune(message.Text)) != size || event.Data.SentAt.Location() != time.UTC || !utf8.ValidString(event.Data.Text) {
			t.Fatal("event altered identity, source text, timestamp or retry payload")
		}
		if event.Data.TextTruncated != (size > 2048) || len([]rune(event.Data.Text)) != 2048 {
			t.Fatal("incorrect truncation boundary")
		}
		if size > 2048 {
			if event.Data.TextResourceURI == nil {
				t.Fatal("missing full text reference")
			}
			uri, err := url.Parse(*event.Data.TextResourceURI)
			if err != nil || uri.Fragment != "" || uri.RawQuery != "" || uri.EscapedPath() != "/group%2Fone/messages/message%3F%23two" {
				t.Fatalf("unsafe resource reference: %v", uri)
			}
		} else if event.Data.TextResourceURI != nil {
			t.Fatal("unexpected resource reference")
		}
	}
}

func TestMessageEventRejectsInvalidUTF8(t *testing.T) {
	// Arrange.
	encoder, err := NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	message := domain.Message{GroupID: "g", ID: "m", SenderID: "s", SentAt: time.Now(), Text: string([]byte{0xff})}
	// Act.
	_, err = encoder.Encode("id", message)
	// Assert: JSON encoding must not silently replace source bytes.
	if err == nil {
		t.Fatal("invalid text was silently normalized")
	}
}

func TestConversationEventUnicodeBoundaryAndStableTypedURI(t *testing.T) {
	// Arrange: both namespaces with IDs exceeding JSON numeric precision and
	// reserved URI characters, covering the exact executable text boundary.
	encoder, err := NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"direct", "group"} {
		for _, size := range []int{2048, 2049} {
			m := domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: "9007199254740993/peer"}, ID: "9007199254740995?#message", SenderID: "owner", SentAt: time.Now().UTC(), Text: strings.Repeat("界", size)}
			// Act.
			wire, err := encoder.EncodeProfile(ConversationMessageCreated, "stable", m)
			if err != nil {
				t.Fatal(err)
			}
			retry, err := encoder.EncodeProfile(ConversationMessageCreated, "stable", m)
			var event struct {
				Data struct {
					Type      string  `json:"conversation_type"`
					ID        string  `json:"conversation_id"`
					MessageID string  `json:"message_id"`
					Text      string  `json:"text"`
					Truncated bool    `json:"text_truncated"`
					URI       *string `json:"text_resource_uri"`
				} `json:"data"`
			}
			if err := json.Unmarshal(wire, &event); err != nil {
				t.Fatal(err)
			}
			// Assert.
			if err != nil || !bytes.Equal(wire, retry) || event.Data.Type != kind || event.Data.ID != m.Ref().ID || event.Data.MessageID != m.ID || len([]rune(event.Data.Text)) != 2048 || event.Data.Truncated != (size > 2048) {
				t.Fatal("typed payload altered identity, boundary or retry bytes")
			}
			if size == 2048 {
				if event.Data.URI != nil {
					t.Fatal("unexpected truncation URI")
				}
				continue
			}
			if event.Data.URI == nil {
				t.Fatal("missing URI")
			}
			u, err := url.Parse(*event.Data.URI)
			if err != nil || u.RawQuery != "" || u.Fragment != "" || u.EscapedPath() != "/"+kind+"/9007199254740993%2Fpeer/messages/9007199254740995%3F%23message" {
				t.Fatal("unsafe typed URI")
			}
		}
	}
}
