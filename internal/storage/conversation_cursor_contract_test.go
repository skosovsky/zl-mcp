package storage

import (
	"encoding/json"
	"testing"
)

func TestConversationCursorWireBytes(t *testing.T) {
	// Arrange: bytes signed by EncodeCursor must retain the pre-refactor field order and names.
	cursor := conversationCursor{Fingerprint: "synthetic-digest", Kind: "direct", ID: "9007199254740993-чат"}
	want := `{"Fingerprint":"synthetic-digest","Kind":"direct","ID":"9007199254740993-чат"}`

	// Act.
	raw, err := json.Marshal(cursor)

	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("cursor wire changed: %s", raw)
	}
}
