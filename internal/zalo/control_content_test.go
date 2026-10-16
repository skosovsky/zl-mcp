package zalo

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

//nolint:gocognit // Keep the same control fixture across direct/group live, replay and historical source assertions.
func TestControlsDoNotBecomeLiveReplayOrHistoryMessages(t *testing.T) {
	for _, wire := range []string{
		`[{"actionType":1,"globalDelMsgId":42}]`,
		`{"deleteMsg":1,"globalMsgId":42}`,
		`{"deleteMsg":1,"globalMsgId":42,"title":"metadata must not hide control"}`,
	} {
		// Arrange: control followed by a valid message in the same batch.
		var content model.Content
		if err := json.Unmarshal([]byte(wire), &content); err != nil {
			t.Fatal(err)
		}
		raw := model.TMessage{MsgID: "control", UIDFrom: "peer", IDTo: "owner", TS: "1700000000000", Content: content}
		text := "🙂"
		normal := model.TMessage{
			MsgID:   "message",
			UIDFrom: "peer",
			IDTo:    "owner",
			TS:      raw.TS,
			MsgType: "webchat",
			Content: model.Content{String: &text},
		}
		for _, kind := range []string{"direct", "group"} {
			batch := []model.Message{model.NewUserMessage("owner", raw), model.NewUserMessage("owner", normal)}
			if kind == "group" {
				batch = []model.Message{
					model.NewGroupMessage("owner", model.TGroupMessage{TMessage: raw}),
					model.NewGroupMessage("owner", model.TGroupMessage{TMessage: normal}),
				}
			}
			got := []domain.Message{}
			receive := func(m domain.Message) error { got = append(got, m); return nil }
			// Act: live and replay both bypass the control without stopping the next record.
			if err := deliverIncoming(batch[0], "live", receive); err != nil {
				t.Fatal(err)
			}
			if err := deliverReplay(batch, receive); err != nil {
				t.Fatal(err)
			}
			// Assert: downstream storage/novelty can see only the actual message.
			if len(got) != 1 || got[0].ID != "message" {
				t.Fatal("control projected as message")
			}
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		// Act / Assert: historical decoders preserve the same non-message classification.
		if _, err = parsePreloadMessage(encoded, "direct", "owner"); !errors.Is(err, errControlRecord) {
			t.Fatal("preload control lost", err)
		}
		raw.IDTo = "group"
		encoded, err = json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = decodeHistoryRecord(
			encoded,
			"owner",
			domain.ConversationRef{Type: "group", ID: "group"},
		); !errors.Is(
			err,
			errControlRecord,
		) {
			t.Fatal("history control lost", err)
		}
	}
}
