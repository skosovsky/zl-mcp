# Recovering event payloads after an automation trigger

The callback receiver may accept an event and trigger an automation without
making its payload visible to the agent. Recovery reads the exact persisted
callback envelope, including its stable eventId, rather than inferring it from
recent messages. This application extension uses ordinary MCP tools; it does not
change MCP Events delivery, batching, or the receiver's model context.

## Tools and executable schemas

- `zalo_list_event_subscriptions {}` lists active subscriptions owned by the
  authenticated single-account transport. Output includes scope, direction and
  first-incoming filter, never callback URLs or secrets. Save the exact ID with
  the automation rule. If several subscriptions match, resolve the association
  before processing; do not select one by recency.
- `zalo_read_subscription_events {"subscription_id":"…","limit":20}` returns
  unacknowledged records in delivery order. Reads do not acknowledge anything.
  Every record has the exact callback `event` envelope or null, `delivery_state`,
  nullable `gap_reason`, and an opaque `receipt`. A response is bounded by both
  record count and byte budget; `has_more` requests another page after processing
  the current one. `blocked_on_delivery` means an earlier callback is still
  pending/sending; finish this run rather than polling indefinitely.
- `zalo_ack_subscription_events {"subscription_id":"…","receipt":"…"}` persists
  completion of that one record and its ordered prefix. Acknowledge receipts in
  page order, only after the authorized action succeeds or an irrelevant record
  or explicit gap has been accounted for. A repeated acknowledgement is safe.
  Out-of-order/stale receipts cannot skip unprocessed work; reread the journal.

Input/output contracts are the corresponding `.input.json` / `.output.json`
files. This is available through HTTP and the STDIO bridge of the running service.
Only HTTP has Events subscription methods.

## Processing and scope

Use the journal for event-triggered runs whether the initial payload is visible
or absent, so the same durable cursor handles both routes. If a visible event is
already acknowledged, do not notify from its payload again. A run can recover
several callbacks (including callbacks whose triggers were batched or missed).
Scope and direction are those captured by the subscription at fanout. Collection
policy and retained-message access are checked again on reads. Historical imports
never enter this journal. The first insertion after activation remains the event
boundary; event timestamp is not that boundary.

The cursor is stored in SQLite per subscription and generation. Refresh and
restart preserve it; cancellation and reactivation create a new generation and
reject old receipts. Existing retained callbacks are initially unacknowledged:
when adopting recovery for an existing subscription, explicitly reconcile the
backlog with known completed notifications before enabling the new rule. Do not
silently mark old callbacks processed or replay their notifications blindly.
Different automation rules need separate subscriptions/cursors. The existing
single-account transport identifies the owner as the local account; this is not
isolation between different clients sharing its credential.

## Retention, gaps and delivery guarantees

Delivered means HTTP acceptance, not agent processing. Pending/sending callbacks
block later journal records. Failed/cancelled terminal records expose a gap,
not an invented event. Completed delivery records retain their existing seven-day
lifetime. Pruning saves a durable gap watermark before deleting rows. If that
watermark exceeds processing progress, the next read returns an explicit
`retention_expired` gap with its own receipt. Deleted messages or revoked access
never return their former callback text. Full text for a clipped event uses its
normal resource URI and corpus retention policy.

Acknowledgement does not atomically send a ChatGPT notification. Concurrent runs
can perform the same external action before one acknowledges; a crash after
notification and before acknowledgement can also repeat it. Serial processing
and a channel-supported idempotency key based on subscription/generation/eventId
are needed to strengthen this guarantee. The durable cursor alone does not
provide exactly-once notifications. It does eliminate replay of acknowledged
records after restart and offers exact recovery within retention.

The workaround was inspired by a first-person Things/dot experiment published
on [4 October 2026](https://note.com/unco3/n/nb5426444e496?hl=en).
That report observed a startup without a immediately visible body; it did not
establish an official platform root cause. Our journal additionally persists
processing progress. It is not a fix to ChatGPT's payload injection.
