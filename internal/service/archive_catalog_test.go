package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func testOfflineArchiveCatalogue(t *testing.T, owner *membershipPort, sourceID string) {
	t.Helper()
	// Arrange: authenticated library has two direct files and a colliding group ID.
	ctx := context.Background()
	restricted, err := owner.Call(ctx, "zalo_list_conversations", map[string]any{"source_id": sourceID})
	if err != nil || len(restricted["conversations"].([]map[string]any)) != 0 {
		t.Fatal("restricted archive leaked peers", err)
	}
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.BindAccount(ctx, "10"); err != nil {
		t.Fatal(err)
	}
	offline := &membershipPort{store: store, library: owner.library, stateDir: t.TempDir()}
	seen := map[domain.ConversationRef]bool{}
	args := map[string]any{"source_id": sourceID, "limit": 1}
	schema, err := contracts.Compile("zalo_list_conversations", "output")
	if err != nil {
		t.Fatal(err)
	}
	var firstCursor string
	// Act: page the immutable file mapping without a collector or catalogue rows.
	for page := 0; page < 3; page++ {
		result, err := offline.Call(ctx, "zalo_list_conversations", args)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(result)
		var value any
		json.Unmarshal(encoded, &value)
		if err = schema.Validate(value); err != nil {
			t.Fatal("archive catalogue contract", err)
		}
		items := result["conversations"].([]map[string]any)
		if len(items) != 1 {
			t.Fatal("archive pagination skipped mapping")
		}
		ref := domain.ConversationRef{Type: items[0]["conversation_type"].(string), ID: items[0]["conversation_id"].(string)}
		if seen[ref] {
			t.Fatal("archive pagination repeated mapping")
		}
		seen[ref] = true
		if page < 2 {
			token, ok := result["next_cursor"].(string)
			if !ok || !result["has_more"].(bool) {
				t.Fatal("missing archive cursor")
			}
			args["cursor"] = token
			if page == 0 {
				firstCursor = token
			}
		} else if result["has_more"].(bool) || result["next_cursor"] != nil {
			t.Fatal("archive pagination did not stop")
		}
	}
	// Assert: typed collisions stay separate; cursor substitution and new filters fail.
	if len(seen) != 3 || !seen[domain.ConversationRef{Type: "direct", ID: "12"}] || !seen[domain.ConversationRef{Type: "group", ID: "12"}] {
		t.Fatal("typed source mapping changed")
	}
	if _, err = offline.Call(ctx, "zalo_list_conversations", map[string]any{"source_id": sourceID, "cursor": firstCursor, "conversation_type": "group"}); err == nil {
		t.Fatal("cursor filter substitution accepted")
	}
	narrowed := &membershipPort{store: owner.store, library: owner.library}
	denied, err := narrowed.Call(ctx, "zalo_list_conversations", map[string]any{"source_id": sourceID, "cursor": firstCursor})
	if err != nil || len(denied["conversations"].([]map[string]any)) != 0 {
		t.Fatal("cursor bypassed current policy", err)
	}
	result, err := offline.Call(ctx, "zalo_list_conversations", map[string]any{"source_id": sourceID, "query": "unobserved name"})
	if err != nil || len(result["conversations"].([]map[string]any)) != 0 {
		t.Fatal("unknown names invented", err)
	}
	testOfflineArchiveMessages(t, offline, owner, sourceID)
	for _, table := range []string{"conversations", "messages", "message_events", "event_deliveries", "send_operations", "event_subscriptions"} {
		var n int
		if err = store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("archive catalogue modified corpus", table, err)
		}
	}
}
