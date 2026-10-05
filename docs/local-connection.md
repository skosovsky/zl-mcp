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
2. Read `zalo://collection` to confirm the mode and counts by type. Call `zalo_list_conversations` for discovered dialogues; `zalo_list_groups` remains a group catalogue view. Configure explicit `collection.mode = "all"` or typed selected entries as described in [conversation collection](conversations.md), then restart the service.
3. Search a distinctive word with `zalo_search_conversation_messages` and inspect a hit with `zalo_get_conversation_message_context`, preserving both its conversation type and ID. Legacy `zalo_search_messages`/`zalo_get_message_context` remain group-only. An old sent_at does not exclude a record recovered on first insertion after subscription activation.
4. Read `zalo://capabilities` for current capabilities and `zalo://events/diagnostics` for delivery queue diagnostics.

An empty selected policy means no collection; `all` dynamically includes newly discovered conversations. An empty corpus is not a failed MCP connection. During `auth_required` or reconnection, saved messages remain readable; collection resumes only when the upstream is available. Status and gap information describe observed coverage, not a complete Zalo archive.

Optional [skills](skills.md) provide research and event-processing instructions after the relevant client capabilities are available.

## Refresh ChatGPT plugin discovery after an update

For a personal developer-mode plugin, OpenAI documents a separate metadata
refresh: after deploying the server, open the existing connection at ChatGPT
Plugins, select **Refresh**, confirm the changed tools/events, then test in a new
conversation with that plugin enabled. See
[Connect and test your plugin](https://developers.openai.com/plugins/deploy/connect-chatgpt).
Do this in the account/workspace that owns the existing Zalo connection; a
different browser account can have an empty personal catalogue.

Compare three distinct observations:

| Layer | Evidence |
| --- | --- |
| Running service | Authenticated `server/discover`, `tools/list`, and `events/list` on its actual HTTP endpoint |
| Plugin metadata | Expected tools and Events visible after Refresh on that connection's page |
| Client conversation | A new enabled conversation actually exposes/calls the expected tools and can subscribe through the standard Events lifecycle |

A working status tool or healthy tunnel proves neither a complete client
catalogue nor Events acceptance. A mismatch alone does not establish a cache
defect. Refresh preserves the connection; do not recreate subscriptions or
restart the collector merely to test tool discovery. Published plugins have a
different reviewed tool-update flow; the personal developer-mode instructions
do not prove their updates are approved.

## Conversation Events

Discovery includes the additive `zalo.conversation.message.created` version 1 and unchanged group-only `zalo.message.created`. Subscribe with `arguments: {"scope":"all"}`, `{"scope":"direct"}`, `{"scope":"group"}`, or `{"scope":"conversation","conversation_type":"direct","conversation_id":"PEER_ID"}`. Supply the standard delivery settings through the trusted receiver; scopes are always limited by the current collection policy. Existing group subscriptions do not expand after a collection-mode change.

The new payload includes typed identity, message ID, author, time, at most 2048 Unicode code points, and a full-text URI when truncated. Both profiles use the same verification, signature, persistence, retry and cancellation rules in the [Events contract](contracts/events.md). Receiver acceptance and an agent notification remain separately verifiable outcomes.

For migration, activate the new all-scope subscription with `ttlMs: null`, then
cancel the legacy group subscription by its ID. Obtain delivery settings from the
trusted client integration rather than copying credentials into documentation or
chat. Handle overlapping group deliveries by event ID and the agreed processing
rule. Previously stored messages remain available through reads and are not sent
automatically. See [subscription migration](conversations.md#replacing-a-legacy-group-subscription).

Current source also advertises `zalo.conversation.message.created.v2`: direction
filters and first_incoming_only for direct incoming scopes. Version 1 remains unchanged.
See [incoming subscriptions and direct sending](direct-messaging.md) for examples,
discovery verification, separate send permissions and handling an unknown send result.
