# Direct chats and groups

The service stores conversation identity as `(conversation_type, conversation_id)`.
Types are `direct` and `group`; IDs are opaque strings. A direct ID identifies the
peer for both incoming and self-sent messages. The author remains a separate ID.
Identical IDs in different types do not share messages, context, cursors or events.
One account owns one state directory; changing accounts cannot merge their data.

## Collection policy

To collect accessible direct chats and groups, including newly discovered ones:

```toml
[collection]
mode = "all"
```

For a selected scope:

```toml
[collection]
mode = "selected"
conversations = [
  { type = "direct", id = "PEER_ID" },
  { type = "group", id = "GROUP_ID" }
]
```

An empty selected list collects nothing. Existing configurations containing only
`group_ids` keep their group-only meaning; do not combine that legacy form with
`mode` or `conversations`. Restart the service after a policy change. The same
policy controls ingestion, reads and event delivery, including already stored data.
Collection mode does not create or widen callback subscriptions.

## Read interface

| Tool/resource | Purpose |
| --- | --- |
| `zalo_list_conversations` | Discovered catalogue, optional type/name filters and pagination |
| `zalo_get_conversation` | Typed metadata and local coverage |
| `zalo_search_conversation_messages` | Search permitted direct/group messages; optional type or exact dialogue |
| `zalo_get_conversation_message_context` | Context restricted to the exact typed dialogue |
| `zalo://collection` | Mode and permitted counts by type, catalogue/history completeness |
| Returned full-text URI | Read through standard `resources/read` |

Full-text URIs use `zalo://conversations/{type}/{escaped-id}/messages/{escaped-message-id}`.
Use the URI returned by the service, preserving escaping. Legacy group tools and
`zalo://groups/...` URIs remain group-only and keep their original response schemas.
Both HTTP and STDIO expose the read tools/resources; Events require HTTP.

## Discovery and recovery

The group API supplies group metadata. Incoming/replayed messages discover other
conversations transactionally. Inbound peer metadata can supply a direct-chat name;
self messages do not replace that name with the account owner's name. A friend list
is not a complete conversation catalogue. Unknown peers and new accessible groups
are admitted in all mode, even if no catalogue record existed beforehand.

After the listener receives its cipher key, it requests available group and direct
replay. Mixed responses preserve both categories. Live and replay deduplicate by
typed message identity; retention retains first-insertion identities so recovered
text does not create another message-created event.

Upstream replay is bounded and can be empty. No API in the pinned implementation
establishes a complete historical archive or full chat catalogue. Connecting or
receiving an empty replay does not close known gaps. Sleep, disconnection and
upstream withholding can leave unavailable messages. Catalogue and history
completeness remain explicitly false. Hidden, encrypted and special system-chat categories are unverified; messages exposed by the pinned direct/group protocol are the supported surface. These limits are also disclosed in zalo://capabilities. See [research evidence](conversation-research.md).

This distinguishes two stages: Zalo must first expose a message to the collector;
only then can the service persist it and enqueue a callback. Durable delivery does
not reconstruct messages that Zalo never returned. A recovered message is readable
through MCP search/context even when no subscription matched its first insertion.

## Events and compatibility

`zalo.conversation.message.created.v2` schema version 2 adds typed identity and an
available conversation name. Its scopes are all, direct, group or one exact typed
conversation. The legacy `zalo.message.created` retains its group-only scope and
payload, including across migration. Both profiles share signed verification,
2048-code-point text limits, full-text retrieval and durable cancellation/retry.

The first insertion after activation determines eligibility, even when sent_at is
older. Refresh/restart preserves the boundary. Delivery is ordered within each
subscription and limited by documented capacity/deadline; different subscriptions
do not form a global order. One journal event can match overlapping subscriptions;
its event ID remains stable. See the [executable Events contract](contracts/events.md).

### Replacing a legacy group subscription

Collection mode `all` and subscription scope `all` are separate settings. To receive
both direct and group events, the trusted client creates a new subscription with
`name: "zalo.conversation.message.created.v2"`, `arguments: {"scope":"all"}` and
`ttlMs: null`, using its own verified delivery settings. After successful activation,
cancel the old `zalo.message.created` subscription by its returned ID. During the
overlap, group events can arrive through both profiles; the receiver must apply its
deduplication rule. This transition does not deliver records already in the corpus
at activation. Read those through search/context when needed. Do not delete the old
subscription before the new receiver has passed verification.

The SQLite migration preserves corpus, session references, sequence high-water
marks, identities, gaps, subscriptions, generations, watermark and queued bytes.
Old records do not generate historical events. Always make a consistent private
stopped-service backup before deployment; rollback requires a compatible database
backup as well as the old binary. See [macOS lifecycle](mac-deployment.md).

## Incoming filters and direct sends

The new source implementation adds `zalo.conversation.message.created.v2`,
schema_version=2. Version 1 and the legacy group profile remain unchanged. Arguments
retain the same scopes and add direction (all/incoming/outgoing, default all) and
first_incoming_only (default false). First-only requires direct incoming scope,
including an exact direct conversation. It means first incoming insertion known to
this service, not first ever in Zalo. Unknown historical evidence is nullable and
does not match a positive first-only filter. Outgoing messages do not consume the
first-incoming marker; deletion/retention does not reset it.

`zalo_send_direct_message` and `zalo_get_send_status` provide explicit text sends and
retained-message quotes, protected by separate allow_send/send_recipient_ids settings.
See [the executable sending contract](contracts/direct-messaging.md). Unknown send
results must not be retried with a new UUID. These source changes require rebuilding
and migrating the service; they do not update an already installed binary or client
discovery automatically. Live acceptance is tracked in [the extension task](task-direct-messaging.md).

The service additionally refreshes the verified preload catalogue at session
startup and every five minutes. It merges permitted typed dialogue metadata,
including dialogues outside the friendship directory when returned by Zalo.
Absent entries are retained because each snapshot is partial. This does not
classify Strangers or prove a complete inbox. Source message previews are not
silently imported by catalogue refresh. Inspect the `preload` section of
`zalo://catalog/diagnostics` for attempt/success times, observed/permitted counts
and safe failure reasons; its completeness and message-import flags remain false.

For an explicitly requested import, `zalo_import_conversation_history` also
accepts `source=conversation_preload` for one typed direct/group dialogue. It
imports only a bounded currently available snapshot and always reports
partial/source_window_limited with history_complete=false. Source messages from
other conversations are not stored. Omitted source keeps the group_cloud
behaviour. Preload is not a replacement for deeper paginated direct history.
