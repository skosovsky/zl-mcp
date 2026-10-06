package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestMCPDiscoverySearchContextAndResourceAuthorization(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	store, e := storage.Open(ctx, filepath.Join(dir, "messages.sqlite"), []string{"g"}, 90)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.ReplaceCatalog(ctx, []domain.Group{{ID: "g", Name: "Test group"}}); e != nil {
		t.Fatal(e)
	}
	if e = store.Put(ctx, domain.Message{GroupID: "g", ID: "m", SenderID: "s", SentAt: time.Now(), Text: "Ремонт. " + strings.Repeat("a", 9000), Source: "live"}); e != nil {
		t.Fatal(e)
	}
	server, e := New(store, dir)
	if e != nil {
		t.Fatal(e)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, e := server.Connect(ctx, st, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "contract-test", Version: "1"}, nil)
	cs, e := client.Connect(ctx, ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	// Act
	tools, e := cs.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	result, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_search_messages", Arguments: map[string]any{"query": "РЕМОНТ", "limit": "10"}})
	// Assert
	if len(tools.Tools) != 21 {
		t.Fatalf("tools=%d", len(tools.Tools))
	}
	if e != nil || result.IsError {
		t.Fatalf("search failed: %+v %v", result, e)
	}
	for i := 1; i < len(tools.Tools); i++ {
		if tools.Tools[i-1].Name >= tools.Tools[i].Name {
			t.Fatal("catalog is not sorted")
		}
	}
	// Act
	result, e = cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_get_message_context", Arguments: map[string]any{"group_id": "g", "message_id": "m"}})
	// Assert
	if e != nil || result.IsError {
		t.Fatalf("context failed: %+v %v", result, e)
	}
	b, _ := json.Marshal(result.StructuredContent)
	var v map[string]any
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	anchor := v["anchor"].(map[string]any)
	if anchor["text_truncated"] != true || anchor["text_resource_uri"] == nil {
		t.Fatal("long text silently truncated")
	}
	// Act
	resource, e := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: anchor["text_resource_uri"].(string)})
	_, denied := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "zalo://groups/forbidden/messages/m"})
	// Assert
	if e != nil || len(resource.Contents[0].Text) < 9000 {
		t.Fatalf("resource failed: %v", e)
	}
	if denied == nil {
		t.Fatal("resource bypassed allowlist")
	}
	// Act
	bad, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_get_status", Arguments: map[string]any{"extra": "secret"}})
	// Assert
	if e != nil || !bad.IsError {
		t.Fatal("invalid tool input did not produce a tool error")
	}
}
