package zalo

import (
	"encoding/json"
	"github.com/amrakk/zcago/model"
	"testing"
	"time"
)

func TestNormalizeGroupTextAndReply(t *testing.T) {
	// Arrange
	text := "Тестовое сообщение"
	raw := model.NewGroupMessage("own", model.TGroupMessage{TMessage: model.TMessage{MsgID: "9007199254740993", UIDFrom: "sender", IDTo: "group", TS: "1790812800123", DName: "Test", Content: model.Content{String: &text}, Quote: &model.TQuote{GlobalMsgID: 42}}})
	// Act
	result, err := Convert(raw, "replay")
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "9007199254740993" || result.GroupID != "group" || result.Text != text || result.Source != "replay" || *result.ReplyTo != "42" {
		t.Fatalf("bad normalization: %+v", result)
	}
	if !result.SentAt.Equal(time.UnixMilli(1790812800123)) {
		t.Fatal("timestamp lost precision")
	}
}

func TestQuoteWireIntegerAndStringIDs(t *testing.T) {
	// Arrange: live error reduced to synthetic quote fields, without account data.
	for _, raw := range []string{
		`{"ownerId":"sender","cliMsgId":"9007199254740993","globalMsgId":"9007199254740994","ts":"1790812800123"}`,
		`{"ownerId":"sender","cliMsgId":9007199254740993,"globalMsgId":9007199254740994,"ts":1790812800123}`,
	} {
		// Act
		var quote model.TQuote
		err := json.Unmarshal([]byte(raw), &quote)
		// Assert
		if err != nil || quote.CliMsgID != 9007199254740993 || quote.GlobalMsgID != 9007199254740994 || quote.Timestamp != 1790812800123 {
			t.Fatalf("quote decoding failed: %v", err)
		}
	}
	for _, value := range []string{`"not-an-id"`, `"9223372036854775808"`, `1.5`, `true`} {
		// Act/Assert: invalid representations are rejected, never rounded.
		var quote model.TQuote
		if err := json.Unmarshal([]byte(`{"ownerId":"sender","cliMsgId":`+value+`}`), &quote); err == nil {
			t.Fatal("invalid quote ID accepted")
		}
	}
}
