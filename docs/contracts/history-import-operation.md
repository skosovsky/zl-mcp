# Explicit history import operation (design draft)

Status: design draft, not an advertised MCP capability or installed feature.
Executable input/output schemas and implementation must follow the notification
policy decision below. Existing offline replay and subscriptions are unchanged.

## Boundary and inputs

The operation imports available messages into the existing account's local
corpus. It uses the single service's guarded authenticated source, not another
listener, login or native-client database reader. The currently implemented page
candidate supports group cloud history only. Direct history must report
`unsupported`; known-ID profile enrichment is not a history source.

Proposed tools are `zalo_import_conversation_history`,
`zalo_get_history_import_status` and `zalo_cancel_history_import`. Their contracts
must use the same flat `conversation_type` and `conversation_id` fields as
browse, rather than a generic action/payload tool. Start additionally requires
`request_id` (UUID), `since` and `until` (RFC3339 instants, half-open interval),
with bounded `page_size`, `max_pages` and `max_messages`. Numerical limits and
defaults must be fixed in schemas before implementation; the source page size
cannot exceed 50. Reversed/equal dates, malformed IDs and unsupported types must
be rejected before upstream access. Calendar interpretations must not be guessed.

The caller never supplies a raw upstream cursor, storage path, account ID or
Events callback. Source cursors are exact decimal strings in the internal
checkpoint and must not pass through float64. Start checks the current collection
policy and source availability. It does not add a conversation to that policy,
broaden a subscription, mark messages read or authorize sending.

## Durable request and operation identity

Persist the request fingerprint, operation ID, immutable requested interval and
limits, effective import policy, account binding and checkpoint before returning
an accepted operation. A repeated request UUID with identical effective inputs
returns the existing operation; a changed conversation, interval or limits
produces a stable request-conflict error. A UUID must not silently start another
operation after restart or a lost response.

Status and cancellation must enforce the same local MCP authorization boundary
and bound account as start. Operation IDs are references, not permission grants.
Only one import worker may advance an operation. Concurrent work for one typed
conversation must be serialized or explicitly rejected; the implementation must
not race checkpoints or use a second authenticated session.

Use durable states `queued`, `running`, `paused`, `completed`, `partial`,
`cancelled`, `failed` and `unsupported`, with stable, safe stop reasons. A worker
crash leaves a recoverable checkpoint rather than a fabricated completed result.
On restart revalidate account, source support and collection policy before
resuming. Authentication loss pauses work; cancellation and policy revocation
prevent new source calls and persistence. Explicit user cancellation is terminal
for that operation; retrying the same UUID does not reactivate it.

## Page commit and recovery

Normalize and validate the complete bounded source page before any write. Reject
wrong conversation identities, malformed timestamps/IDs and oversized records
without importing a valid-looking subset. Deduplicate against permanent message
identities and concurrent live/replay writes. Do not overwrite an existing live
message, reset first-incoming facts or rewrite ambiguous send outcomes.

Atomically commit newly accepted messages, import provenance, effective Events
decisions, counters and the next checkpoint. A crash before commit safely retries
the page; a crash after commit resumes at the committed checkpoint. Duplicate
pages must not double-count inserted records or produce duplicate outbox entries.
Any retained identity used for deduplication remains subject to the existing
retention/account invariants.

Continuation is permitted only from validated source evidence. Missing/invalid
or repeated cursors, absent continuation metadata, upstream filtering, an empty
continuing page and client limits each have a distinct stop reason. Never infer
full history from a short/empty response or a terminal available-source flag.
Stopping at `since` requires a proven source ordering guarantee; otherwise scan
within the limits and filter records locally. Bound HTTP attempts, elapsed work,
pages and accepted/source-record counts, including pages with only duplicates
or out-of-interval records.

## Progress and coverage

Status returns requested interval, source kind, lifecycle state, timestamps,
pages/records observed, inserted/duplicate/out-of-interval counts, applicable
source flags, stop reason and whether source continuation remains available.
Counts describe this operation; they are not total inbox/history size. Preserve
nullable filtering/join evidence and distinguish unknown from false.

Report the earliest/latest newly imported timestamps separately from existing
corpus coverage and the requested interval. An operation with zero inserted
records can still have observed duplicates, filtering or unavailable data. Do
not claim that a continuous requested interval was covered merely because its
endpoints have messages. Existing collector gaps and retention limitations
remain visible. Source exhaustion describes the available source, not complete
Zalo history; `history_complete` remains false unless an independent documented
source guarantee is verified.

No message bodies, profile details, tokens, raw source responses or signing keys
belong in progress logs. Full imported text is read through existing conversation
message resources; status is not an alternative corpus dump.

## Pending Events decision

The user has been asked whether explicit backfill should persist without
notifications or notify matching subscribers about first-persisted historical
records. This remains unanswered. Do not select a default or implement the
dependent ingestion/outbox path while that decision is pending.

Either decision must retain the effective policy in the operation and preserve
ordinary live/offline replay behavior. Historical import must not fabricate a
first incoming contact, shift a subscription's activation boundary, clear
existing novelty facts or enqueue an event for an already-known message.
Contracts must specify historical provenance and novelty semantics before
implementation, including how a later live/replay duplicate is treated. A
message first observed through backfill must not later become a newly discovered
peer merely because a live duplicate arrives.

## Required acceptance evidence

Before exposure, executable schemas and AAA regressions must cover UUID
conflicts, typed/account identity, permissions/revocation, time boundaries,
decimal cursor precision, malformed whole pages, duplicate/live concurrency,
missing/repeated continuation, filtering, bounds, cancellation, auth loss and
restart at both sides of the atomic page commit. Events tests depend on the
selected policy and must include historical first-contact behavior.

An installed live check must separately establish advertised source support,
actual page results, durable imports and client discovery. Group-only source
evidence cannot close direct/Strangers history acceptance. Existing sends and
subscriptions must remain unchanged throughout this check.
