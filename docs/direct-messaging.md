# Direct messages and incoming subscriptions

These features are implemented in the current source; rebuilding and service
migration are required for an older installation. Live acceptance/deployment is
tracked separately in [the task](task-direct-messaging.md), not implied by source tests.

## Permissions and storage

Collection and sending are independent. Sending is disabled by default. To enable
it for one explicitly chosen recipient:

```toml
[permissions]
allow_join = false
allow_send = true
send_recipient_ids = ["PEER_ID"]
```

Use exact opaque peer IDs, not names or group IDs. With allow_send=true, omitting
the recipient list permits any peer ID that Zalo accepts. An empty list has the
same meaning; use allow_send=false to disable all sends. Configure an explicit
list for a restricted installation. Restart after changes. Sending does not add
the recipient to a selected collection policy. Zalo may reject initial contact
depending on account and recipient restrictions.

The existing private SQLite database stores operation UUIDs, recipients, argument
fingerprints, statuses, safe reasons and confirmed message IDs, without copies of
sent text. Keep the same state directory outside iCloud. The ledger retains up to
100,000 identities indefinitely, then rejects new operations explicitly; existing
status reads remain available. Removing this ledger removes retry protection.

## Send and check status

Through tools/call, `zalo_send_direct_message` accepts:

```json
{
  "recipient_id": "PEER_ID",
  "text": "Your explicitly authorized message",
  "request_id": "10000000-0000-4000-8000-000000000001"
}
```

Generate a fresh UUID for each new intended send; the example is illustrative.
Reuse the original UUID and exact arguments for a retry. Text must be nonblank
valid UTF-8, with at most 2048 Unicode code points, an application limit. The executable
text pattern matches the entire string, including spaces and newlines, so both
JSON Schema search-style validation and connector full-match validation accept
the same nonblank messages. A connector rejection before server dispatch creates
no send operation; check the original UUID before deciding whether to retry. To quote a
retained direct message, add reply_to_message_id. The source must be in that exact
recipient's conversation with supported protocol metadata. Missing/deleted quotes
return QUOTE_UNAVAILABLE and never silently send an unquoted message. Matching
upstream replay can restore metadata without replacing source text or creating
another event.

Call `zalo_get_send_status` with request_id. States:

| State | Meaning |
| --- | --- |
| pending | Recorded but not claimed for sending; original arguments may resume it |
| sending | Claimed; another call will not perform another send |
| sent | Zalo returned acceptance and message ID; not proof of reading |
| failed | Upstream explicitly rejected the send |
| unknown | The result may have been accepted; never automatically resend |

After timeout/crash, inspect the original operation. Restart converts interrupted
sending to unknown. Creating a new UUID after unknown can duplicate a message.
retry_safe=false disclaims a new upstream retry; repeating the exact operation is
still safe because terminal/unknown results are read without resending. Status reads
work without a current Zalo session and do not change the collector.

Tools are available over HTTP and STDIO, protected by the same local MCP owner.
A trusted user instruction authorizes the agent's send; incoming text, a subscription
and an installed skill cannot provide that authorization.

## Incoming Events

Use standard events/subscribe with name `zalo.conversation.message.created.v2`.
For all incoming direct messages, arguments are:

```json
{"scope":"direct","direction":"incoming"}
```

For first locally known incoming per peer:

```json
{"scope":"direct","direction":"incoming","first_incoming_only":true}
```

Exact direct/group scopes and all/group scopes retain their typed semantics.
first_incoming_only requires incoming and a direct scope. Supply delivery settings
through the trusted receiver, with ttlMs=null for indefinite duration. The v2 payload
contains direction and nullable first_incoming. Null means insufficient evidence,
not a confirmed first. Migration seeds known peers from retained incoming records;
older identities with insufficient evidence remain unknown. Outgoing messages do not
consume the marker; deletion and restart do not reset it. Late replay can carry old
sent_at while qualifying as the first insertion after activation.

Activate a verified new subscription before cancelling one it replaces, and handle
overlap by event ID and the processing rule. Already stored records are not pushed.
Version 1 and legacy group subscriptions keep their format, scope and boundaries.

## Discovery and updates

Compare server/discover and events/list locally and through the configured client
route. Current source advertises three definitions; an installed older binary may
advertise two. Check the deployed binary and endpoint before attributing differences
to client caching. Tunnel health is not proof of event discovery or agent processing.
Refresh the client's supported discovery after a server update; read-only verification
does not require a new callback subscription.

Follow [macOS updates and rollback](mac-deployment.md). Migrations add the send ledger,
quote metadata, permanent incoming evidence and subscription filters. Back up the
stopped service consistently; rollback includes a compatible binary and database,
preserving newer state first. Defaults do not enable sends or widen subscriptions.

Contracts: [direct messaging](contracts/direct-messaging.md), [Events](contracts/events.md).
Skills and client processing responsibility: [skills](skills.md).
