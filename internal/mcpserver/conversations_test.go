package mcpserver

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConversationMCPSearchContextCatalogueAndFullText(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(dir, "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stamp := time.Now()
	text := "synthetic " + strings.Repeat("я", 9000)
	name := "Synthetic peer"
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: "same/opaque%"}
	for _, kind := range []string{domain.ConversationDirect, domain.ConversationGroup} {
		if err = store.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: ref.ID}, ID: "m/opaque%", SenderID: ref.ID, SenderName: &name, SentAt: stamp, Text: text, Source: "replay"}); err != nil {
			t.Fatal(err)
		}
	}
	server, err := New(store, dir)
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "conversation-test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		r, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if e != nil || r.IsError {
			t.Fatalf("%s: %v %+v", name, e, r)
		}
		b, e := json.Marshal(r.StructuredContent)
		if e != nil {
			t.Fatal(e)
		}
		v := map[string]any{}
		if e = json.Unmarshal(b, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	// Act
	first := call("zalo_search_conversation_messages", map[string]any{"query": "synthetic", "limit": 1})
	second := call("zalo_search_conversation_messages", map[string]any{"query": "synthetic", "limit": 1, "cursor": first["next_cursor"]})
	contextResult := call("zalo_get_conversation_message_context", map[string]any{"conversation_type": ref.Type, "conversation_id": ref.ID, "message_id": "m/opaque%"})
	catalog := call("zalo_list_conversations", map[string]any{})
	metadata := call("zalo_get_conversation", map[string]any{"conversation_type": ref.Type, "conversation_id": ref.ID})
	full, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: contextResult["anchor"].(map[string]any)["text_resource_uri"].(string)})
	// Assert
	if err != nil || full.Contents[0].Text != text {
		t.Fatal("escaped full-text resource failed")
	}
	a := first["messages"].([]any)[0].(map[string]any)
	b := second["messages"].([]any)[0].(map[string]any)
	if a["conversation_type"] == b["conversation_type"] || first["has_more"] != true || second["has_more"] != false {
		t.Fatal("typed pagination lost or duplicated a collision")
	}
	if _, present := a["group_id"]; present {
		t.Fatal("general hit leaked legacy address")
	}
	if len(catalog["conversations"].([]any)) != 2 || catalog["catalog_complete"] != false || metadata["conversation"].(map[string]any)["name"] != name {
		t.Fatal("catalogue discovery/name/completeness incorrect")
	}
	if contextResult["anchor"].(map[string]any)["conversation_type"] != "direct" || len(contextResult["before"].([]any)) != 0 || len(contextResult["after"].([]any)) != 0 {
		t.Fatal("context crossed namespace")
	}
	status, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "zalo://collection"})
	if err != nil {
		t.Fatal(err)
	}
	var observed map[string]any
	if err = json.Unmarshal([]byte(status.Contents[0].Text), &observed); err != nil {
		t.Fatal(err)
	}
	counts := observed["message_counts"].(map[string]any)
	if observed["mode"] != "all" || counts["direct"] != float64(1) || counts["group"] != float64(1) || observed["catalog_complete"] != false {
		t.Fatal("collection diagnostics mismatch")
	}
	// Act / Assert: a general cursor is bound to the original filters.
	invalid, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_search_conversation_messages", Arguments: map[string]any{"query": "synthetic", "conversation_type": "group", "cursor": first["next_cursor"]}})
	if err != nil || !invalid.IsError {
		t.Fatal("cursor accepted different namespace filter")
	}
}
