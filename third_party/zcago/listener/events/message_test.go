package events

import (
	"encoding/json"
	"testing"
)

func TestDirectDecodeRejectsInvalidAndClearsReusedValue(t *testing.T) {
	// Arrange
	valid := []byte(`{"msgId":"m","uidFrom":"u","idTo":"0","ts":"1791029400000","content":"synthetic"}`)
	var item messageOrUndo
	if err := json.Unmarshal(valid, &item); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{`{"msgId":"m","ts":1791029400000}`, `{"msgId":"m","quote":{"cliMsgId":"invalid"}}`, `{}`} {
		// Act
		err := json.Unmarshal([]byte(payload), &item)
		// Assert
		if err == nil || item.Message != nil || item.Undo != nil {
			t.Fatal("invalid message silently accepted or stale value retained")
		}
	}
}

func TestDirectDecodeUndoClearsPreviousMessage(t *testing.T) {
	// Arrange
	var item messageOrUndo
	if err := json.Unmarshal([]byte(`{"msgId":"m","content":"synthetic"}`), &item); err != nil {
		t.Fatal(err)
	}
	// Act
	err := json.Unmarshal([]byte(`{"msgId":"m","msgType":"chat.undo"}`), &item)
	// Assert
	if err != nil || item.Undo == nil || item.Message != nil {
		t.Fatal("undo did not replace previous message")
	}
}
