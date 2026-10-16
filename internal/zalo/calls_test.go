package zalo

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func syntheticCallRecord() model.TMessage {
	return model.TMessage{
		MsgID:   "call-record",
		UIDFrom: "peer",
		IDTo:    "owner",
		TS:      "1700000000000",
		MsgType: "chat.recommended",
		Content: model.Content{
			Attachment: &model.TAttachmentContent{
				Action:      nativeMissCallAction,
				Title:       "private-title",
				Description: "private-description",
				Params:      `{"callId":11,"calltype":2,"duration":0,"reason":4,"private":"secret"}`,
			},
		},
	}
}

func TestNativeCallSummarySeparatesLiveReplayAndSilentHistory(t *testing.T) {
	// Arrange: the observed discriminator, followed by genuine author text.
	raw := syntheticCallRecord()
	message := model.NewUserMessage("owner", raw)
	gotMessages := []domain.Message{}
	gotActivities := []domain.Activity{}
	onMessage := func(m domain.Message) error { gotMessages = append(gotMessages, m); return nil }
	onActivity := func(a domain.Activity) error { gotActivities = append(gotActivities, a); return nil }
	// Act: both sources use the same classifier/identity and never create messages.
	for _, source := range []string{"live", "replay"} {
		if err := deliverIncomingActivity(message, source, onMessage, onActivity); err != nil {
			t.Fatal(err)
		}
	}
	text := "hello 🙂"
	normal := raw
	normal.MsgType = "webchat"
	normal.MsgID = "real-message"
	normal.Content = model.Content{String: &text}
	if err := deliverIncomingActivity(
		model.NewUserMessage("owner", normal),
		"live",
		onMessage,
		onActivity,
	); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, historyErr := parsePreloadMessage(encoded, "direct", "owner")
	// Assert: unknown outcome, private hint excluded, future message not blocked.
	if len(gotActivities) != 2 || len(gotMessages) != 1 || gotMessages[0].ID != "real-message" ||
		!errors.Is(historyErr, errCallRecord) {
		t.Fatal("call entered message/history or blocked following text", historyErr)
	}
	first := gotActivities[0]
	if first.SourceID != gotActivities[1].SourceID || first.Call.Status != "unknown" || first.Call.Mode != "unknown" ||
		first.Call.DurationMS != nil ||
		first.Call.NativeStatus != nil ||
		first.Call.CallID != nil ||
		first.ProtocolMessageID != raw.MsgID {
		t.Fatal("call invented semantics or unstable identity")
	}
	body, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-title", "private-description", "secret", "protocol", "call-record"} {
		if strings.Contains(string(body), private) {
			t.Fatal("call exposed native metadata")
		}
	}
}

//nolint:gocognit // Table-driven source/direction boundaries keep AAA assertions together.
func TestNativeCallSummaryRequiresExactDirectDiscriminatorAndNumericShape(t *testing.T) {
	for _, name := range []string{"group", "other action", "other native", "bad params", "outgoing"} {
		t.Run(name, func(t *testing.T) {
			// Arrange: adversarial neighboring forms are not evidence of a call.
			raw := syntheticCallRecord()
			var message model.Message
			switch name {
			case "group":
				message = model.NewGroupMessage("owner", model.TGroupMessage{TMessage: raw})
			case "other action":
				raw.Content.Attachment.Action = "recommened.groupcall"
			case "other native":
				raw.MsgType = "chat.photo"
			case "bad params":
				raw.Content.Attachment.Params = `{"callId":null,"calltype":1,"duration":1,"reason":1}`
			case "outgoing":
				raw.UIDFrom = "0"
				raw.IDTo = "peer"
			}
			if message == nil {
				message = model.NewUserMessage("owner", raw)
			}
			messages, activities := 0, 0
			// Act.
			err := deliverIncomingActivity(
				message,
				"live",
				func(domain.Message) error { messages++; return nil },
				func(a domain.Activity) error {
					activities++
					if name == "outgoing" && a.Direction != "outgoing" {
						t.Fatal("self direction lost")
					}
					return nil
				},
			)
			// Assert: malformed known call rejected without reconnect/message/novelty.
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "bad params":
				if messages != 0 || activities != 0 {
					t.Fatal("invalid call persisted")
				}
			case "outgoing":
				if messages != 0 || activities != 1 {
					t.Fatal("outgoing call not isolated")
				}
			default:
				if messages != 1 || activities != 0 {
					t.Fatal("unverified source classified as call")
				}
			}
		})
	}
}

func TestNativeCallTimeSummaryWithoutReasonPreservesUnknownsAndActionIdentity(t *testing.T) {
	// Arrange: synthetic values, with the three-key numeric form observed natively.
	raw := syntheticCallRecord()
	missed, err := normalizeCall(model.NewUserMessage("owner", raw), "live")
	if err != nil {
		t.Fatal(err)
	}
	raw.Content.Attachment.Action = nativeCallTimeAction
	raw.Content.Attachment.Params = `{"callId":11,"calltype":2,"duration":7,"private":"secret"}`
	var previous string
	for _, source := range []string{"live", "replay"} {
		messages, activities := 0, 0
		// Act: calltime takes the same activity-only route and stable identity.
		err = deliverIncomingActivity(
			model.NewUserMessage("owner", raw),
			source,
			func(domain.Message) error { messages++; return nil },
			func(a domain.Activity) error {
				activities++
				if a.SourceID == missed.SourceID || (previous != "" && a.SourceID != previous) ||
					a.Call.Status != "unknown" ||
					a.Call.Mode != "unknown" ||
					a.Call.DurationMS != nil ||
					a.Call.NativeStatus != nil ||
					a.Call.CallID != nil {
					t.Fatal("calltime guessed semantics or collided with another action")
				}
				previous = a.SourceID
				return nil
			},
		)
		// Assert: no message side effect and no swallowed valid activity.
		if err != nil || messages != 0 || activities != 1 {
			t.Fatal("calltime was not isolated", err)
		}
	}
	// Act / Assert: silent preload rejects it as a nonmessage, never as author text.
	wire, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parsePreloadMessage(wire, "direct", "owner"); !errors.Is(err, errCallRecord) {
		t.Fatal("calltime became a preload message", err)
	}
	for _, params := range []string{`{"callId":11,"calltype":2,"duration":"7"}`, `{"callId":11,"calltype":2}`} {
		if validCallParams(nativeCallTimeAction, params) {
			t.Fatal("invalid calltime params accepted")
		}
	}
	if validCallParams(nativeMissCallAction, `{"callId":11,"calltype":2,"duration":7}`) {
		t.Fatal("misscall lost reason guard")
	}
}
