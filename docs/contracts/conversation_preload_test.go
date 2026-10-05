package contracts

import (
	"encoding/json"
	"testing"
)

func TestConversationPreloadRawContract(t *testing.T) {
	// Arrange: this is source evidence, not a newly advertised MCP tool.
	schema, err := Compile("conversation_preload_page", "output")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"clearUnreads":[],"msgs":[]}`, true},
		{`{"clearUnreads":[{"idTo":"9007199254740993","isGroup":0}],"msgs":null}`, true},
		{`{}`, false}, {`{"clearUnreads":null}`, false}, {`{"clearUnreads":[null]}`, false},
		{`{"clearUnreads":[],"msgs":[[]]}`, false},
	} {
		// Act
		var value any
		if err := json.Unmarshal([]byte(tc.body), &value); err != nil {
			t.Fatal(err)
		}
		err := schema.Validate(value)
		// Assert
		if (err == nil) != tc.valid {
			t.Fatalf("unexpected preload validation result: %v", err)
		}
	}
	for _, name := range Names() {
		if name == "conversation_preload_page" {
			t.Fatal("source schema advertised as tool")
		}
	}
}
