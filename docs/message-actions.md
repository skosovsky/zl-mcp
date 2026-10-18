# Explicit read receipts and own reactions

These capabilities change Zalo state only on a user's explicit instruction for
that action. Reading messages, searching, browsing archives, receiving Events and
acknowledging the event journal never mark messages read or react to them.
Enabling a server permission grants capability; it is not an automation rule.

## Tools

| Tool | Request | Effect |
| --- | --- | --- |
| `zalo_mark_messages_read` | Exact `conversation_type`, `conversation_id`, 1–50 distinct `message_ids`, UUID `request_id` | Send receipts for retained incoming messages with observed native metadata |
| `zalo_set_message_reaction` | Exact typed chat, one `message_id`, `action=set` with `reaction`, or `action=remove` without it, UUID | Clear own reaction state, then optionally add the chosen reaction |
| `zalo_get_message_action_status` | Original `request_id` | Read saved state; no dispatch, retry or resume |

Read the finite codes from `zalo://catalog/reactions` using `resources/read`.
Each entry has `code`, `name`, and `display`. Heart and like have confirmed emoji
representations; other entries use names rather than guessed Unicode mappings.
The catalogue describes 54 upstream encodings; native acceptance of every icon is
not claimed. Unknown or ambiguous user choices require clarification, not a
closest-match replacement.

Resolve the exact typed chat and retained message IDs with ordinary MCP reads
before writing. Names are not write identifiers. The service retrieves private
native fields itself. It never accepts ST/AT/CMD, native reaction type/source,
credentials or raw payloads from the agent. Archive rows and messages lacking
verified metadata are unavailable targets; no phone synchronization is started.
Read receipts reject outgoing messages. Reactions may target incoming or outgoing
retained messages; activities, call records and reaction observations are not
message targets.

## Read scope and evidence

A receipt can affect the chat read watermark, including earlier messages. Do not
promise isolation to the listed IDs, all-device unread counters, or sender UI
confirmation from the API result alone. The direct native case confirmed the
first targeted message becoming read. The second was read after opening the chat,
so it is not evidence for or against a broader watermark effect.

A present native zero status is classified `accepted` on direct and group routes,
based on approved sender-visible native cases. Missing, null and other statuses
are `unknown`, retaining the numeric status when present. Historical unknown records
are never retrospectively changed by a new classification rule. See the living
[acceptance record](message-actions-acceptance.md) for tested routes and remaining
cases.

## Set, replace and remove

`set` always selects a persisted `clear_add` plan: remove the account's existing
reaction state, then add the chosen code. This avoids treating the upstream's
additive API as an atomic set. `remove` selects `clear`. The model cannot choose
raw plans or protocol values through these tools. Own reactions are distinct from
other participants' reactions; native coverage is listed in the acceptance record.

Two calls are not atomic. If clearing succeeds and adding is rejected, the result
is `partial`, and the message may have no own reaction. If a response is lost, the
result is `unknown` even if an earlier step succeeded. No background worker
continues or retries it. Inspect the original UUID, then obtain an explicit user
instruction before any separate corrective operation.

## Permissions

Configure the two independent exact-chat allowlists in the existing config:

```toml
[permissions]
# Preserve existing allow_join / allow_send / send_recipient_ids settings.
read_receipt_chats = [{ type = "direct", id = "EXACT_PEER_ID" }]
reaction_chats = [{ type = "group", id = "EXACT_GROUP_ID" }]
```

Both lists default to empty. Collection access is also required. Apply config
changes with the normal service restart; preserve the existing session and state.
Do not enable sending text to enable these capabilities. Status remains readable
after removing a write grant or losing authentication, while the chat is still
allowed by the collection policy for the current account.

HTTP and the STDIO bridge expose the same tools through the single running
service. Refresh the client's discovered catalogue after a backend update; a
schema cached by a client does not prove the running binary is current.

## Durability and failures

Save one stable UUID for the exact user-authorized operation. Same UUID and same
arguments return the saved result without repeating attempted native steps.
Changed arguments return `REQUEST_CONFLICT`. The UUID is shared across both kinds
of action. A busy chat can leave a request `pending`; it has not been dispatched,
and there is no background executor. Each native step is checkpointed before
sending. An interrupted dispatch becomes `unknown` on restart.

States are `pending`, `dispatching`, `accepted`, `rejected`, `partial`, `unknown`.
`accepted` means upstream acceptance of the plan, not independent UI/callback
confirmation. `completed_steps`, `stage`, `reason` and nullable `upstream_status`
preserve evidence. `completed_steps` counts confirmed upstream calls, not messages
read. Read batches have one aggregate native status; individual sender-visible
outcomes are not available from that response. `retry_safe=false` prohibits blind native resending with a new
UUID; querying or replaying the original UUID cannot repeat an attempted step.

`ACTION_METADATA_UNAVAILABLE` means the exact retained target lacks required
provenance; `PERMISSION_DENIED` means the exact chat/action is not allowed.
`NOT_FOUND` hides unavailable/out-of-policy operation records.
`CAPACITY_EXCEEDED` rejects new intents when the bounded ledger is full; old UUIDs
are not silently evicted and reused. A storage error after possible dispatch is
not proof that Zalo was unchanged. Read the saved status before any correction.

This feature does not automatically mark incoming messages read, manage manual
unread flags, or subscribe to other users' read receipts. No model eval result is
claimed for these new actions.
