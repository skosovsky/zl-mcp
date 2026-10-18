package mcpserver

import (
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

//nolint:gocognit // Keep the complete Arrange–Act–Assert continuation and boundary scenario together.
func TestCorpusBrowseBudgetShorteningKeepsSnapshotContinuation(t *testing.T) {
	// Arrange: each item fits, but the requested 50-record page exceeds wire budget.
	ctx := t.Context()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.ReplaceCatalog(ctx, []domain.Group{{ID: "g", Name: strings.Repeat("name", 700)}}); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC()
	for i := range 30 {
		if err = store.Put(
			ctx,
			domain.Message{
				GroupID:  "g",
				ID:       fmt.Sprintf("m%02d", i),
				SenderID: "sender",
				SentAt:   stamp.Add(time.Duration(i) * time.Second),
				Text:     "short",
				Source:   "live",
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	server, err := New(migrationReadPorts(store))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "browse-budget", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	// Act: follow exactly server-issued cursors while every page is shortened.
	args := map[string]any{
		"conversation_type": "group",
		"conversation_id":   "g",
		"order":             "asc",
		"limit":             50,
		"coverage_mode":     "compact",
	}
	seen := map[string]bool{}
	pages := 0
	for {
		result, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_list_conversation_messages", Arguments: args})
		if e != nil || result.IsError {
			t.Fatalf("browse %+v %v", result, e)
		}
		wire, e := json.Marshal(result)
		if e != nil {
			t.Fatal(e)
		}
		if len(wire) > 64<<10 {
			t.Fatalf("wire over budget %d", len(wire))
		}
		raw, e := json.Marshal(result.StructuredContent)
		if e != nil {
			t.Fatal(e)
		}
		var body map[string]any
		if e = json.Unmarshal(raw, &body); e != nil {
			t.Fatal(e)
		}
		for _, item := range body["messages"].([]any) {
			id := item.(map[string]any)["message_id"].(string)
			if seen[id] {
				t.Fatal("duplicate", id)
			}
			seen[id] = true
		}
		pages++
		if pages == 1 {
			// A later insertion with an old sent_at remains excluded from this snapshot.
			if e = store.Put(
				ctx,
				domain.Message{
					GroupID:  "g",
					ID:       "late",
					SenderID: "sender",
					SentAt:   stamp,
					Text:     "late",
					Source:   "live",
				},
			); e != nil {
				t.Fatal(e)
			}
		}
		if body["has_more"] == false {
			break
		}
		args["cursor"] = body["next_cursor"]
	}
	// Assert: shrinking consumes no unseen rows and preserves original snapshot.
	if pages < 2 || len(seen) != 30 || seen["late"] {
		t.Fatalf("pages=%d seen=%d late=%v", pages, len(seen), seen["late"])
	}
}
