package mcpserver

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:cyclop,gocognit,gocyclo // Keep the complete Arrange–Act–Assert continuation and boundary scenario together.
func TestOversizedRecordAndCoverageReturnActionableErrors(t *testing.T) {
	// Arrange: legitimate data too large even for a single result page.
	// This checks response budgets, not wall-clock speed. Fixture preparation
	// must not consume an arbitrary shared deadline under the race detector.
	ctx := t.Context()
	dir := t.TempDir()
	store, err := storage.Open(ctx, filepath.Join(dir, "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.ReplaceCatalog(ctx, []domain.Group{{ID: "g", Name: strings.Repeat("名", 20000)}}); err != nil {
		t.Fatal(err)
	}
	if err = store.Put(
		ctx,
		domain.Message{GroupID: "g", ID: "m", SenderID: "sender", SentAt: time.Now(), Text: "Ремонт", Source: "live"},
	); err != nil {
		t.Fatal(err)
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
	client := mcp.NewClient(&mcp.Implementation{Name: "oversized-result", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	schema, err := contracts.Compile("control", "output")
	if err != nil {
		t.Fatal(err)
	}
	assertError := func(name string, args map[string]any) {
		t.Helper()
		// Act
		result, callToolErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		// Assert: safe bounded error, not invalid argument or false storage failure.
		if callToolErr != nil || !result.IsError || len(result.Content) != 1 {
			t.Fatalf("%s result=%+v error=%v", name, result, callToolErr)
		}
		content := result.Content[0].(*mcp.TextContent)
		var body map[string]any
		if callToolErr = json.Unmarshal([]byte(content.Text), &body); callToolErr != nil {
			t.Fatal(callToolErr)
		}
		if callToolErr = schema.Validate(body); callToolErr != nil {
			t.Fatal(callToolErr)
		}
		if body["code"] != "RESPONSE_TOO_LARGE" || body["next_action"].(map[string]any)["instruction"] == "" {
			t.Fatalf("%s error=%+v", name, body)
		}
		wire, callToolErr := json.Marshal(result)
		if callToolErr != nil || len(wire) > 64<<10 {
			t.Fatalf("oversized error response: %d %v", len(wire), callToolErr)
		}
	}
	assertError("zalo_list_groups", map[string]any{})
	assertError("zalo_search_messages", map[string]any{"query": "ремонт", "group_id": "g"})
	assertError("zalo_list_conversation_messages", map[string]any{"conversation_type": "group", "conversation_id": "g"})
	// Arrange: compact records with an oversized coverage history.
	if err = store.ReplaceCatalog(ctx, []domain.Group{{ID: "g", Name: "Test"}}); err != nil {
		t.Fatal(err)
	}
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for range 800 {
		if _, err = tx.ExecContext(
			ctx,
			"INSERT INTO collection_gaps(group_id,started_at,ended_at,reason) VALUES(?,?,?,?)",
			"g",
			"2026-10-01T00:00:00Z",
			"2026-10-01T00:01:00Z",
			"listener_disconnected",
		); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertError("zalo_search_messages", map[string]any{"query": "ремонт", "group_id": "g"})
	assertError("zalo_get_message_context", map[string]any{"group_id": "g", "message_id": "m", "before": 0, "after": 0})
	// Act/Assert: an explicit compact representation keeps the same anchor/search
	// readable and exposes every omitted gap through a signed continuation.
	for index, spec := range []struct {
		name string
		args map[string]any
	}{
		{"zalo_get_message_context", map[string]any{"group_id": "g", "message_id": "m", "before": 0, "after": 0, "coverage_mode": "compact"}},
		{"zalo_get_conversation_message_context", map[string]any{"conversation_type": "group", "conversation_id": "g", "message_id": "m", "before": 0, "after": 0, "coverage_mode": "compact"}},
		{"zalo_search_messages", map[string]any{"group_id": "g", "query": "ремонт", "coverage_mode": "compact"}},
		{"zalo_search_conversation_messages", map[string]any{"conversation_type": "group", "conversation_id": "g", "query": "ремонт", "coverage_mode": "compact"}},
		{"zalo_list_conversation_messages", map[string]any{"conversation_type": "group", "conversation_id": "g", "coverage_mode": "compact"}},
		{"zalo_get_conversation", map[string]any{"conversation_type": "group", "conversation_id": "g", "coverage_mode": "compact"}},
	} {
		result, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: spec.name, Arguments: spec.args})
		if e != nil || result.IsError {
			t.Fatalf("%s compact failed: %+v %v", spec.name, result, e)
		}
		wire, e := json.Marshal(result.StructuredContent)
		if e != nil {
			t.Fatal(e)
		}
		var body map[string]any
		if e = json.Unmarshal(wire, &body); e != nil {
			t.Fatal(e)
		}
		successSchema, e := contracts.Compile(spec.name, "output")
		if e != nil {
			t.Fatal(e)
		}
		if e = successSchema.Validate(body); e != nil {
			t.Fatal(e)
		}
		var coverage map[string]any
		if list, ok := body["coverage"].([]any); ok {
			coverage = list[0].(map[string]any)
		} else {
			coverage = body["coverage"].(map[string]any)
		}
		details := coverage["coverage_details"].(map[string]any)
		if len(coverage["known_gaps"].([]any)) != 0 || details["total_known_gaps"] != float64(800) ||
			details["gaps_truncated"] != true {
			t.Fatalf("invalid compact projection %+v", coverage)
		}
		if index > 0 {
			continue
		}
		cursor := details["gaps_cursor"]
		count := 0
		for cursor != nil {
			page, e := cs.CallTool(
				ctx,
				&mcp.CallToolParams{
					Name: "zalo_list_coverage_gaps",
					Arguments: map[string]any{
						"conversation_type": "group",
						"conversation_id":   "g",
						"cursor":            cursor,
						"limit":             100,
					},
				},
			)
			if e != nil || page.IsError {
				t.Fatalf("gaps %+v %v", page, e)
			}
			raw, e := json.Marshal(page.StructuredContent)
			if e != nil {
				t.Fatal(e)
			}
			var value map[string]any
			if e = json.Unmarshal(raw, &value); e != nil {
				t.Fatal(e)
			}
			if len(raw) > 64<<10 {
				t.Fatal("gap page over budget")
			}
			count += len(value["gaps"].([]any))
			cursor = value["next_cursor"]
		}
		if count != 800 {
			t.Fatalf("%s got %d gaps", spec.name, count)
		}
	}
}
