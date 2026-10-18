package contracts

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMessageActionInputsRejectAmbiguousOrProtocolArguments(t *testing.T) {
	id := `"request_id":"a0000000-0000-4000-8000-000000000001","conversation_type":"direct","conversation_id":"peer"`
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"zalo_mark_messages_read", `{` + id + `,"message_ids":["123"]}`, true},
		{"zalo_mark_messages_read", `{` + id + `,"message_ids":[]}`, false},
		{"zalo_mark_messages_read", `{` + id + `,"message_ids":["123","123"]}`, false},
		{"zalo_mark_messages_read", `{` + id + `,"message_ids":["123"],"st":0}`, false},
		{"zalo_set_message_reaction", `{` + id + `,"message_id":"123","action":"set","reaction":"heart"}`, true},
		{"zalo_set_message_reaction", `{` + id + `,"message_id":"123","action":"remove"}`, true},
		{"zalo_set_message_reaction", `{` + id + `,"message_id":"123","action":"remove","reaction":""}`, false},
		{"zalo_set_message_reaction", `{` + id + `,"message_id":"123","action":"set"}`, false},
		{"zalo_set_message_reaction", `{` + id + `,"message_id":"123","action":"set","reaction":"❤️"}`, false},
		{"zalo_set_message_reaction", `{` + id + `,"message_id":"123","action":"set","reaction":"heart","plan":"add"}`, false},
	} {
		// Arrange: actual embedded schema, no application fallback.
		schema, err := Compile(tc.name, "input")
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err = json.Unmarshal([]byte(tc.body), &value); err != nil {
			t.Fatal(err)
		}
		// Act / Assert.
		if valid := schema.Validate(value) == nil; valid != tc.valid {
			t.Fatalf("%s validity=%v for %s", tc.name, valid, tc.body)
		}
	}
}

func TestMessageActionOutputRejectsFalseCompletedPlans(t *testing.T) {
	for _, tc := range []struct {
		state, plan string
		steps       int
		reason      any
		valid       bool
	}{
		{"accepted", "clear_add", 2, nil, true},
		{"accepted", "clear_add", 1, nil, false},
		{"accepted", "clear", 1, nil, true},
		{"accepted", "clear", 0, nil, false},
		{"unknown", "clear_add", 1, "upstream_ambiguous", true},
		{"unknown", "clear_add", 1, nil, false},
		{"partial", "clear_add", 1, "upstream_rejected", true},
		{"partial", "clear_add", 0, "upstream_rejected", false},
		{"partial", "clear", 1, "upstream_rejected", false},
	} {
		// Arrange: same shared output shape on both writes and status.
		value := map[string]any{
			"request_id":      "a0000000-0000-4000-8000-000000000001",
			"kind":            "reaction",
			"plan":            tc.plan,
			"conversation":    map[string]any{"conversation_type": "direct", "conversation_id": "peer"},
			"status":          tc.state,
			"stage":           "none",
			"completed_steps": tc.steps,
			"reason":          tc.reason,
			"upstream_status": nil,
			"updated_at":      "2026-10-17T00:00:00Z",
			"retry_safe":      false,
		}
		for _, name := range []string{"zalo_mark_messages_read", "zalo_set_message_reaction", "zalo_get_message_action_status"} {
			schema, err := Compile(name, "output")
			if err != nil {
				t.Fatal(err)
			}
			// Act / Assert: a clear-only result cannot masquerade as completed replacement.
			if valid := schema.Validate(value) == nil; valid != tc.valid {
				t.Fatal(name, tc, valid)
			}
		}
	}
}

func TestConcurrentContractConsumersKeepFormatsAndOwnedDocuments(t *testing.T) {
	// Arrange: every consumer receives the embedded contract, never a mutable caller map.
	document, err := Document("zalo_mark_messages_read", "input")
	if err != nil {
		t.Fatal(err)
	}
	document["additionalProperties"] = true
	done := make(chan error, 8)
	for range 8 {
		go func() {
			schema, compileErr := Compile("zalo_mark_messages_read", "input")
			if compileErr != nil {
				done <- compileErr
				return
			}
			valid := map[string]any{
				"request_id":        "a0000000-0000-4000-8000-000000000001",
				"conversation_type": "direct",
				"conversation_id":   "peer",
				"message_ids":       []any{"123"},
			}
			// Act: independently validate a real request, bad UUID and forbidden native field.
			if validateErr := schema.Validate(valid); validateErr != nil {
				done <- validateErr
				return
			}
			valid["request_id"] = "invalid"
			if schema.Validate(valid) == nil {
				done <- errors.New("UUID format was lost")
				return
			}
			valid["request_id"] = "a0000000-0000-4000-8000-000000000001"
			valid["st"] = 0
			if schema.Validate(valid) == nil {
				done <- errors.New("caller document altered contract")
				return
			}
			done <- nil
		}()
	}
	// Assert: concurrent validators preserve strict semantics and race safety.
	for range 8 {
		if err = <-done; err != nil {
			t.Fatal(err)
		}
	}
}
