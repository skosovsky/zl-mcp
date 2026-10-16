package zalo

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:gocognit // One AAA path verifies native conversion, storage, resolution and quote boundaries.
func TestNativeMediaAndGroupClientReferencesReachReactionTargets(t *testing.T) {
	for _, kind := range []model.ThreadType{model.ThreadTypeUser, model.ThreadTypeGroup} {
		name := "direct"
		if kind == model.ThreadTypeGroup {
			name = "group"
		}
		t.Run(name, func(t *testing.T) {
			// Arrange: a title-less native photo with its protocol client identity.
			s, err := storage.OpenWithPolicy(
				t.Context(),
				filepath.Join(t.TempDir(), "ids.sqlite"),
				domain.CollectionPolicy{All: true},
				90,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			wire := model.TMessage{
				MsgID:    "999",
				CliMsgID: "123",
				UIDFrom:  "peer",
				IDTo:     "0",
				TS:       "1790812800123",
				MsgType:  "chat.photo",
				Content:  model.Content{Attachment: &model.TAttachmentContent{}},
			}
			var message domain.Message
			if kind == model.ThreadTypeUser {
				message, err = ConvertDirect(model.NewUserMessage("owner", wire), "live")
			} else {
				wire.IDTo = "group"
				message, err = Convert(model.NewGroupMessage("owner", model.TGroupMessage{TMessage: wire}), "live")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Put(t.Context(), message); err != nil {
				t.Fatal(err)
			}
			reactionWire := model.TReaction{
				ActionID: "native-action",
				UIDFrom:  "peer",
				IDTo:     wire.IDTo,
				TS:       wire.TS,
				Content:  model.ReactionContent{RMsg: []model.ReactionMessageRef{{CMsgID: 123}}},
			}
			// Act: normalize a native client-only reaction, then persist it.
			a, err := normalizeReaction(model.NewReaction("owner", reactionWire, kind), "live")
			if err != nil {
				t.Fatal(err)
			}
			if err = s.PutActivity(t.Context(), a); err != nil {
				t.Fatal(err)
			}
			var body string
			if err = s.DB.QueryRow(`SELECT payload_json FROM activity_observations`).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var stored domain.Activity
			if err = json.Unmarshal([]byte(body), &stored); err != nil {
				t.Fatal(err)
			}
			// Assert: photo/group targets are known without granting quote authority.
			if message.ProtocolClientMessageID != "123" || message.QuoteMetadata != nil ||
				stored.ConversationID != message.Ref().ID ||
				len(stored.Reaction.TargetMessageIDs) != 1 ||
				stored.Reaction.TargetMessageIDs[0] != "999" {
				t.Fatal("native protocol alias lost or quote authority expanded")
			}
			var quotes int
			if err = s.DB.QueryRow(`SELECT count(*) FROM message_quote_metadata`).
				Scan(&quotes); err != nil ||
				quotes != 0 {
				t.Fatal("media/group alias became quote metadata", err)
			}
			if _, err = s.SendQuote(t.Context(), message.Ref().ID, message.ID); err == nil {
				t.Fatal("media alias granted send quote authority")
			}
		})
	}
}
