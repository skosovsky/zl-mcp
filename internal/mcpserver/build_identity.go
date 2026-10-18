package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/buildinfo"
)

// backendImplementation keeps application identity separate from protocolVersion.
func backendImplementation() *mcp.Implementation {
	//nolint:exhaustruct_v5 // Optional visual metadata is not part of build identity.
	return &mcp.Implementation{Name: "zl-mcp", Version: buildinfo.Current().DisplayVersion()}
}

// withBuildIdentity uses MCP metadata so existing strict successful/error output
// contracts and historical stored operation outcomes remain unchanged.
func withBuildIdentity(result *mcp.CallToolResult) *mcp.CallToolResult {
	if result.Meta == nil {
		result.Meta = make(mcp.Meta)
	}
	result.Meta[buildinfo.MetadataKey] = map[string]any{"backend": buildinfo.Current()}
	return result
}
