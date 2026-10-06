# zl-mcp

A Go MCP service for a personal Zalo account: collect incoming and self-sent messages from accessible direct chats and groups, search a local SQLite corpus, read message context, send explicitly authorized direct text, and join groups through invitations with local approval.

One persistent `service` process owns the Zalo listener, SQLite database, MCP endpoint, subscriptions, and delivery queue. An optional `serve` process bridges STDIO clients to that service. SQLite is built in; CGO and Python are not runtime dependencies.

## Getting started

Source installation is supported on macOS. Install Go **1.26.5** or a compatible newer version, then:

```sh
git clone https://github.com/skosovsky/zl-mcp.git
cd zl-mcp
go build -o bin/zl-mcp ./cmd/zl-mcp
```

Follow [macOS installation and operation](docs/mac-deployment.md) to install the binary outside your checkout, create private configuration and storage, sign in through a local QR code, and set up launchd. The example uses explicit `all` mode to collect accessible direct chats and groups. For a restricted installation, choose `selected` mode with typed conversation IDs before starting the service. See [conversation collection](docs/conversations.md).

Build from the complete checkout. The project uses a local replacement for patched `zcago`; `go install ...@latest` is not the installation path. This publication distributes source, without release binaries.

## Connecting a client

| Transport | Capabilities | Requirements |
| --- | --- | --- |
| Streamable HTTP at `http://127.0.0.1:18765/mcp` | Tools, resources, MCP Events | Running service and bearer token |
| STDIO through `zl-mcp -config CONFIG serve` | Tools and resources | Running service and existing token file |

See [client connection examples](docs/local-connection.md). Connecting a client does not start the collector or sign in to Zalo.

General read tools: `zalo_list_conversations`, `zalo_get_conversation`, `zalo_search_conversation_messages`, `zalo_get_conversation_message_context`, `zalo_list_conversation_messages`. Legacy group and membership tools remain compatible: `zalo_get_status`, `zalo_list_groups`, `zalo_get_group`, `zalo_inspect_invite`, `zalo_join_group`, `zalo_get_join_status`, `zalo_search_messages`, `zalo_get_message_context`. Resources include `zalo://capabilities`, `zalo://collection`, `zalo://events/diagnostics`, and full-message URIs returned by the server.

For joining, inspect an invitation, approve its preview in an interactive local terminal with `zl-mcp -config CONFIG approve-join PREVIEW_ID`, then call `zalo_join_group` with the resulting token and a UUID request ID. Reuse the original arguments when retrying; check `zalo_get_join_status` rather than issuing another join for an uncertain result. Joining does not add the group to a selected collection policy; all mode admits accessible discovered groups.

## Browsing and available history

Use `zalo_list_conversation_messages` to page through one typed conversation over
an explicit `[since, until)` interval without a search word. Its cursor binds the
filters and a local snapshot. It reads collected data, not Zalo history.

For explicitly requested historical recovery, start
`zalo_import_conversation_history` with an exact conversation identity, RFC3339
interval and stable UUID request ID. Poll `zalo_get_history_import_status`; use
`zalo_cancel_history_import` to stop it. The single service imports available
group-cloud pages within page, record and time limits. Explicit
`source=conversation_preload` imports a bounded available snapshot for a typed
direct/group dialogue and reports `partial/source_window_limited`; deeper
direct/Strangers history remains unverified. Imported records appear in ordinary reads without Events
or notifications; later duplicates do not notify either. Completion describes
the available bounded source, never a complete archive. Inspect counts, nullable
filtering evidence, stop reason and newly imported time bounds. See the
[history workflow](docs/history-import.md) and [operation contract](docs/contracts/history-import-operation.md).

## Events and limitations

Events use a pinned webhook draft profile, requiring a compatible HTTP client and a public HTTPS receiver with verification and signature support. STDIO does not deliver Events. The service rejects loopback and private callback addresses. Tunnels, reverse proxies, receiver infrastructure, and agent notification delivery are outside the application.

`zalo.conversation.message.created.v2` version 2 supports exact/all/direct/group scopes and includes typed conversation identity, an available name, the message ID, author, time, and up to 2048 Unicode code points of text. Truncated text has a flag and a resource URI for the full record. By default, subscriptions begin with the first insertion after activation and remain active until cancelled; queued delivery survives restarts, within documented retry and capacity limits. See the [Events contract](docs/contracts/events.md).

The legacy group-only event `zalo.message.created` retains its original payload and subscription scope. Collecting all conversations does not widen callback subscriptions.

`zalo.conversation.message.created.v2` preserves these scopes and adds incoming/outgoing
filters and first locally known incoming evidence. Unknown historical evidence is
not a confirmed new correspondent. Existing profiles remain unchanged.

For direct text, use `zalo_send_direct_message` with an exact recipient ID, stable
UUID request_id and optional reply_to_message_id. Sending defaults to disabled;
configure allow_send separately from collection. Read `zalo_get_send_status` after
an uncertain outcome. Never create a new request to retry unknown. Quotes require
retained source text and protocol metadata from the same direct conversation.
See [direct messaging](docs/direct-messaging.md).

This service uses unofficial Zalo APIs, which can change or result in account restrictions. It does not search for public groups, fetch a complete historical archive, send attachments or group messages, or administer groups. Collection depends on the machine being awake and connected; `connected` does not prove corpus completeness, and upstream replay is bounded and can be empty. Local message buffers apply cancellable backpressure; successful enqueue does not prove persistence or a complete upstream archive. Opening Zalo Web can interrupt the account's web listener.

One account uses one state directory. Private file permissions restrict local access but do not encrypt stored messages or credentials. The local OS user is the trust boundary for CLI approvals.

## Documentation and development

- [Installation, storage, lifecycle, updates, and recovery](docs/mac-deployment.md)
- [HTTP and STDIO connections](docs/local-connection.md)
- [Optional agent skills and their boundaries](docs/skills.md)
- [Technical specification](docs/technical-spec.md) and [service contract](docs/contracts/service.md)
- [Conversation collection, compatibility and recovery](docs/conversations.md) and [extension task](docs/task-all-conversations.md) and [acceptance evidence](docs/all-conversations-acceptance.md)
- [Planned incoming-message filters and direct messaging](docs/task-direct-messaging.md)
- [Remaining acceptance, Strangers discovery, and conversation browsing/history task](docs/task-conversation-completion.md)
- [Development and checks](docs/development.md)
- [Historical records](docs/archive/README.md)
- [Third-party licenses](THIRD_PARTY.md)
- [Upstream patch provenance](third_party/zcago/PATCHES.md)

```sh
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/zl-mcp-darwin-arm64 ./cmd/zl-mcp
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/zl-mcp-linux-amd64 ./cmd/zl-mcp
```

Automated tests use synthetic data and fake upstreams. Linux tests and builds are covered by CI; Linux service deployment is manual and has no supported systemd setup. Default log paths are currently macOS-style. Agent skills are optional instructions, not receivers or transport implementations.

Licensed under [MIT](LICENSE). The vendored dependency retains its own license and patch provenance.

Event-triggered clients can recover missing payloads through the [subscription journal](docs/contracts/event-recovery.md), with an explicit durable acknowledgement cursor. Conversation Events v1 was removed; v2 and the legacy group profile remain.
