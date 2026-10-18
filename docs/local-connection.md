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

Events require a client that implements the pinned `2026-07-28` webhook profile, including discovery, `events/list`, `events/subscribe`, and `events/unsubscribe`. Public HTTPS callbacks are the default. A same-host receiver can use an exact operator-allowlisted HTTP/HTTPS loopback URL; see [local delivery configuration](contracts/local-callbacks.md). This option is disabled by default, requires a service restart after configuration changes and does not create the receiver. The receiver verifies the challenge and delivery signatures. Delivery acknowledgment is distinct from running an agent or sending a notification. See the [Events contract](contracts/events.md); compatibility with any particular agent is not implied by ordinary MCP tool support.

## Verify the connection

1. Call `zalo_get_status` with no arguments. A signed-in, connected collector should report `authenticated=true` and `collector_state=connected`.
2. Read `zalo://collection` to confirm the mode and counts by type. Call `zalo_list_conversations` for discovered dialogues; `zalo_list_groups` remains a group catalogue view. Configure explicit `collection.mode = "all"` or typed selected entries as described in [conversation collection](conversations.md), then restart the service.
3. Search a distinctive word with `zalo_search_conversation_messages` and inspect a hit with `zalo_get_conversation_message_context`, preserving both its conversation type and ID. Legacy `zalo_search_messages`/`zalo_get_message_context` remain group-only. An old sent_at does not exclude a record recovered on first insertion after subscription activation.
4. Read `zalo://capabilities` for current capabilities and `zalo://events/diagnostics` for delivery queue diagnostics.

An empty selected policy means no collection; `all` dynamically includes newly discovered conversations. An empty corpus is not a failed MCP connection. During `auth_required` or reconnection, saved messages remain readable; collection resumes only when the upstream is available. Status and gap information describe observed coverage, not a complete Zalo archive.

Optional [skills](skills.md) provide research and event-processing instructions after the relevant client capabilities are available.

## Read a retained local archive

When `zalo_list_archive_sources` is discovered, select its immutable `source_id`.
Pass that ID to `zalo_list_conversations`, then to
`zalo_list_conversation_messages` with an exact conversation type/ID and explicit
RFC3339 `since`/`until` interval. Omitting `source_id` keeps normal corpus reads.
Follow cursors even when a page has no supported text but `has_more=true`.
Read truncated text using the server-issued `zalo://archives/<token>` resource
or `zalo_read_message_content` with that exact URI and its continuation cursor.
Archive rows have their own identity and cannot serve as outgoing quote anchors.

These reads work offline without a phone request, corpus import or Events.
The permanent library lives under `StateDir/account-archive-library/sources`
and remains until owner deletion. Capture provenance and examined-page coverage
do not establish complete account history. Unclassified controls block the
selected file explicitly. See [archive reading](contracts/archive-mcp-reading.md).

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

Discovery includes the additive `zalo.conversation.message.created.v2` version 2 and unchanged group-only `zalo.message.created`. Subscribe with `arguments: {"scope":"all"}`, `{"scope":"direct"}`, `{"scope":"group"}`, or `{"scope":"conversation","conversation_type":"direct","conversation_id":"PEER_ID"}`. Supply the standard delivery settings through the trusted receiver; scopes are always limited by the current collection policy. Existing group subscriptions do not expand after a collection-mode change.

The new payload includes typed identity, message ID, author, time, at most 2048 Unicode code points, and a full-text URI when truncated. Both profiles use the same verification, signature, persistence, retry and cancellation rules in the [Events contract](contracts/events.md). Receiver acceptance and an agent notification remain separately verifiable outcomes.

