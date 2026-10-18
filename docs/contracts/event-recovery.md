# Recovering event payloads after an automation trigger

A callback receiver may start an automation without exposing its payload to the
agent. The journal returns the exact retained envelope and stable eventId;
it never guesses an event from recent messages. These ordinary MCP tools are
available through HTTP and STDIO. Only HTTP has Events subscription methods.

The [processing-readiness contract](journal-processing-readiness.md) separates
three facts: retained envelope, HTTP delivery outcome and processing ack. The
preferred opt-in `persisted` view closes the callback/read race **without a new
subscription**. The optional companion remains insurance for missing or
interrupted runs, not the prerequisite for processing retained event data.

## Tools and compatible representations

- `zalo_list_event_subscriptions {}` lists the caller's active business
  subscriptions with scope/direction/first-incoming filters, never callback URLs
  or secrets. Save the exact ID with the workflow; resolve ambiguous matches
  rather than choosing by recency. Optional `include_recovery=true` adds
  generation/companion metadata for explicit companion setup only.
- `zalo_read_subscription_events` reads ordered unacknowledged records, without
  acknowledging them. Each record contains an exact envelope or explicit gap,
  truthful `delivery_state`, nullable `gap_reason` and opaque `receipt`.
- `zalo_ack_subscription_events {"subscription_id":"…","receipt":"…"}` advances
  that business subscription's processing checkpoint only after the authorized
  action succeeds or the irrelevant/gap record has been accounted for. Process
  page receipts in order. Repeated ack is safe; stale/out-of-order receipts do
  not authorize skipping work.

Select the new view only after loading a schema that advertises `readiness_mode`:

```json
{"subscription_id":"<exact-business-id>","limit":20,"readiness_mode":"persisted"}
```

In `persisted`, the response explicitly carries `readiness_mode=persisted` and
entry `processing_state=ready|gap`. A retained valid envelope is ready even when
HTTP delivery is pending, sending or failed. `ready` means non-null event and
null gap reason; `gap` means event=null and a reason. Transport state is never
renamed to delivered. `blocked_on_delivery=false` in this view; diagnostic
`retry_after_ms` is null. A failed delivery with retained envelope is processable;
an empty or invalid envelope is an explicit gap.

Omitted/default `delivery_terminal` preserves old keys and behavior for strict
clients: pending/sending can block; terminal failed/cancelled entries expose a
gap. These are views of **one** processing cursor, not independent consumers.
Switching readiness modes does not replay acknowledged records. Inputs, outputs,
limits and strict property rules are in the corresponding executable schemas.

## Early processing and transport remain independent

Count/byte budgets, order, receipts and `has_more` are unchanged. Process the
retained event before returning receiver HTTP 2xx when appropriate; do not wait
for a future delivery commit merely to read already committed data. Early ack
updates only processing progress. It neither claims HTTP acceptance nor marks a
Zalo message read and must not clear pending/sending bytes still owned by the
HTTP worker. That worker retains its lease, retry policy and original event
ID/body. Its later success, 503, timeout or restart cannot rewind the ack cursor.
A retry wakeup rereads the same journal and finds no already-acknowledged item.

Generation, principal, current access, recall, retention and schema checks still
apply. Persisted mode does not reopen cancelled/expired subscriptions, replace
activation/filter boundaries or reconstruct deleted payload from the current
corpus. Capacity/deadline admission gaps, invalid/cleared payloads, activity
expiry and retained pruning watermarks remain explicit. There is no unlimited
second journal or fabricated historical event.

## Legacy blocking and optional continuation

When the current schema supports persisted, prefer that mode to avoid the
ordinary callback/read race. Do not attach a new subscription as its workaround.
If the client cannot use the new representation, the old default may still
return `blocked_on_delivery`. With `include_diagnostics=true`, honor a positive
`retry_after_ms` only through a waiting mechanism actually supplied by the host:
at most three rereads after the first and 30 seconds of total waiting. Never
shorten an over-budget server hint or busy-loop when waiting is unavailable.

For separate durable insurance beyond that budget or a missing/interrupted run,
a trusted host may explicitly attach
[`zalo.subscription.journal.ready.v1`](subscription-recovery-ready.md) to the
same existing business subscription/generation. Control wakeups run the same
workflow and read that business journal; control references are not messages or
ack receipts. Companion setup does not widen scope or permissions. A quiet empty
journal is not a reason to send a notification or inspect an unrelated chat.

Without a configured and host-verified companion, do not promise that separate
continuation path. It is optional, bounded and observable on exhaustion or
cancellation. Ordinary retained-envelope processing in persisted mode does not
require it. Synthetic adapter proof and actual saved-host-task adoption are
different evidence: installing skills or receiving HTTP 2xx does not prove that
the host saved the new read arguments or executed its workflow.

## Processing, scope, retention and notification limits

Use the journal whether trigger payload is visible or absent. Do not notify
again from an already-acknowledged trigger payload. Multiple callbacks may be
recovered in one run. Scope/direction come from the fanout generation and current
policy is checked again. Historical imports never enter this journal; first
insertion after activation, not event timestamp, remains the boundary.

SQLite stores the cursor per business subscription/generation. Refresh/restart
preserve it; cancellation/reactivation changes generation and rejects old
receipts. When adopting recovery for existing callbacks, reconcile backlog with
known completed notifications. Do not blindly replay notifications or mark the
whole backlog done. Different rules need separate subscriptions/cursors. Clients
sharing the same local credential are not isolated tenants.

Completed delivery records retain their existing seven-day lifetime. Pruning
writes a durable watermark before deletion. A watermark ahead of processing
returns an explicit `retention_expired` gap and receipt. Deleted messages or
revoked access never return their former callback text. Clipped author text uses
its normal issued resource URI and current retention policy; a tools-only host
can use `zalo_read_message_content` with that exact URI.

External notification and processing ack are not atomic. Concurrent runs or a
crash after notification/before ack can duplicate the external effect. Serialize
processing and use a channel-supported idempotency key based on subscription,
generation and eventId when available; exactly-once notification is not promised.
Journal reads/ack, control wakeups and ordinary message reads never send Zalo
messages, read receipts or reactions without their separate explicit workflow.

## Historical motivation

A first-person Things/dot [report from 4 October 2026](https://note.com/unco3/n/nb5426444e496?hl=en)
described a trigger without an immediately visible body. It did not establish an
official platform root cause. The journal provides exact data and durable
processing progress; neither readiness mode changes ChatGPT payload injection.
