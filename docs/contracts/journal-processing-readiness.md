# Processing readiness independent of callback receipt

Status: implemented candidate; final acceptance and deployment pending. Supersedes the use of
companion wakeups as the primary fix for the R01 callback/read race.

## Independent facts

Fanout atomically commits a serialized event envelope and its subscription
generation before the callback worker claims it. A permitted, retained, valid
envelope is already processable at this point. HTTP acceptance is a separate
transport fact; processing acknowledgement is a third fact.

Never set `delivered` before a successful receiver response. Never require that
response to read an otherwise available stored event. An early processing ack
updates only the existing ordered processing checkpoint; the worker still owns
its delivery lease, retry policy and original ID/body.

## Executable contract change

Add optional `readiness_mode` to `zalo_read_subscription_events`:

- `delivery_terminal` (default/omitted): preserve existing callback-terminal
  visibility, keys and blocked behavior for old clients.
- `persisted`: read retained envelopes independently of `pending`, `sending`,
  `delivered` or `failed` transport states. The result explicitly includes
  `readiness_mode=persisted`; each entry adds `processing_state=ready|gap`.

The public output enum admits `pending` and `sending` only in the explicit new
representation. `processing_state=ready` requires a non-null event and null gap
reason. `processing_state=gap` requires event=null and an explicit gap reason.
`delivery_state` always describes the real transport state. Persisted mode never
blocks on transport and has `blocked_on_delivery=false`; diagnostic retry hint
is absent or null. Count/byte budgets, `has_more`, order and receipt shape are unchanged.

Use the same principal, exact subscription generation and processing checkpoint
in both representations. They are views of one consumer journal, not two
independent consumer cursors. Switching modes does not replay acknowledged work.
Different rules still need separate business subscriptions/checkpoints.

## Trust, gaps and lifecycle boundaries

Stored readiness is not authority from a message author. Existing collector,
activation/filter, schema, owner, generation and current policy checks apply.
Reads never send Zalo messages, read receipts or reactions. Ack does not report
HTTP delivery or a Zalo read state.

Do not resurrect missing bytes from current corpus data under an old event ID.
Existing invalid-payload, transport-admission capacity/deadline gaps, recall,
retention, activity expiry and access-revocation gaps remain explicit. This
change separates processing of an already retained envelope from callback
outcome; it does not add an unlimited second journal or manufacture events that
were never admitted with a retained payload. A retained failed-delivery envelope
is processable in persisted mode; an empty failed envelope is a gap.

Subscription cancellation, expiry, generation replacement and HTTP410's existing
subscription-deactivation contract still end that generation's active access.
This change does not reopen an inactive subscription. Ordinary retained-message
tools remain separate from subscription processing access.

Preserve existing pruning/watermark and privacy rules. An early processing ack
must not delete the pending/sending outbox row or clear bytes still leased to
the HTTP worker. Callback completion/retry must not rewind processing progress.

## Companion role

The existing optional journal-ready companion remains a bounded recovery signal
for missing/interrupted runs. It is not required to close either callback/read
race in persisted mode. Its published payload, default readiness semantics and
existing subscriptions are not silently changed by this decision.

## Acceptance

1. With no companion, a real TLS receiver reads and acks the exact stored event
   before returning 2xx; transport remains sending until worker receipt commit.
2. The post-2xx/pre-commit window also reads/acks without waiting for delivered.
3. Omitted/default mode still blocks in those windows and preserves old keys.
4. Early ack followed by503/timeout preserves checkpoint and exact retry ID/body;
   callback retry causes no second processing of the acknowledged record.
5. Retained failed payload is readable; invalid/empty/cleared payload is an
   explicit gap. Permission, recall, pruning and ordered/generation checks hold.
6. Restart preserves independent processing and transport progress; early ack
   cannot remove unfinished delivery bytes. No upstream Zalo writes occur.
7. Update executable schemas, runtime ports, SDK fixtures/manifest/reference,
   source/installed skills and recovery/automation docs. Verify both the explicit
   new representation and old strict-client defaults. Synthetic adapter proof
   remains distinct from a saved real-host task update.

### Per-entry serialization boundary

Persisted-mode recovery validates each ready entry against the existing full
`zalo_read_subscription_events.output` contract before decoding it into the typed
application DTO and before counting its recovery response bytes. Validation uses the cached embedded contract and a single-entry
persisted-mode page; no separate copies of profile validators are introduced.
The SQLite owner still checks envelope identity/profile against the selected
stored delivery and enforces policy/lifecycle before this serialization boundary.

A retained envelope with invalid profile data, including an outer-valid envelope
with `data={}`, is an ordered `processing_state=gap`, `gap_reason=invalid_payload`,
`event=null` entry. If schema-valid data cannot be decoded by the closed typed
Event/Activity union (including Activity.Validate), it has the same gap outcome.
Its authenticated receipt, ordering and actual HTTP delivery state remain intact;
no checkpoint moves until the ordinary explicit ordered acknowledgement. One bad
payload must not reject the page and permanently hide following valid records.

Contract compilation failures, invalid local projection metadata, and JSON
serialization/infrastructure failures remain errors. They are not reported as
invalid author data. Legacy/default delivery-terminal representation is unchanged.
The store read adapter may depend narrowly on `docs/contracts` for this explicit
serialization contract boundary; this grants no SDK, owner or network capability.
The owner accepts a pure per-entry projection callback after its privacy/identity
guards and before its existing 28 KiB page budget. The callback changes only the
read projection; retained outbox bytes, delivery state and checkpoint are unchanged.
Thus even a large invalid nested payload becomes a bounded gap before budget
admission; valid oversized envelopes retain their existing explicit budget error.

## Existing host workflow migration

Update the saved workflow through its supported host interface after discovery
advertises the new input field. Keep its exact business subscription, filters,
direction, notification conditions and authorized actions. Only change journal
reads to explicitly include `readiness_mode=persisted`; continue ordered handling
and ack-after-success. Do not create a companion or replace a business
subscription just to fix the callback/read race.

Verify the saved task text/configuration and a safe read through the host's
loaded tool schema. A source skill update, HTTP200, direct local SDK read or
plugin connection alone does not prove that a saved host task was updated.
If the host's cached schema lacks the input, refresh discovery using a supported
host mechanism; do not invent API methods, inspect private host storage or use
an unadvertised argument blindly. No new Zalo message or write permission is
needed for this configuration check.
