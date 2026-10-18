package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReactionResponseRepresentations(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []int
		invalid    bool
	}{
		{"array", `{"msgIds":[9007199254740993]}`, []int{9007199254740993}, false},
		{"string", `{"msgIds":"[9007199254740993]"}`, []int{9007199254740993}, false},
		{"empty_array", `{"msgIds":[]}`, []int{}, false},
		{"empty_string", `{"msgIds":""}`, []int{}, false},
		{"missing", `{}`, []int{}, false},
		{"null", `{"msgIds":null}`, []int{}, false},
		{"object", `{"msgIds":{}}`, nil, true},
		{"fraction", `{"msgIds":[1.5]}`, nil, true},
		{"null_element", `{"msgIds":[null]}`, nil, true},
		{"zero", `{"msgIds":[0]}`, nil, true},
		{"negative", `{"msgIds":[-1]}`, nil, true},
		{"overflow", `{"msgIds":[9223372036854775808]}`, nil, true},
		{"bad_string", `{"msgIds":"oops"}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: response forms contain synthetic identifiers only.
			var response AddReactionResponse
			// Act.
			err := json.Unmarshal([]byte(tc.body), &response)
			// Assert: preserve integers; malformed data cannot imply success.
			if (err != nil) != tc.invalid {
				t.Fatalf("decode error mismatch: %v", err)
			}
			if !tc.invalid && !reflect.DeepEqual(response.MsgIDs, tc.want) {
				t.Fatalf("unexpected IDs: %v", response.MsgIDs)
			}
		})
	}
}
