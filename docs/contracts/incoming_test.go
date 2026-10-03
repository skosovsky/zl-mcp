package contracts

import "testing"

func TestFilteredSubscriptionContractRejectsContradictions(t *testing.T) {
	// Arrange.
	s, err := Compile("conversation_v2_events_subscribe", "input")
	if err != nil {
		t.Fatal(err)
	}
	delivery := map[string]any{"mode": "webhook", "url": "https://receiver.example/events", "secret": "whsec_" + "synthetic"}
	for _, tc := range []struct {
		args  map[string]any
		valid bool
	}{
		{map[string]any{"scope": "direct", "direction": "incoming", "first_incoming_only": true}, true},
		{map[string]any{"scope": "all"}, true},
		{map[string]any{"scope": "group", "direction": "incoming", "first_incoming_only": true}, false},
		{map[string]any{"scope": "direct", "direction": "outgoing", "first_incoming_only": true}, false},
		{map[string]any{"scope": "direct", "first_incoming_only": true}, false},
	} {
		// Act.
		err = s.Validate(map[string]any{"name": "zalo.conversation.message.created.v2", "arguments": tc.args, "delivery": delivery})
		// Assert.
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%v error=%v", tc.valid, err)
		}
	}
}
