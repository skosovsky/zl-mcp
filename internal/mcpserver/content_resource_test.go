package mcpserver

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amrakk/zcago/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/zalo"
)

func TestControlledPhotoCaptionUsesActualFullTextResource(t *testing.T) {
	// Arrange: long synthetic caption through the proven shape, not a copied
	// native payload; quote absence and guards here are explicit test inputs.
	ctx := t.Context()
	caption := strings.Repeat("👩🏽‍💻🙂", 600)
	raw := model.TMessage{MsgID: "300", UIDFrom: "200", IDTo: "100", TS: strconv.FormatInt(time.Now().UnixMilli(), 10),
		MsgType: "chat.photo", Content: model.Content{Attachment: &model.TAttachmentContent{
			Title: caption, Params: `{"width":20,"height":30}`,
		}}}
	message, err := zalo.ConvertDirect(model.NewUserMessage("100", raw), "live")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = owner.Put(ctx, message); err != nil {
		t.Fatal(err)
	}
	encoder, err := events.NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	body, err := encoder.EncodeProfile(domain.ConversationMessageCreatedV3, "event", message)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct {
			Text      string `json:"text"`
			Truncated bool   `json:"text_truncated"`
			URI       string `json:"text_resource_uri"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	server, err := New(migrationReadPorts(owner))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "caption-resource-test", Version: "1"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	// Act: resolve the encoder's resource through the actual MCP route.
	resource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: envelope.Data.URI})
	// Assert: Unicode-bound excerpt, full caption and provenance survive storage.
	if !envelope.Data.Truncated || len([]rune(envelope.Data.Text)) != 2048 || err != nil ||
		len(resource.Contents) != 1 ||
		resource.Contents[0].Text != caption {
		t.Fatal("caption lost between adapter, event and full MCP resource", err)
	}
}

//nolint:gocognit // Keep the actual MCP read, narrowed policy, expiry and recall assertions in one ordered AAA scenario.
func TestContentResourceRespectsNamespacePolicyRecallAndExpiry(t *testing.T) {
	// Arrange: colliding namespaces and retained nontext metadata, no media bytes.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	owner, err := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	content := domain.MessageContent{
		Kind:         "image",
		TextRole:     "none",
		Summary:      "Image message; media bytes unavailable",
		Availability: "metadata_only",
		Reason:       new("metadata_limited"),
		Elements:     []domain.ContentElement{{Kind: "image", Label: new("untrusted label"), BytesAvailable: false}},
	}
	// Full metadata can exceed the compact event/journal budget; resource reads
	// still return every schema-valid element without silently trimming labels.
	content.Elements = nil
	for range 16 {
		content.Elements = append(content.Elements, domain.ContentElement{
			Kind: "image", Label: new(strings.Repeat("\x00", 512)), BytesAvailable: false,
		})
	}
	for _, kind := range []string{"direct", "group"} {
		for _, id := range []string{"record", "expired"} {
			if err = owner.Put(
				ctx,
				domain.Message{
					Conversation: domain.ConversationRef{Type: kind, ID: "same/id"},
					ID:           id,
					SenderID:     "peer",
					SentAt:       time.Now().UTC(),
					Source:       "live",
					Content:      &content,
				},
			); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen under narrower authorization: existing group bytes must stay denied.
	owner, err = storage.OpenWithPolicy(
		ctx,
		path,
		domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{{Type: "direct", ID: "same/id"}: true}},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	server, err := New(migrationReadPorts(owner))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "content-resource-test", Version: "1"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	uri := "zalo://conversations/direct/same%2Fid/messages/record/content"
	// Act: actual resources/read route, not a direct store call.
	got, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	// Assert: exact typed JSON and provenance; no fabricated media URL or params.
	if err != nil || len(got.Contents) != 1 {
		t.Fatal("metadata resource unavailable", err)
	}
	expected, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if got.Contents[0].MIMEType != "application/json" || got.Contents[0].Text != string(expected) {
		t.Fatal("metadata representation changed")
	}
	denied := func(uri string) {
		t.Helper()
		if result, readErr := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); readErr == nil {
			t.Fatal("resource visibility bypass", result)
		}
	}
	denied(strings.Replace(uri, "/direct/", "/group/", 1))
	denied(strings.Replace(uri, "same%2Fid", "unknown", 1))
	// Act / Assert: source expiry denies the route before physical cleanup.
	if _, err = owner.DB.ExecContext(
		ctx,
		`INSERT INTO history_message_expiry VALUES('direct','same/id','expired',?,0)`,
		time.Now().Add(-time.Minute).UnixMilli(),
	); err != nil {
		t.Fatal(err)
	}
	denied(strings.Replace(uri, "/record/", "/expired/", 1))
	if err = owner.DeleteConversation(
		ctx,
		domain.ConversationRef{Type: "direct", ID: "same/id"},
		"record",
	); err != nil {
		t.Fatal(err)
	}
	denied(uri)
}
