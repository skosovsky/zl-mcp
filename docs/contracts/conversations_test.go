package contracts

import (
	"encoding/base64"
	"testing"
)

func TestConversationContractsRejectAmbiguousScope(t *testing.T) {
	// Arrange
	cases := []struct {
		name, suffix string
		good, bad    map[string]any
	}{
		{"conversation_ref", "input", map[string]any{"conversation_type": "direct", "conversation_id": "same"}, map[string]any{"conversation_id": "same"}},
		{"collection_policy", "input", map[string]any{"mode": "all"}, map[string]any{"mode": "all", "conversations": []any{}}},
		{"collection_policy", "input", map[string]any{"mode": "selected", "conversations": []any{map[string]any{"type": "direct", "id": "peer"}}}, map[string]any{"mode": "selected"}},
	}
	for _, tc := range cases {
		s, err := Compile(tc.name, tc.suffix)
		if err != nil {
			t.Fatal(err)
		}
		// Act / Assert
		if err = s.Validate(tc.good); err != nil {
			t.Fatal(err)
		}
		if s.Validate(tc.bad) == nil {
			t.Fatal("ambiguous or invalid policy accepted")
		}
	}
	s, err := Compile("conversation_v2_events_subscribe", "input")
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic zero bytes, constructed at runtime; never a receiver credential.
	secret := "whsec_" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	base := map[string]any{"name": "zalo.conversation.message.created.v2", "arguments": map[string]any{"scope": "all"}, "delivery": map[string]any{"mode": "webhook", "url": "https://receiver.example/callback", "secret": secret}}
	// Act / Assert
	if err = s.Validate(base); err != nil {
		t.Fatal(err)
	}
	base["arguments"] = map[string]any{"scope": "all", "conversation_id": "peer", "conversation_type": "direct"}
	if s.Validate(base) == nil {
		t.Fatal("wide scope accepted contradictory peer filter")
	}
	base["arguments"] = map[string]any{"scope": "conversation", "conversation_id": "peer"}
	if s.Validate(base) == nil {
		t.Fatal("untyped peer scope accepted")
	}
}

func TestConversationEventPayloadHasTypedIdentity(t *testing.T) {
	// Arrange
	s, err := Compile("conversation_v2_message_created", "payload")
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"schema_version": 2, "conversation_type": "direct", "conversation_id": "peer", "conversation_name": nil, "message_id": "m", "sender_id": "peer", "sender_name": nil, "sent_at": "2026-10-03T08:13:00Z", "text": "synthetic", "text_truncated": false, "text_resource_uri": nil, "direction": "incoming", "first_incoming": nil}
	// Act / Assert
	if err = s.Validate(payload); err != nil {
		t.Fatal(err)
	}
	delete(payload, "conversation_type")
	if s.Validate(payload) == nil {
		t.Fatal("untyped event accepted")
	}
}

func TestGeneralSearchRequiresTypeOnlyForConcreteConversation(t *testing.T) {
	// Arrange
	s, err := Compile("zalo_search_conversation_messages", "input")
	if err != nil {
		t.Fatal(err)
	}
	// Act / Assert
	for _, args := range []map[string]any{{"query": "synthetic"}, {"query": "synthetic", "conversation_type": "direct"}, {"query": "synthetic", "conversation_type": "group", "conversation_id": "same"}} {
		if err = s.Validate(args); err != nil {
			t.Fatal(err)
		}
	}
	if s.Validate(map[string]any{"query": "synthetic", "conversation_id": "same"}) == nil {
		t.Fatal("untyped conversation accepted")
	}
	if s.Validate(map[string]any{"query": "synthetic", "group_id": "same"}) == nil {
		t.Fatal("legacy filter accepted by general API")
	}
}
