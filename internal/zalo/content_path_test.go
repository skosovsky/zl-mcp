//nolint:cyclop // One cross-layer AAA scenario keeps adapter/storage/profile evidence in order.
package zalo_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/zalo"
)

//nolint:gocognit,govet // This AAA scenario traces one synthetic title through all affected layers.
func TestAttachmentTitleIsMetadataAcrossAdapterStoreAndProfiles(t *testing.T) {
	for _, native := range []string{"share.file", "chat.photo", "chat.recommended"} {
		t.Run(native, func(t *testing.T) {
			// Arrange: a native attachment title is not a proven author caption.
			ctx := context.Background()
			var content model.Content
			wire := []byte(`{"title":"report.pdf","href":"https://synthetic.invalid/private"}`)
			if err := json.Unmarshal(wire, &content); err != nil {
				t.Fatal(err)
			}
			raw := model.NewUserMessage(
				"owner",
				model.TMessage{
					MsgID:   "id",
					UIDFrom: "peer",
					IDTo:    "0",
					MsgType: native,
					TS:      "1790812800123",
					Content: content,
				},
			)
			// Act: actual adapter -> persistent row -> typed context / browse / both profiles.
			normalized, err := zalo.ConvertDirect(raw, "replay")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "messages.sqlite")
			s, err := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err := s.Put(ctx, normalized); err != nil {
				t.Fatal(err)
			}
			ref := domain.ConversationRef{Type: "direct", ID: "peer"}
			saved, err := s.ConversationMessage(ctx, ref, "id")
			if err != nil {
				t.Fatal(err)
			}
			view, err := s.ConversationContext(ctx, ref, "id", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			page, err := s.Browse(ctx, ref, "", "", "asc", 20, "")
			if err != nil {
				t.Fatal(err)
			}
			encoder, err := events.NewMessageEncoder()
			if err != nil {
				t.Fatal(err)
			}
			rich, err := encoder.EncodeProfile(domain.ConversationMessageCreatedV3, "event", saved)
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := encoder.EncodeProfile(domain.ConversationMessageCreatedV2, "event", saved)
			if err != nil {
				t.Fatal(err)
			}
			var richEvent, oldEvent map[string]any
			if err := json.Unmarshal(rich, &richEvent); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(legacy, &oldEvent); err != nil {
				t.Fatal(err)
			}
			// Assert: old projection preserved, new authored text absent, metadata retained.
			if oldEvent["data"].(map[string]any)["text"] != "report.pdf" {
				t.Fatal("v2 projection changed")
			}
			if richEvent["data"].(map[string]any)["text"] != "" || view["anchor"].(map[string]any)["text"] != "" ||
				page["messages"].([]map[string]any)[0]["excerpt"] != "" {
				t.Fatal("attachment title became author text")
			}
			if saved.Content.TextRole != "none" || *saved.Content.Elements[0].Label != "report.pdf" {
				t.Fatal("metadata or provenance lost")
			}
		})
	}
}
