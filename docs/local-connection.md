# Connect an MCP client

First install and start the service using the [macOS guide](mac-deployment.md). The client and service must run as the same local OS user for the documented STDIO setup. Use absolute paths; replace `/Users/YOUR_USER` in every example. The client does not start the service or perform QR login.

## STDIO: tools and resources

For Codex, add this to its MCP configuration:

```toml
[mcp_servers.zalo]
command = "/Users/YOUR_USER/.local/bin/zl-mcp"
args = ["-config", "/Users/YOUR_USER/.config/zl-mcp/config.toml", "serve"]
```

For a client using `mcpServers` JSON:

```json
{
  "mcpServers": {
    "zalo": {
      "command": "/Users/YOUR_USER/.local/bin/zl-mcp",
      "args": ["-config", "/Users/YOUR_USER/.config/zl-mcp/config.toml", "serve"]
    }
  }
}
```

The bridge reads the existing private token file and connects to the loopback service. It creates neither a collector nor a database. An absent service or token causes an explicit startup error. Reconnect STDIO clients after restarting the service. STDIO exposes tools and resources, including full-message reads; it does not advertise Events.

## Streamable HTTP: tools, resources, and Events

Configure the client's Streamable HTTP transport with:

| Setting | Value |
| --- | --- |
| URL | `http://127.0.0.1:18765/mcp` |
| Header | `Authorization: Bearer TOKEN` |
| Token source | `/Users/YOUR_USER/.local/share/zl-mcp/mcp-token` |

The service generates the token privately on first startup. Transfer its value only into the client's protected settings. Do not paste it into documentation, issues, logs, or versioned configuration. Header syntax varies by client; use its supported bearer-token configuration. The endpoint binds only to a literal loopback IP; remote access requires separately managed infrastructure.

Events require a client that implements the pinned `2026-07-28` webhook profile, including discovery, `events/list`, `events/subscribe`, and `events/unsubscribe`. The callback must be publicly reachable HTTPS; private and loopback addresses are rejected even for a local client. The receiver verifies the challenge and delivery signatures. Delivery acknowledgment is distinct from running an agent or sending a notification. See the [Events contract](contracts/events.md); compatibility with any particular agent is not implied by ordinary MCP tool support.

## Verify the connection

1. Call `zalo_get_status` with no arguments. A signed-in, connected collector should report `authenticated=true` and `collector_state=connected`.
2. Call `zalo_list_groups` and select the IDs you want to collect. Add them to `collection.group_ids` in the private config and restart the service.
3. After a message arrives in a selected group, search for a distinctive word with `zalo_search_messages` and inspect a result with `zalo_get_message_context`.
4. Read `zalo://capabilities` for current capabilities and `zalo://events/diagnostics` for delivery queue diagnostics.

An empty allowlist means no collection. An empty corpus is not a failed MCP connection. During `auth_required` or reconnection, saved messages remain readable; collection resumes only when the upstream is available. Status and gap information describe observed coverage, not a complete Zalo archive.

Optional [skills](skills.md) provide research and event-processing instructions after the relevant client capabilities are available.
