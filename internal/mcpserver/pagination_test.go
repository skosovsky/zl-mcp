package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestLargeWirePagesKeepAllResultsAndSearchSnapshot(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	store, err := storage.Open(ctx, filepath.Join(dir, "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	groups := []domain.Group{}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		groups = append(groups, domain.Group{ID: fmt.Sprintf("g%02d", i), Name: strings.Repeat("名", 3000)})
	}
	groups = append(groups, domain.Group{ID: "g", Name: strings.Repeat("名", 3000)})
	if err = store.ReplaceCatalog(ctx, groups); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err = store.Put(ctx, domain.Message{GroupID: "g", ID: fmt.Sprintf("m%02d", i), SenderID: "sender", SentAt: at.Add(time.Duration(i) * time.Minute), Text: "Ремонт", Source: "live"}); err != nil {
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
	client := mcp.NewClient(&mcp.Implementation{Name: "wire-budget", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		r, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if e != nil || r.IsError {
			t.Fatalf("call failed: %+v %v", r, e)
		}
		wire, e := json.Marshal(r)
		if e != nil || len(wire) > 64<<10 {
			t.Fatalf("wire budget: %d %v", len(wire), e)
		}
		b, _ := json.Marshal(r.StructuredContent)
		var value map[string]any
		if e = json.Unmarshal(b, &value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	// Act: caller always asks for 50; the transport chooses a smaller fitting page.
	groupIDs := map[string]bool{}
	args := map[string]any{"limit": 50}
	pages := 0
	for {
		result := call("zalo_list_groups", args)
		pages++
		items := result["groups"].([]any)
		for _, item := range items {
			id := item.(map[string]any)["group_id"].(string)
			if groupIDs[id] {
				t.Fatal("duplicate group across shortened pages")
			}
			groupIDs[id] = true
		}
		if !result["has_more"].(bool) {
			if result["next_cursor"] != nil {
				t.Fatal("cursor without more results")
			}
			break
		}
		args["cursor"] = result["next_cursor"]
	}
	// Assert
	if len(groupIDs) != 13 || pages < 2 {
		t.Fatalf("groups lost: %d over %d pages", len(groupIDs), pages)
	}
	// Act: insert a backdated matching message after the first page.
	args = map[string]any{"query": "Ремонт", "limit": 50}
	messageIDs := map[string]bool{}
	snapshot := ""
	pages = 0
	for {
		result := call("zalo_search_messages", args)
		pages++
		if snapshot == "" {
			snapshot = result["snapshot_at"].(string)
		} else if snapshot != result["snapshot_at"] {
			t.Fatal("snapshot changed during budget resizing")
		}
		for _, item := range result["messages"].([]any) {
			id := item.(map[string]any)["message_id"].(string)
			if messageIDs[id] {
				t.Fatal("duplicate search hit")
			}
			messageIDs[id] = true
		}
		if pages == 1 {
			if err = store.Put(ctx, domain.Message{GroupID: "g", ID: "late", SenderID: "sender", SentAt: at, Text: "Ремонт", Source: "live"}); err != nil {
				t.Fatal(err)
			}
		}
		if !result["has_more"].(bool) {
			break
		}
		args["cursor"] = result["next_cursor"]
	}
	// Assert
	if len(messageIDs) != 12 || messageIDs["late"] || pages < 2 {
		t.Fatalf("snapshot/pagination failed: %+v", messageIDs)
	}
}
