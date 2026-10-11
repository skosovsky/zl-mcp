package eventaccess

import (
	"encoding/json"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"reflect"
	"testing"
)

func TestJournalProfileProjectionPreservesNullsAndOpaqueValues(t *testing.T) {
	for _, data := range []string{
		`{"group_id":"g","message_id":"9007199254740993","sender_id":"p","sender_name":null,"sent_at":"2026-10-01T00:00:00.000000000Z","text":"Cà phê 🍀","text_truncated":false,"text_resource_uri":null}`,
		`{"schema_version":2,"conversation_id":"9007199254740993","conversation_type":"direct","conversation_name":null,"direction":"unknown","first_incoming":null,"message_id":"m","sender_id":"p","sender_name":null,"sent_at":"2026-10-01T00:00:00.000000000Z","text":"Cà phê 🍀","text_truncated":false,"text_resource_uri":null}`,
		`{"schema_version":2,"conversation_id":"d","conversation_type":"direct","conversation_name":"","direction":"incoming","first_incoming":false,"message_id":"m","sender_id":"p","sender_name":"","sent_at":"2026-10-01T00:00:00Z","text":"Cà phê 🍀","text_truncated":false,"text_resource_uri":null}`,
	} {
		t.Run(data, func(t *testing.T) {
			// Arrange: each supported profile has its own required nullable fields.
			name := "zalo.message.created"
			var original map[string]any
			if err := json.Unmarshal([]byte(data), &original); err != nil {
				t.Fatal(err)
			}
			if original["schema_version"] != nil {
				name = "zalo.conversation.message.created.v2"
			}
			wire := `{"subscription_id":"s","events":[{"event":{"eventId":"opaque","name":"` + name + `","timestamp":"2026-10-01T00:00:00Z","data":` + data + `,"cursor":null},"delivery_state":"delivered","gap_reason":null,"receipt":"receipt"}],"has_more":false,"blocked_on_delivery":false}`
			var page Page
			// Act: the adapter's typed projection roundtrip.
			if err := json.Unmarshal([]byte(wire), &page); err != nil {
				t.Fatal(err)
			}
			projected, err := json.Marshal(page)
			if err != nil {
				t.Fatal(err)
			}
			var before, after any
			if err = json.Unmarshal([]byte(wire), &before); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(projected, &after); err != nil {
				t.Fatal(err)
			}
			// Assert: fields, nulls, false, timestamp spelling and string IDs survive.
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("projection drift: %s", projected)
			}
			schema, err := contracts.Compile("zalo_read_subscription_events", "output")
			if err != nil {
				t.Fatal(err)
			}
			if err = schema.Validate(after); err != nil {
				t.Fatal(err)
			}
		})
	}
}
