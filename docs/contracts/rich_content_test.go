package contracts

import "testing"

func TestRichContentContractsCompileAndRejectAmbiguity(t *testing.T) {
	// Arrange.
	for _, tc := range []struct{ name, suffix string }{
		{"message_content", "output"}, {"conversation_v3_message_created", "payload"},
		{"conversation_v3_event", "delivery"}, {"conversation_v3_events_subscribe", "input"}, {"conversation_v3_events_unsubscribe", "input"},
		{"reaction_v1", "payload"}, {"reaction_v1_event", "delivery"}, {"reaction_v1_events_subscribe", "input"}, {"reaction_v1_events_unsubscribe", "input"},
		{"call_v1", "payload"}, {"call_v1_event", "delivery"}, {"call_v1_events_subscribe", "input"}, {"call_v1_events_unsubscribe", "input"},
	} {
		// Act.
		_, err := Compile(tc.name, tc.suffix)
		// Assert.
		if err != nil {
			t.Fatal(tc.name, err)
		}
	}
	schema, err := Compile("message_content", "output")
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{
		"text_role":    "none",
		"kind":         "unsupported",
		"native_type":  nil,
		"summary":      "Unavailable content",
		"availability": "unavailable",
		"reason":       "legacy_projection",
		"elements":     []any{},
	}
	if err := schema.Validate(value); err != nil {
		t.Fatal(err)
	}
	// Act: raw wire payload injection must not extend the closed contract.
	value["params"] = "private payload"
	// Assert.
	if schema.Validate(value) == nil {
		t.Fatal("raw metadata accepted")
	}
}

func TestActivitySelectorsRejectMessageNoveltyFilter(t *testing.T) {
	// Arrange: activity families never inherit message first-incoming semantics.
	for _, family := range []string{"call", "reaction"} {
		schema, err := Compile(family+"_v1_events_unsubscribe", "input")
		if err != nil {
			t.Fatal(err)
		}
		args := map[string]any{"scope": "direct", "direction": "incoming"}
		value := map[string]any{
			"name":      "zalo.conversation." + family + ".observed.v1",
			"arguments": args,
			"delivery":  map[string]any{"mode": "webhook", "url": "https://receiver.example/events"},
		}
		// Act / Assert: normal selectors work, invented message novelty is rejected.
		if err := schema.Validate(value); err != nil {
			t.Fatal(err)
		}
		args["first_incoming_only"] = false
		if schema.Validate(value) == nil {
			t.Fatal("activity silently accepted message-only novelty filter")
		}
	}
}
