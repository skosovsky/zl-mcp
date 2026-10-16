package model

import (
	"encoding/json"
	"testing"
)

func TestReactionContentPreservesNativeTypeAndSource(t *testing.T) {
	// Arrange: native fields are unprefixed, including nonzero enum values.
	payload := []byte(`{"rMsg":[{"gMsgID":"123","cMsgID":"456","msgType":1}],"rIcon":"/-heart","rType":5,"source":6}`)
	var got ReactionContent
	// Act.
	err := json.Unmarshal(payload, &got)
	encoded, encodeErr := json.Marshal(got)
	var wire map[string]json.RawMessage
	wireErr := json.Unmarshal(encoded, &wire)
	// Assert: round trip preserves fields without inventing action semantics.
	if err != nil || encodeErr != nil || wireErr != nil {
		t.Fatalf("reaction codec failed: %v / %v / %v", err, encodeErr, wireErr)
	}
	if got.RType != 5 || got.Source != 6 || len(got.RMsg) != 1 || got.RMsg[0].GMsgID != 123 {
		t.Fatalf("native fields lost: %#v", got)
	}
	if string(wire["rType"]) != "5" || string(wire["source"]) != "6" || wire["r.RType"] != nil || wire["r.Source"] != nil {
		t.Fatalf("wrong native field names: %s", encoded)
	}
}

func TestReactionContentShapeDoesNotRetainNativeValues(t *testing.T) {
	for _, tc := range []struct {
		payload, references   string
		count, global, client int
	}{
		{`{"private-key":"private-value"}`, "missing", 0, 0, 0},
		{`{"rMsg":null}`, "null", 0, 0, 0},
		{`{"rMsg":[]}`, "array", 0, 0, 0},
		{`{"rMsg":[{"cMsgID":"456"}]}`, "array", 1, 0, 1},
		{`{"rMsg":[{"gMsgID":"123","cMsgID":0}]}`, "array", 1, 1, 0},
	} {
		// Arrange: reuse clears diagnostics from the previous native record.
		got := ReactionContent{Shape: ReactionContentShape{Count: 99}}
		// Act.
		err := json.Unmarshal([]byte(tc.payload), &got)
		encoded, encodeErr := json.Marshal(got)
		// Assert: distinguish missing targets from client-only refs without raw retention.
		if err != nil || encodeErr != nil || got.Shape.References != tc.references || got.Shape.Count != tc.count || got.Shape.PositiveGlobal != tc.global || got.Shape.PositiveClient != tc.client {
			t.Fatalf("wrong shape: %#v / %v / %v", got.Shape, err, encodeErr)
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &wire); err != nil || wire["Shape"] != nil || wire["private-key"] != nil {
			t.Fatal("diagnostics or unknown fields leaked into wire content")
		}
	}
}

func TestReactionMessageIDsAcceptNumbersAndStrings(t *testing.T) {
	for _, payload := range []string{
		`{"gMsgID":9007199254740993,"cMsgID":9007199254740995,"msgType":1}`,
		`{"gMsgID":"9007199254740993","cMsgID":"9007199254740995","msgType":1}`,
	} {
		// Arrange: synthetic identifiers exceed float64's exact integer range.
		var got ReactionMessageRef
		// Act.
		err := json.Unmarshal([]byte(payload), &got)
		// Assert.
		if err != nil || got.GMsgID != 9007199254740993 || got.CMsgID != 9007199254740995 || got.MsgType != 1 {
			t.Fatalf("identifier precision lost: %#v, %v", got, err)
		}
	}
}

func TestReactionMessageIDsRejectInvalidValues(t *testing.T) {
	for _, value := range []string{`"not-an-id"`, `1.5`, `true`, `{}`, `"9223372036854775808"`} {
		// Arrange.
		var got ReactionMessageRef
		payload := []byte(`{"gMsgID":` + value + `,"cMsgID":1}`)
		// Act.
		err := json.Unmarshal(payload, &got)
		// Assert: do not truncate, round or silently replace invalid identifiers.
		if err == nil {
			t.Fatalf("accepted invalid identifier %s", value)
		}
	}
}

func TestReactionIdentityKeepsOutgoingBeforeUIDReplacement(t *testing.T) {
	// Arrange: the native self sentinel must be inspected before normalization.
	for _, tc := range []struct {
		name, from, to, peer string
		kind                 ThreadType
		self                 bool
	}{
		{"outgoing direct", "0", "peer", "peer", ThreadTypeUser, true},
		{"incoming direct", "peer", "0", "peer", ThreadTypeUser, false},
		{"outgoing group", "0", "group", "group", ThreadTypeGroup, true},
		{"incoming group", "peer", "group", "group", ThreadTypeGroup, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			got := NewReaction("owner", TReaction{UIDFrom: tc.from, IDTo: tc.to}, tc.kind)
			// Assert: self direction and conversation namespace remain independent.
			if got.IsSelf != tc.self || got.ThreadID != tc.peer {
				t.Fatal("reaction identity or direction changed", got.IsSelf, got.ThreadID)
			}
			if tc.self && got.Data.UIDFrom != "owner" {
				t.Fatal("self author not normalized")
			}
		})
	}
}
