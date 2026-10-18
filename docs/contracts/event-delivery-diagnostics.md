# Safe delivery and tool diagnostics

Status: implemented; final revision and acceptance evidence are tracked in
[the hardening report](../mcp-skills-hardening-acceptance.md).

## Opt-in journal representation

`zalo_read_subscription_events` accepts `include_diagnostics: boolean`, default
false. With omitted/default `readiness_mode=delivery_terminal`, omission/false
preserves the previous JSON result and entry keys. The independent
`readiness_mode=persisted` opt-in adds its own explicit view/processing fields
even without diagnostics; it does not require `include_diagnostics=true`. When
true, retained business-delivery entries also contain:

- `delivery_reference`: `dlv_` followed by a base64url HMAC-SHA256. The state key
  and delivery row bind the reference. It is stable across attempts and service
  restart of the same state, differs between rows and is not an event/message ID.
- `delivery_reason`: a category from the executable schema's closed enum.
  Unclassified/internal strings become `unknown`; never return raw errors.
- The page may include `retry_after_ms` (milliseconds). A blocked delivery
  currently suggests 1000 ms. It is advice, not a scheduled continuation or a
  guarantee of completion within that time. Prefer advertised
  `readiness_mode=persisted`: retained envelopes do not wait for callback receipt,
  blocked_on_delivery is false and this retry hint is null. No new subscription
  is needed for the ordinary race. The [companion](subscription-recovery-ready.md)
  is optional insurance for missing/interrupted runs.

With diagnostics enabled, the page also includes continuation status:

| Field | Meaning |
| --- | --- |
| `recovery_mode` | `unsupported` or `companion`; describes configured server continuation capability, not proof of host execution |
| `recovery_state` | `unsupported`, `idle`, `armed`, `ready`, `waiting`, `completed`, `cancelled` or `exhausted` |
| `recovery_reference` | Nullable opaque episode reference; not a message ID, processing cursor or ack receipt |
| `recovery_reason` | Nullable closed safe category from the output schema; never a raw storage or receiver error |

`unsupported/not_configured` means no configured companion for this business
subscription generation. `idle` means a configured companion has no current
episode. `armed` is waiting for the blocked business head to become readable;
`ready` is eligible for a control round and `waiting` has an outstanding control
delivery. `completed` describes the server episode's completion, not an external
notification guarantee. `cancelled` and `exhausted` are explicit stop states;
inspect the reason rather than claiming silent recovery. Representative reasons
include `pending_delivery`, `awaiting_ack`, `acknowledged`, `generation_changed`,
`head_changed`, `episode_expired`, `subscription_inactive`, `capacity` and the
applicable delivery-failure categories. Unknown internal values become `unknown`.

`zalo_list_event_subscriptions(include_recovery=true)` separately opts into
`generation_reference`, `recovery_mode` and nullable `recovery_subscription_id`.
The generation reference binds explicit companion setup to an existing owned
business subscription. Omitting/false preserves that list's old strict shape.
These fields do not enable a companion or alter the business scope.

The companion is not the primary fix for callback/read races: explicit persisted
reads make already-retained envelopes independently processable. Control rounds
use the optional dedicated profile `zalo.subscription.journal.ready.v1`;
they are not inserted into the business message journal. Its payload references
the exact business subscription/generation and recovery episode. Both kinds of
agent wakeup reread the same business journal and use its original receipts.
Synthetic signed-receiver tests exercise this flow; actual target-host/Ann
activation and processing remain **NOT_RUN** until separately evidenced.

Pruned watermark entries have no retained delivery row and omit the delivery
reference/reason. Existing `delivery_state` and `gap_reason` semantics remain
unchanged in the default view. A terminal failed delivery remains a gap there;
in persisted mode a retained valid failed-delivery envelope is ready, while
missing/invalid/withdrawn bytes remain gaps. Diagnostics alone neither changes
readiness nor advances the processing checkpoint. Early ack never marks the
transport delivered or clears unfinished worker bytes. See
[independent processing readiness](journal-processing-readiness.md).

Examples of distinct categories: `queue_capacity`, `deadline`, `attempts`,
`payload_too_large`, `http_rejected`, `http_transient`, `network`,
`subscription_inactive`, `access_revoked`, `callback_gone`, `profile_removed`.
`accepted` denotes HTTP receipt only, not notification success or Zalo read state.

Strict old clients do not request this representation. Clients choosing it must
use the updated output schema. Historical envelopes/operation outcomes are not
rewritten. Subscription ownership, current access, HMAC receipt and ordered ack
checks apply identically in both modes.

## Local delivery logs

Each attempt has a fresh random `correlation_id` and a closed `delivery_kind`:
`business` for normal event delivery or `journal_recovery` for a companion control
round. For the production store-backed business queue, `delivery_reference` links start/finish logs, journal diagnostics and later
attempts without revealing the original event/subscription identifier. A foreign
Queue implementation without this optional reference capability retains the
previous attempt-only logs; it cannot claim durable diagnostic correlation.

For the production store-backed control queue, `delivery_reference` is `rdlv_`
followed by a base64url HMAC-SHA256 over the durable control delivery row in a
separate key domain. It survives retries and restart, differs between reminder
rounds, and cannot collide with the `dlv_` business-delivery reference even when
their row numbers match. Control attempt logs also contain `recovery_reference`:
the same random, opaque application episode reference exposed by the control
payload and opt-in journal recovery status. It links all reminders for that
unacknowledged head without exposing native IDs or a callback address.
A foreign queue without the optional control reference capability retains
attempt-only delivery correlation; the episode reference remains available.

No callback URL, native account/message ID, payload, signing secret or session
value is added to logs. References are local log attributes, not metric labels;
external telemetry policy and retention remain independent.

## Tool failures

An invocation creates a random correlation ID before preflight/dispatch. Failure
results place the same ID in `details.correlation_id` as the final `mcp_call` log.
The error code, retryability and next action are unchanged. Domain errors are not
mutated or formatted into logs; stored operation results remain intact.

The error contract already allows application-specific details, so this addition
uses that existing extension point. Correlation is evidence for a particular
request, never proof of an upstream business outcome.

## Verification

- `TestJournalDiagnosticsAreOptInClosedAndStableAcrossRestart`: legacy shape,
  closed categories and durable keyed references, including pending retry hint.
- `TestFinalAuditIncludesPreflightAndPostExecutionFailures`: client/log ID match
  for authorization, admission and execution failure paths.
- Existing worker diagnostic allowlist tests reject payload and credential leaks.

- `TestRecoveryWorkerSignedHTTPRetryReminderAndAckSuppression`: signed local
  control transport, retry/reminder and suppression after business ack.
- `TestRecoveryAutonomousTerminalRestartReminderAndAck` and
  `TestRecoveryLongBlockTerminalPrunedAndExhaustion`: persisted episode lifecycle,
  restart, blocked head, pruning and finite continuation limits.
- `TestRecoveryManagerOwnedGenerationIdentityAndControlIsolation`: exact owned
  business-generation binding and separation from business event data.
- `TestSyntheticHostRecoveryActualSDKClosesBothRacesAfterRestart`: actual-SDK
  synthetic host reads before 2xx and between 2xx/receipt commit, then resumes
  without another business event, including restart.

The names above describe executable coverage, not proof that a particular hosted
agent installed its companion or updated its saved task. See the canonical
[continuation contract](subscription-recovery-ready.md) for limits and that gate.
