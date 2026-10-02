package contracts

import (
	"strings"
	"testing"
)

func TestEventsSchemasCompile(t *testing.T) {
	// Arrange.
	names := []struct{ name, suffix string }{
		{"events_list", "input"}, {"events_list", "output"},
		{"events_subscribe", "input"}, {"events_subscribe", "output"},
		{"events_unsubscribe", "input"}, {"events_unsubscribe", "output"},
		{"zalo_message_created", "payload"}, {"mcp_event", "delivery"},
	}
	for _, tc := range names {
		t.Run(tc.name+"."+tc.suffix, func(t *testing.T) {
			// Act.
			_, err := Compile(tc.name, tc.suffix)
			// Assert.
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEventsPayloadBoundsAndFullTextReference(t *testing.T) {
	// Arrange.
	schema, err := Compile("zalo_message_created", "payload")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		text      string
		truncated bool
		uri       any
		valid     bool
	}{
		{"unicode-boundary", strings.Repeat("界", 2048), false, nil, true},
		{"over-limit", strings.Repeat("界", 2049), false, nil, false},
		{"truncated-with-reference", "prefix", true, "zalo://groups/group/messages/message", true},
		{"missing-reference", "prefix", true, nil, false},
		{"wrong-reference", "prefix", true, "https://example.com", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"group_id": "group", "message_id": "message", "sender_id": "author", "sender_name": nil, "sent_at": "2026-10-02T00:00:00Z", "text": tc.text, "text_truncated": tc.truncated, "text_resource_uri": tc.uri}
			// Act.
			err := schema.Validate(payload)
			// Assert.
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestEventsInputsRejectProtocolAndFilterAmbiguity(t *testing.T) {
	// Arrange.
	schema, err := Compile("events_subscribe", "input")
	if err != nil {
		t.Fatal(err)
	}
	for _, ttl := range []any{nil, float64(60000), float64(59999), "forever"} {
		args := map[string]any{"name": "zalo.message.created", "arguments": map[string]any{"group_id": "group"}, "delivery": map[string]any{"mode": "webhook", "url": "https://callback.example/events", "secret": "whsec_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, "ttlMs": ttl, "cursor": nil}
		// Act.
		err := schema.Validate(args)
		// Assert.
		valid := ttl == nil || ttl == float64(60000)
		if (err == nil) != valid {
			t.Fatalf("ttl=%v error=%v", ttl, err)
		}
		// Act: arbitrary application fields must be rejected.
		args["principal"] = "other-account"
		err = schema.Validate(args)
		// Assert.
		if err == nil {
			t.Fatal("caller-supplied principal accepted")
		}
	}
}
