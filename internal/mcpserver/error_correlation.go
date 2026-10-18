package mcpserver

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Correlation is transport diagnostics: it does not mutate shared domain errors
// or add private error formatting to the application result.
func failureWithCorrelation(err error, id string) *mcp.CallToolResult {
	result := failure(err)
	if len(result.Content) != 1 {
		return result
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return result
	}
	var value map[string]any
	if json.Unmarshal([]byte(text.Text), &value) != nil {
		return result
	}
	details, _ := value["details"].(map[string]any)
	if details == nil {
		details = map[string]any{}
	}
	details["correlation_id"] = id
	value["details"] = details
	body, encodeErr := json.Marshal(value)
	if encodeErr == nil {
		text.Text = string(body)
	}
	return result
}
