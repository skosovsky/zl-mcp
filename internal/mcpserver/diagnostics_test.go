package mcpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestDeliveryDiagnosticsResourceThroughHTTP(t *testing.T) {
	// Arrange: authenticated real MCP transport, no session or callbacks.
	dir := t.TempDir()
	store, err := storage.Open(context.Background(), filepath.Join(dir, "messages.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := strings.Repeat("t", 64)
	handler, err := NewHTTP(store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(handler)
	defer endpoint.Close()
	// Act
	resource := httpRPC(t, endpoint.URL+"/mcp", token, "resources/read", map[string]any{"uri": "zalo://events/diagnostics"})
	content := resource["contents"].([]any)[0].(map[string]any)
	var wire map[string]any
	if err := json.Unmarshal([]byte(content["text"].(string)), &wire); err != nil {
		t.Fatal(err)
	}
	// Assert: exact executable contract with an empty queue.
	schema, err := contracts.Compile("events_diagnostics", "output")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(wire); err != nil {
		t.Fatal(err)
	}
	if content["mimeType"] != "application/json" || wire["active_subscriptions"] != float64(0) || wire["state_counts"].(map[string]any)["pending"] != float64(0) {
		t.Fatal("incorrect diagnostic resource")
	}
}
