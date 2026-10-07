# MCP discovery diagnostics

Authenticated, valid JSON-RPC POST discovery requests emit `mcp_discovery` in the
existing service JSON log. Its executable contract is
[`mcp_discovery_log.output.json`](mcp_discovery_log.output.json). This is transport evidence, not client installation
or schema adoption evidence. No request headers, parameters, client names,
credentials, resources, conversations or response bodies are logged.

Allowed methods: `initialize`, `server/discover`, `tools/list`, `resources/list`,
`resources/templates/list`, `events/list`. Fields: fixed method, HTTP status,
elapsed milliseconds, response byte count and protocol header classified as
`2026-07-28`, `2025-11-25`, `2025-06-18`, `2024-11-05`, `absent` or `other`.
Unauthenticated, invalid JSON-RPC and other methods emit no discovery record.

For a successfully decoded `tools/list` response of at most 1 MiB, also emit
`tool_count`, `archive_inventory_present`, `has_more` and `catalogue_sha256`.
The hash covers the tool definitions only, never errors or request fields.
Incomplete or oversized capture emits no catalogue claims. Responses are forwarded
unchanged; capture is bounded and never buffers message-reading results.

Compare these records with a client refresh observation. A successful server
response does not prove that the client replaced its cached catalogue. An absent
record only proves that this process did not record that authenticated discovery
request; it does not identify the external failure stage.
