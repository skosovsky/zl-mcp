package api

import (
	"encoding/json"
	"testing"
)

func TestSendMessageResultPreservesAcknowledgementIDs(t *testing.T) {
	for _, tc := range []struct{ payload, want string }{
		{`{"msgId":"9007199254740993"}`, "9007199254740993"},
		{`{"msgId":9007199254740993}`, "9007199254740993"},
		{`{"msgId":1234567890123456789012345678901234567890}`, "1234567890123456789012345678901234567890"},
		{`{"msgId":"accepted"}`, "accepted"},
		{`{"msgId":""}`, ""},
		{`{"msgId":null}`, ""},
		{`{}`, ""},
	} {
		// Arrange: prefill to ensure a missing acknowledgement cannot reuse an ID.
		result := SendMessageResult{MsgID: "previous"}
		// Act.
		err := json.Unmarshal([]byte(tc.payload), &result)
		// Assert: exact digits and missing acknowledgement semantics are retained.
		if err != nil || result.MsgID != tc.want {
			t.Fatalf("incorrect acknowledgement: %v", err)
		}
	}
}

func TestSendMessageResultRejectsNonintegerAcknowledgements(t *testing.T) {
	for _, value := range []string{`true`, `{}`, `[]`, `1.5`, `1e3`, `-1`} {
		// Arrange.
		var result SendMessageResult
		// Act.
		err := json.Unmarshal([]byte(`{"msgId":`+value+`}`), &result)
		// Assert: do not round, truncate or manufacture confirmation.
		if err == nil || result.MsgID != "" {
			t.Fatal("accepted invalid acknowledgement")
		}
	}
}
