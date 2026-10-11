package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestMigrationInputCharacterization(t *testing.T) {
	cases := []struct{ name, tool, raw, code string }{
		{"missing-query", "zalo_search_messages", `{}`, "INVALID_ARGUMENT"},
		{"null-query", "zalo_search_messages", `{"query":null}`, "INVALID_ARGUMENT"},
		{"malformed", "zalo_search_messages", `{`, "INVALID_ARGUMENT"},
		{"array", "zalo_search_messages", `[]`, "INVALID_ARGUMENT"},
		{"zero-limit", "zalo_search_messages", `{"query":"x","limit":0}`, "INVALID_ARGUMENT"},
		{"limit-over-bound", "zalo_search_messages", `{"query":"x","limit":51}`, "INVALID_ARGUMENT"},
		{"fractional-limit", "zalo_search_messages", `{"query":"x","limit":1.5}`, "INVALID_ARGUMENT"},
		{"unparseable-limit", "zalo_search_messages", `{"query":"x","limit":"many"}`, "INVALID_ARGUMENT"},
		{"invalid-time", "zalo_search_messages", `{"query":"x","since":"2026-99-01T00:00:00Z"}`, "INVALID_ARGUMENT"},
		{"invalid-uuid", "zalo_get_send_status", `{"request_id":"not-a-uuid"}`, "INVALID_ARGUMENT"},
		{"numeric-id", "zalo_get_message_context", `{"group_id":9007199254740993,"message_id":"m"}`, "INVALID_ARGUMENT"},
		{"opaque-id", "zalo_get_message_context", `{"group_id":"9007199254740993","message_id":"9007199254740993123","before":"0","after":"0"}`, "NOT_FOUND"},
		{"unicode-normalized-limit", "zalo_search_messages", `{"query":"Кофе ☕","limit":"10"}`, ""},
		{"null-object-baseline", "zalo_get_status", `null`, ""},
		{"missing-object-baseline", "zalo_get_status", ``, ""},
		{"explicit-zero-neighbors", "zalo_get_message_context", `{"group_id":"missing","message_id":"missing","before":0,"after":0}`, "NOT_FOUND"},
		{"query-size", "zalo_search_messages", `{"query":"` + strings.Repeat("x", 501) + `"}`, "INVALID_ARGUMENT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: independent synthetic state gives each case a fresh admission budget.
			ctx := context.Background()
			store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), []string{"9007199254740993", "missing"}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			schema, err := contracts.Compile(tc.tool, "input")
			if err != nil {
				t.Fatal(err)
			}
			output, err := contracts.Compile(tc.tool, "output")
			if err != nil {
				t.Fatal(err)
			}
			svc := withMigrationReads(&migrationService{Store: store, input: map[string]*jsonschema.Schema{tc.tool: schema}, output: map[string]*jsonschema.Schema{tc.tool: output}})
			// Act
			result := svc.call(ctx, tc.tool, json.RawMessage(tc.raw))
			// Assert
			if result.IsError != (tc.code != "") {
				t.Fatalf("unexpected result: %+v", result)
			}
			if tc.code != "" {
				var failure map[string]any
				if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &failure); err != nil {
					t.Fatal(err)
				}
				if failure["code"] != tc.code {
					t.Fatalf("code=%v expected=%s", failure["code"], tc.code)
				}
			}
			if tc.name == "unicode-normalized-limit" {
				if err := normalizeSearchSnapshot(result); err != nil {
					t.Fatal(err)
				}
			}
			baselineJSON(t, "input-"+tc.name, result)
		})
	}
}

// Check the original independent wire projections before changing the one allowed
// dynamic field. All other text and structured fields remain authoritative.
func normalizeSearchSnapshot(result *mcp.CallToolResult) error {
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	var payload, textPayload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &textPayload); err != nil {
		return err
	}
	if !reflect.DeepEqual(payload, textPayload) {
		return fmt.Errorf("text and structured projections differ")
	}
	stamp, ok := payload["snapshot_at"].(string)
	if !ok {
		return fmt.Errorf("snapshot_at missing")
	}
	if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		return err
	}
	payload["snapshot_at"] = "2026-10-01T00:00:00Z"
	textPayload["snapshot_at"] = "2026-10-01T00:00:00Z"
	b, err = json.Marshal(textPayload)
	if err != nil {
		return err
	}
	result.StructuredContent = payload
	result.Content[0].(*mcp.TextContent).Text = string(b)
	return nil
}

func TestMigrationProjectionRegressionCannotBeNormalizedAway(t *testing.T) {
	// Arrange: the timestamp is valid but the text silently lost coverage.
	result := &mcp.CallToolResult{
		StructuredContent: map[string]any{"snapshot_at": "2026-10-01T00:00:00Z", "coverage": []any{}},
		Content:           []mcp.Content{&mcp.TextContent{Text: `{"snapshot_at":"2026-10-01T00:00:00Z"}`}},
	}
	original := result.Content[0].(*mcp.TextContent).Text
	// Act
	err := normalizeSearchSnapshot(result)
	// Assert
	if err == nil || result.Content[0].(*mcp.TextContent).Text != original {
		t.Fatal("text regression was hidden")
	}
}
