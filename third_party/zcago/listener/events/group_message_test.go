package events

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestGroupMessageDecodeDoesNotHideErrors(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		wantError     bool
	}{
		{"valid", `{"groupMsgs":[{"msgId":"m","uidFrom":"u","idTo":"g","ts":"1791029400000","content":"synthetic"}]}`, false},
		{"numeric_timestamp", `{"groupMsgs":[{"msgId":"m","ts":1791029400000}]}`, true},
		{"invalid_quote", `{"groupMsgs":[{"msgId":"m","quote":{"cliMsgId":"invalid"}}]}`, true},
		{"missing_id", `{"groupMsgs":[{"ts":"1791029400000"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var event GroupMessageEventData
			// Act
			err := json.Unmarshal([]byte(tc.payload), &event)
			// Assert
			if (err != nil) != tc.wantError {
				t.Fatalf("error presence mismatch: %T", err)
			}
			if !tc.wantError && event.GroupMsgs[0].Message == nil {
				t.Fatal("valid message missing")
			}
			if tc.name == "numeric_timestamp" {
				var typed *json.UnmarshalTypeError
				if !errors.As(err, &typed) {
					t.Fatal("decode type lost")
				}
			}
		})
	}
}
