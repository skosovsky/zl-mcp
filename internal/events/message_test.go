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
