package mcpserver

import (
	"encoding/json"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/buildinfo"
)

// normalizeBuildSnapshot makes checked-in compatibility snapshots independent
// of the checkout revision. Runtime tests assert the unmodified identity.
func normalizeBuildSnapshot(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	normalizeBuildSnapshotValue(doc)
	return doc
}
func normalizeBuildSnapshotValue(value any) {
	switch node := value.(type) {
	case map[string]any:
		normalizeErrorText(node)
		if meta, ok := node["_meta"].(map[string]any); ok {
			if server, ok := meta["io.modelcontextprotocol/serverInfo"].(map[string]any); ok {
				server["version"] = "build-identity-tested-separately"
			}
			if _, ok := meta[buildinfo.MetadataKey]; ok {
				meta[buildinfo.MetadataKey] = map[string]any{
					"backend": map[string]any{"version": "unknown", "revision": "unknown", "dirty": nil},
				}
			}
		}
		if details, ok := node["details"].(map[string]any); ok {
			if _, exists := details["correlation_id"]; exists {
				details["correlation_id"] = "correlation-tested-separately"
			}
		}
		for _, child := range node {
			normalizeBuildSnapshotValue(child)
		}
	case []any:
		for _, child := range node {
			normalizeBuildSnapshotValue(child)
		}
	}
}

// Tool error JSON is also serialized inside its text content block.
func normalizeErrorText(node map[string]any) {
	text, ok := node["text"].(string)
	if !ok {
		return
	}
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		return
	}
	if _, hasCode := body["code"].(string); !hasCode {
		return
	}
	details, ok := body["details"].(map[string]any)
	if !ok {
		return
	}
	if _, exists := details["correlation_id"]; !exists {
		return
	}
	details["correlation_id"] = "correlation-tested-separately"
	raw, err := json.Marshal(body)
	if err == nil {
		node["text"] = string(raw)
	}
}