For an explicitly agreed migration, activate a subscription with the same
intended scope and `ttlMs: null`, then cancel the superseded subscription by its ID.
Do not widen a group/direct scope to all without the user instruction. Obtain delivery settings from the
trusted client integration rather than copying credentials into documentation or
chat. Handle overlapping group deliveries by event ID and the agreed processing
rule. Previously stored messages remain available through reads and are not sent
automatically. See [subscription migration](conversations.md#replacing-a-legacy-group-subscription).

Current source also advertises `zalo.conversation.message.created.v2`: direction
filters and first_incoming_only for direct incoming scopes. Conversation version 1
has been removed; the legacy group event remains supported.
See [incoming subscriptions and direct sending](direct-messaging.md) for examples,
discovery verification, separate send permissions and handling an unknown send result.

For event-triggered automation, store the exact subscription ID with its rule and use [journal recovery](contracts/event-recovery.md). Refresh the client/plugin catalogue to discover the three recovery tools after updating the service. Reconcile previously completed notifications before processing an existing backlog.

## Rich content discovery

After an upgrade, refresh the actual HTTP client's discovery
and inspect `events/list`: message v3 and reaction v1 require compatible schemas;
STDIO still provides tools/resources only. Do not replace existing v2
subscriptions just because v3 is listed. Preserve processing checkpoints and
reconcile source conversation/message identities before an agreed cutover.
Call observations use a narrow direct-chat profile; they do not establish call
outcome, mode, duration or a complete lifecycle. A missing automation payload is different from a typed
nontext event with empty author text. Read the exact subscription journal,
process in order, and acknowledge only the authorized successful action. See
[rich-content contract](contracts/rich-content-events.md). For the accepted rich-content scope and remaining upstream limits, see
[rich-content acceptance](rich-content-acceptance.md).

## Explicit reaction choices and read receipts

When the backend advertises `zalo://catalog/reactions`, read it through standard
`resources/read` over HTTP or STDIO to obtain finite reaction codes, names and display labels.
Listing choices sends no reaction and grants no write permission. Arbitrary
emoji and raw protocol type/source fields are not catalogue inputs. Discover
available write tools and their current schemas rather than assuming that a
catalogue entry proves replacement or removal semantics.

Receipt/reaction write tools have explicit input/output schemas and verified
native cases. See the [action guide](message-actions.md),
[task](task-mark-messages-read.md) and [current evidence](message-actions-acceptance.md).
Ordinary searches, context/resources, archive reads, Events and journal ack do
not change Zalo read state or reactions. These actions require a separate explicit
user instruction on exact targets; the current workflow does not automate them.
Journal acknowledgement confirms processing, independently of Zalo read receipts.

After upgrading, refresh the client/plugin tool catalogue to discover
`zalo_mark_messages_read`, `zalo_set_message_reaction` and
`zalo_get_message_action_status`. Existing registered clients can retain an older
catalogue even while server-side `tools/list` exposes the updated tools. Discovery
does not grant write access; read/react allowlists default to empty.

## Bounded tools-only reads and coverage

Discover `zalo_read_message_content` and use its current schema. Supply the exact
`resource_uri` returned by context/browse; never manufacture an archive token.
Pages default to 2048 Unicode scalar values (maximum 8192) and may be shorter to
fit the serialized budget. Repeat the same URI with `next_cursor` until
`has_more=false`. Cursors expire and are invalidated by a service restart;
retention, recall and permissions are rechecked on every page. `/content` metadata
URIs and media bytes are not text targets. See [contract](contracts/message-content-read.md).

When the advertised schema supports `coverage_mode`, pass `compact` to corpus
reads when detailed gap history is unnecessary. `coverage_details` explicitly
reports total gaps, truncation, snapshot and nullable cursor. For the complete
observed gap snapshot, discover `zalo_list_coverage_gaps` and paginate with the
exact conversation type/ID. Old clients continue using default full coverage;
loading the new output schema is required before opting into compact mode.
Archive coverage remains separate. See [coverage contract](contracts/coverage-pagination.md).

## Identify the running backend and correlate failures

Inspect the actual unnormalized tool result `_meta["zl-mcp/build"]`: `backend`
contains safe version/revision/dirty fields; STDIO also reports `bridge`.
`serverInfo.version` identifies the executable at that transport boundary, so
STDIO initialize alone identifies its bridge, not a potentially different backend.
Unknown metadata must not be interpreted as a clean/current build. Protocol
negotiation is independent. See [build contract](contracts/schema-publication-and-build.md).

For a failed tool call, retain its safe correlation reference and compare it with
service logs; do not include tokens or raw private payloads. For journal diagnosis,
opt into `include_diagnostics=true` on `zalo_read_subscription_events` after
loading the new schema. Default journal replies retain the old strict shape.
Reasons distinguish delivery failure categories without exposing callback URLs,
message IDs or signing values. Callback HTTP 2xx proves receiver acceptance,
not model payload visibility or notification completion.

## Client capability and evidence matrix

| Client/path | Tools | Resources | Events subscription methods | Agent continuation evidence |
| --- | --- | --- | --- | --- |
| HTTP SDK harness, negotiated 2026-07-28 for Events | Supported | Supported | Pinned webhook profile plus optional journal-ready companion | Signed synthetic receiver covers server continuation; actual target-host activation/Ann processing NOT_RUN |
| Native STDIO bridge and SDK client | Supported | Supported | Unavailable by design | Journal reads/ack supported; bridge does not wake an agent |
| Target ChatGPT/plugin connection | Must verify loaded catalogue | Host-dependent; bounded text tool is the alternative | Requires compatible plugin/receiver discovery | Do not infer from HTTP 2xx or ordinary tool support; final target-host recovery acceptance remains required |
| Other MCP clients | Verify current schemas | Verify host resource support | Unverified unless profile tested | Unverified unless exact host mechanism is tested |

These are capability boundaries, not a claim that every current client/version
has passed the new hardening acceptance. See the [task and final gate](task-mcp-skills-hardening.md)
for exact revision/test evidence. Refresh discovery before assuming that source
code availability means a host can call the new tools.

## Processing readiness before callback receipt

After a release update, first inspect the actual backend revision and loaded
`zalo_read_subscription_events` schema. If it advertises `readiness_mode`, use
`readiness_mode=persisted` for a trusted journal workflow. Retained valid events
can then be read/acked while their callback is pending/sending; delivery is never
reported as successful merely because processing completed. Omitted/default
`delivery_terminal` preserves the old representation. Empty/invalid/admission
payload gaps and inactive-generation restrictions remain explicit. Do not infer
this mode is installed from documentation alone. See the
[readiness contract](contracts/journal-processing-readiness.md).

## Optional durable journal continuation

The additional control profile `zalo.subscription.journal.ready.v1` attaches to
one existing owned business subscription and its exact generation. Discover the
current contract; request `zalo_list_event_subscriptions(include_recovery=true)`
for the trusted generation reference and configured mode. The trusted host
explicitly creates the companion using the normal verified delivery lifecycle
and binds business and control wakeups to the **same saved workflow**, scope,
rule and business-journal cursor. Do not recreate or widen the business
subscription and do not treat a control payload as a message or receipt.

The worker independently observes the unacknowledged head and issues bounded
control rounds; retry within a round keeps its event ID, later rounds have new
IDs. Persisted-mode reading does not wait for callback completion and needs no
companion to close that race. The companion is insurance for missing/interrupted
runs; bounded rereads in the legacy mode honor `retry_after_ms`, then await the
configured continuation. `include_diagnostics=true` exposes recovery
mode/state/reference/reason, including explicit cancellation or exhaustion. An
unsupported/unconfigured mode does not provide automatic continuation.

Synthetic signed-receiver scenarios now cover the server flow. Actual target-host
configuration, saved-task update and Ann control-wakeup processing remain
**NOT_RUN**, so do not declare that connection recovered merely from server tests
or a 2xx callback. See [canonical contract](contracts/subscription-recovery-ready.md)
and [separate skill/task migration](skills.md).
