package events

import (
	"encoding/json"
	"testing"
)

func TestReplayContinuationMetadataPreservesExactIDsAndTriState(t *testing.T) {
	for _, tc := range []struct {
		body  string
		more  *bool
		id    string
		valid bool
	}{
		{`{}`, nil, "", true},
		{`{"more":1,"lastActionId":90071992547409931234}`, boolPointer(true), "90071992547409931234", true},
		{`{"more":false,"lastActionId":"00012"}`, boolPointer(false), "00012", true},
		{`{"more":0}`, boolPointer(false), "", true},
		{`{"more":true}`, boolPointer(true), "", true},
		{`{"more":"false"}`, nil, "", false},
		{`{"more":2}`, nil, "", false},
		{`{"more":1,"lastActionId":1e3}`, boolPointer(true), "", false},
		{`{"more":1,"lastActionId":-1}`, boolPointer(true), "", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			// Arrange
			var d OldMessagesEventData
			if err := json.Unmarshal([]byte(tc.body), &d); err != nil {
				t.Fatal(err)
			}
			// Act
			more, id, valid := d.ContinuationMetadata()
			// Assert
			if (more == nil) != (tc.more == nil) || (more != nil && *more != *tc.more) || id != tc.id || valid != tc.valid {
				t.Fatal(more, id, valid)
			}
		})
	}
}
func boolPointer(v bool) *bool { return &v }
