package zalo

import (
	"encoding/json"
	"testing"

	"github.com/amrakk/zcago/model"
)

func TestSeenMetadataComesFromObservedCountersAndPreservesMedia(t *testing.T) {
	for _, kind := range []string{"direct", "group"} {
		for _, fields := range []string{`,"st":3,"at":0,"cmd":501`, `,"st":null,"at":0,"cmd":501`, ``} {
			t.Run(kind+fields, func(t *testing.T) {
				assertSeenWireProjection(t, kind, fields)
			})
		}
	}
}

func assertSeenWireProjection(t *testing.T, kind, fields string) {
	t.Helper()
	// Arrange: actual decoder followed by production normalization.
	raw := []byte(
		`{"msgId":"123","cliMsgId":"456","uidFrom":"peer","idTo":"owner","msgType":"chat.photo","ts":"1791028800000","content":{"title":"","href":"https://example.invalid/photo"}}`,
	)
	raw = append(raw[:len(raw)-1], []byte(fields+`}`)...)
	var wire model.TMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	// Act.
	got, err := ConvertDirect(model.NewUserMessage("owner", wire), "live")
	if kind == "group" {
		wire.IDTo = "group"
		got, err = Convert(
			model.NewGroupMessage("owner", model.TGroupMessage{TMessage: wire}),
			"live",
		)
	}
	// Assert: known zero survives; missing/null never becomes a usable receipt.
	if err != nil {
		t.Fatal(err)
	}
	if wire.SeenFieldsPresent {
		m := got.SeenMetadata
		if m == nil || m.ST != 3 || m.AT != 0 || m.CMD != 501 || m.ClientMessageID != "456" ||
			m.MessageType != "chat.photo" ||
			m.Timestamp != "1791028800000" ||
			m.RecipientID != wire.IDTo {
			t.Fatal("native metadata changed")
		}
	} else if got.SeenMetadata != nil {
		t.Fatal("invented native counters")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &public); err != nil || public["SeenMetadata"] != nil {
		t.Fatal("private metadata exposed")
	}
}
