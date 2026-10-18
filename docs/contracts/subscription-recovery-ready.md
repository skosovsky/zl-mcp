# Durable subscription journal continuation

Status: implemented as an **optional companion profile** in the working tree.
Target-host activation and end-to-end target-host acceptance: **NOT_RUN**.
This document replaces the earlier
[design proposal](subscription-recovery-ready-proposal.md). It defines an
application extension, not an MCP guarantee that a callback executes an agent.

## Purpose and compatibility

The main callback/read race is addressed separately by
[processing readiness](journal-processing-readiness.md): explicit
`readiness_mode=persisted` reads an already retained valid envelope without
waiting for callback receipt and never changes the true delivery state. This
path requires **no companion**. Verify its loaded schema and backend revision;
this document is not evidence that the new mode is already installed.

The optional `zalo.subscription.journal.ready.v1` profile is insurance for an
absent/interrupted workflow run. It signals that the existing business journal
has readable unacknowledged work under its current control-profile contract. It
has a separate outbox and does not change business filters, event IDs, payloads,
processing checkpoints or receipts. Its existing head-terminal wakeup semantics
are not silently changed by the new journal representation.

Installing updated skills does not attach a companion or update a saved host
workflow. Existing subscriptions remain usable without it. An unconfigured
companion reports unsupported **recovery wakeups**, not inability to read a valid
persisted-mode event. Durable continuation after a missing run remains a separate
criterion requiring a proved continuation channel; successful early reading alone
does not establish that criterion.

## Registration and executable contracts

Only the HTTP Events interface exposes `events/subscribe` and
`events/unsubscribe`. See executable contracts:

- [Subscribe input](journal_ready_v1_events_subscribe.input.json).
- [Unsubscribe input](journal_ready_v1_events_unsubscribe.input.json).
- [Control data payload](journal_ready_v1.payload.json).
- [Generic subscribe result](events_subscribe.output.json).

The subscribe request has `name="zalo.subscription.journal.ready.v1"` and required
`arguments.subscription_id` (1–2048 characters) and
`arguments.generation_reference` (1–256 characters). Both identify an existing
owned, active business subscription generation. The authenticated principal is
never supplied by the request. At most **one active companion per business
generation** and **128 active companions** in the store are permitted.

`delivery` requires `mode="webhook"`, `url` (maximum 4096 characters), and the
existing `whsec_`-encoded `secret` (maximum 94 characters). Receiver challenge,
HMAC and callback destination policy are the same as for business callbacks:
production HTTPS; explicitly configured local callback mode may allow loopback.
A schema-matching URL alone does not bypass runtime destination policy.

Optional `cursor` is null. Optional `ttlMs` is null/omitted for no configured
expiration, or an integer from 60000 through 2592000000 (one minute–30 days).
`_meta` is accepted as a protocol object. Unknown application fields are rejected.
Unsubscribe uses the same event name, arguments and delivery mode/URL without a
secret. Revision checks make cancellation during verification win over activation.

To discover the exact generation, call
`zalo_list_event_subscriptions {"include_recovery":true}`. Each business entry
adds `generation_reference`, `recovery_mode` and `recovery_subscription_id`.
Without this opt-in, the legacy shape is preserved. A stale generation, another
principal's target, revoked exact-conversation access or a cancelled/expired
target cannot authorize a companion. IDs of business and control subscriptions
cannot be interchanged. A storage implementation without this extension rejects
registration with `recovery_unsupported`.

## Control envelope and identity

```json
{
  "eventId": "recovery_<opaque-round-id>",
  "name": "zalo.subscription.journal.ready.v1",
  "timestamp": "2030-01-01T00:00:00Z",
  "data": {
    "subscription_id": "<business-subscription-id>",
    "generation_reference": "<business-generation>",
    "recovery_reference": "<episode-id>"
  },
  "cursor": null
}
```

The data object allows only these three required strings. Subscription ID length
is 1–2048; both references are 1–256. These are control identities, not native
Zalo account/message IDs. No message content, callback URL, secret, signing key,
raw journal row or processing receipt is included.

Retries and lease recovery preserve the round's exact serialized bytes and
`eventId`. A later reminder round has a **new control event ID** but the same
recovery episode. It is intentionally a new control notification, not a renamed
retry or another business message. Receivers may deduplicate transport retries
by event ID while allowing later reminder rounds to wake the workflow.

## Autonomous durable state machine

Schema migration 20 adds `recovery_subscriptions`, `recovery_episodes` and
`recovery_deliveries`. Recovery never depends on an agent having read a blocked
journal first. Startup/worker sweeps reconstruct the earliest unacknowledged
business head for every attached active companion from the durable checkpoint.

A unique episode identifies `(companion subscription, companion generation,
business head)`. A pending/sending head arms the episode; a terminal head makes
it ready. A retained pruning watermark ahead of the checkpoint is also a ready
head representing an explicit retention gap. Missing/expired content is not
invented; it remains an ordinary journal gap.

