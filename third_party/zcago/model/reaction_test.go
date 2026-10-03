package model

import (
	"encoding/json"
	"testing"
)

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
