# zl-mcp

Historical MVP snapshot: Go MCP tools for a personal Zalo account, selected-group collection, SQLite search, context and locally approved joins.

Build from a complete checkout with `go build -o bin/zl-mcp ./cmd/zl-mcp`. Schemas are in docs/contracts, synthetic test fixtures in docs/evals, and the Zalo dependency patch in third_party/zcago. Private sessions, configuration and collected messages are excluded.

This commit predates the unified service. Use the current branch documentation for installation and transport support.