Each ready episode has at most one pending/sending control round. Claiming uses
a persisted lease and reclaims an expired lease with the same ID and bytes.
Immediately before dispatch, the worker checks the claim/lease, exact business
head, target ownership, both generations, subscription expiration, current policy
and checkpoint again. The independent control outbox has its own `MaxJobs` and
`MaxBytes` accounting using the configured delivery-policy limits; control traffic
does not consume the business queue's accounting. Capacity pressure is observable
as `capacity`; subsequent sweeps may enqueue when space becomes available.

HTTP 2xx completes only that control round. It does **not** complete the business
episode or acknowledge business processing. If the same head remains unacked,
next-round intervals are 30 seconds, 60 seconds, 120 seconds and so on, capped at
one hour, measured after a successfully accepted round. Transport failures use
the normal worker retry classification/backoff and attempt/deadline limits;
`Retry-After` follows the shared delivery policy. Reminder intervals are not an
infinite tight polling loop.

An episode has a seven-day ready lifetime. A head observed pending first receives
its ready lifetime when it becomes terminal. Exhausted transport attempts,
terminal callback rejection and episode expiry are durable exhausted states.
They do **not** silently rearm while the same episode identity remains current.
HTTP 410 deactivates only the companion and suppresses its work. Expired delivery
rounds and old completed control rows are pruned; exhausted episodes remain
observable until superseded/suppressed. Do not promise indefinite eventual
execution or automatic repair of an exhausted episode.

## Ack, cancellation and feedback-loop protection

Control deliveries never enter message/activity fanout or the business journal,
and never acknowledge a business item. Thus they cannot recursively generate
more message-created events or recovery episodes. The ordinary ordered journal
receipt is the only processing authority.

When the checkpoint advances, the next sweep suppresses covered episodes and
cancels their unsent rounds; a pre-dispatch guard also rejects the old head.
Business cancellation, expiry, generation replacement and access revocation
suppress stale work. Companion unsubscribe suppresses only its recovery work,
not the business subscription. Refresh within the same active identity preserves
its generation; a later generation cannot use earlier control claims/receipts.

A callback already dispatched cannot be recalled by a concurrent ack. The handler
must reread the exact business journal; if the item was already acknowledged,
there is nothing to notify again. This is harmless stale control traffic. A crash
between an external user notification and journal ack can still cause a duplicate
notification: this extension does not provide exactly-once external effects.

## Journal diagnostics and receiver workflow

Use `zalo_read_subscription_events` with `include_diagnostics=true` to add
`retry_after_ms`, `recovery_mode`, `recovery_state`, `recovery_reference` and
`recovery_reason`. `retry_after_ms=1000` is a short reread hint only when delivery
is blocked, otherwise null. `has_more` retains ordinary page semantics;
`blocked_on_delivery` separately identifies an earlier pending/sending blocker
in default delivery-terminal mode. Persisted mode does not block on transport
and its retry hint is null.
A false `has_more` does not cancel a blocker or prove no later work exists.

Modes: `unsupported` or `companion`. States: `unsupported`, `idle`, `armed`,
`ready`, `waiting`, `completed`, `cancelled`, `exhausted`. `waiting` means a
control round is pending/sending; `completed` means its former head is suppressed,
not that every future event is processed. Closed reasons include
`not_configured`, `pending_delivery`, `awaiting_ack`, `capacity`,
`subscription_inactive`, `episode_expired`, `attempts`, `deadline`,
`http_rejected`, `payload_too_large`, `callback_gone`, and safe unknown categories.
No callback credentials or business content are diagnostics.

The saved host workflow must identify the exact business subscription even when
its trigger payload is missing. On business or companion wakeup, read that journal,
process in order, and ack each receipt only after successful authorized handling.
A terminal failed/cancelled item may be an explicit gap with `event=null`;
account for the gap instead of fabricating message delivery. A retention gap
cannot restore deleted content. Explicit persisted-mode reading closes the callback/read causal race without
waiting. Bounded rereads are only a compatibility fallback; companion continuation
supplies the independent insurance route for a missing/interrupted run.

## Evidence and current operational limit

Implementation tests cover storage persistence/leases, restart, retries,
reminders, acknowledgement suppression, generation/owner/cancellation checks,
terminal gaps, retention, capacity, attempt exhaustion and the one-hour cap.
See `internal/storage/recovery_test.go`,
`internal/events/recovery_subscriptions_test.go` and
`internal/events/recovery_worker_test.go`.

`internal/mcpserver/recovery_ready_test.go` contains an actual-SDK synthetic host
harness, including `TestSyntheticHostRecoveryActualSDKClosesBothRacesAfterRestart`,
unsupported-mode visibility, stale-control suppression and terminal-gap handling.
It exercises a reader before 2xx and between 2xx and business receipt commit,
then durable continuation without another business event. These tests establish
server/synthetic-receiver behavior, **not** acceptance by the real target host.

Before production activation, the user/host must select and register the optional
companion, confirm discovery/schema acceptance and fresh-ID wakeups, bind its
saved workflow to the exact business subscription, and verify recovery there.
No companion may be silently added to an existing receiver. Until actual-host
activation is proved, that operational acceptance remains NOT_RUN and the final
R01/R12 gate is not passed merely by this implementation.
