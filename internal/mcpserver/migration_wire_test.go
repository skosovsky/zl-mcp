package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestMigrationJSONRPCIdentityBaseline(t *testing.T) {
	for _, tc := range []struct{ name, id string }{
		{"string-large", `"9007199254740993123"`},
		{"numeric-large", `9007199254740993`},
		{"numeric", `42`},
		{"null", `null`},
		{"missing", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			store, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "messages.sqlite"), nil, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			token := strings.Repeat("synthetic", 8)
			service, err := NewHTTP(store, t.TempDir(), token)
			if err != nil {
				t.Fatal(err)
			}
			id := ""
			if tc.id != "" {
				id = `,"id":` + tc.id
			}
			body := `{"jsonrpc":"2.0"` + id + `,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
			request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("MCP-Protocol-Version", "2026-07-28")
			request.Header.Set("Mcp-Method", "tools/list")
			recorder := httptest.NewRecorder()
			// Act
			service.ServeHTTP(recorder, request)
			// Assert: capture actual behavior, including pinned SDK limitations.
			var response map[string]any
			var nonJSON string
			if recorder.Body.Len() > 0 {
				decoder := json.NewDecoder(bytes.NewReader(recorder.Body.Bytes()))
				decoder.UseNumber()
				if err := decoder.Decode(&response); err != nil {
					if strings.Contains(recorder.Header().Get("Content-Type"), "application/json") {
						t.Fatal(err)
					}
					nonJSON = recorder.Body.String()
				}
			}
			_, hasID := response["id"]
			_, hasResult := response["result"]
			baselineJSON(t, "rpc-id-"+tc.name, map[string]any{"status": recorder.Code, "id": response["id"], "has_id": hasID, "has_result": hasResult, "error": response["error"], "non_json_body": nonJSON})
		})
	}
}
