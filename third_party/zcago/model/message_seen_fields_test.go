package model

import (
	"encoding/json"
	"testing"
)

func TestSeenCountersRequireWirePresenceAndGroupMentionsSurvive(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		present      bool
	}{
		{"zero", `,"st":0,"at":0,"cmd":0`, true},
		{"nonzero", `,"st":3,"at":7,"cmd":501`, true},
		{"missing", `,"at":0,"cmd":0`, false},
		{"null", `,"st":null,"at":0,"cmd":0`, false},
		{"none", ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: reuse a previously complete object to catch stale presence flags.
			got := TGroupMessage{TMessage: TMessage{SeenFieldsPresent: true}}
			data := []byte(`{"msgId":"123","content":"test","mentions":[{"uid":"peer"}]` + tc.fields + `}`)
			// Act.
			err := json.Unmarshal(data, &got)
			encoded, encodeErr := json.Marshal(got)
			// Assert: group fields survive; provenance is internal only.
			if err != nil || encodeErr != nil || got.SeenFieldsPresent != tc.present || got.MsgID != "123" || len(got.Mentions) != 1 {
				t.Fatalf("decode lost provenance or group fields: %#v %v %v", got, err, encodeErr)
			}
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(encoded, &fields); err != nil || fields["SeenFieldsPresent"] != nil {
				t.Fatal("presence leaked")
			}
			if tc.name == "nonzero" && (got.ST != 3 || got.AT != 7 || got.CMD != 501) {
				t.Fatal("counters changed")
			}
		})
	}
	// Arrange / Act / Assert: invalid types still fail rather than inventing fields.
	for _, raw := range []string{`{"st":"0"}`, `{"at":1.5}`, `{"cmd":true}`} {
		var got TMessage
		if err := json.Unmarshal([]byte(raw), &got); err == nil {
			t.Fatal("invalid counter accepted")
		}
	}
}
