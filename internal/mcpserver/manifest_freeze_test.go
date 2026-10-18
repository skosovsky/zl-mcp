package mcpserver

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Freeze updates executable descriptors but retains explicit human-reviewed
// ownership/effect metadata. New tools require that metadata before freezing.
func freezeManifestTools(t *testing.T, body []byte, tools []*mcp.Tool) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	previous := map[string]map[string]any{}
	for _, value := range document["tools"].([]any) {
		tool := value.(map[string]any)
		previous[tool["name"].(string)] = tool
	}
	updated := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		metadata, exists := previous[tool.Name]
		if !exists {
			t.Fatalf("add reviewed manifest ownership metadata before freezing %s", tool.Name)
		}
		raw, err := json.Marshal(tool)
		if err != nil {
			t.Fatal(err)
		}
		var descriptor map[string]any
		if err := json.Unmarshal(raw, &descriptor); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"handler", "permissions", "effects", "operation_owner", "transports", "error_shape", "byte_limit", "normalization", "deadline", "admission"} {
			descriptor[field] = metadata[field]
		}
		updated = append(updated, descriptor)
	}
	document["tools"] = updated
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile("../../docs/mcp-migration/manifest.json", body, 0644); err != nil {
		t.Fatal(err)
	}
	return body
}
