# Explicit history import operation

Status: implemented in source with executable schemas, a durable SQLite journal,
a guarded worker and HTTP/STDIO MCP tools. Installed/live acceptance remains
pending. Existing offline replay and subscriptions are unchanged. The user
selected silent historical import on 2026-10-04.

Schemas: `history_import_request.input.json`,
`history_import_operation.input.json` (status/cancel identity), and
`history_import_operation.output.json`. The contract package maps the three
explicit tool names to these shared schemas and advertises flat input/output
documents; clients do not need to resolve external schema references.

## Boundary and inputs

The operation imports available messages into the existing account's local
corpus. It uses the single service's guarded authenticated source, not another
listener, login or native-client database reader. The currently implemented page
candidate supports group cloud history only. Direct history must report
`unsupported`; known-ID profile enrichment is not a history source.

Tools are `zalo_import_conversation_history`,
`zalo_get_history_import_status` and `zalo_cancel_history_import`. Their contracts
use the same flat `conversation_type` and `conversation_id` fields as
browse, rather than a generic action/payload tool. Start additionally requires
`request_id` (UUID), `since` and `until` (RFC3339 instants, half-open interval),
with bounded `page_size` (1–50, default 50), `max_pages` (1–100, default 20) and
`max_messages` (1–5,000, default 1,000). An operation has a 120-second work budget;
source requests share session cancellation. Reversed/equal dates, malformed IDs and unsupported types must
be rejected before upstream access. Calendar interpretations must not be guessed.

`max_messages` bounds source records examined, including duplicates and records
outside the requested interval; it is not a target number of newly inserted
messages. Each fetch is bounded by the remaining record budget. Work time is
accumulated durably across restart/auth pauses; time spent waiting for restored
authentication is not active work. The ledger admits at most 100 active and
100,000 total operations. Request identities and cursor checkpoints are retained;
capacity exhaustion is explicit rather than deleting idempotency evidence.

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

## Events and novelty policy (selected)

Explicit backfill has immutable `notification_policy=none`. New historical
records use internal persistence provenance `history` and create no
`message_events` or delivery jobs. They remain available to browse, search,
context and full-text resources. There is no caller switch to enable historical
notifications. Ordinary live/offline replay keeps its existing behavior.

Register permanent typed message identities in the same transaction as the
import. A later live/replay duplicate must not create an Event, including after
retention removes the imported text. Import never shifts subscription activation
or modifies an existing first-incoming fact. For a direct peer without a fact,
seed `certainty=unknown`, `first_seq=null`: historical evidence establishes that
the peer was already known, but does not prove a complete history or the actual
first incoming. Later incoming events retain nullable novelty; unknown never
matches `first_incoming_only`. Existing known facts remain known. This rule also
applies to historical outgoing records; neither an outgoing import nor a later
live duplicate may fabricate a newly discovered peer.

The confirmed source candidate is group-only; the direct novelty rule is a
storage invariant for any later supported direct source, not an announcement
that direct history is currently available. Historical insertion does not
fabricate collector collection-start times or close known collector gaps.

## Required acceptance evidence

Before exposure, executable schemas and AAA regressions must cover UUID
conflicts, typed/account identity, permissions/revocation, time boundaries,
decimal cursor precision, malformed whole pages, duplicate/live concurrency,
missing/repeated continuation, filtering, bounds, cancellation, auth loss and
restart at both sides of the atomic page commit. Events tests must verify silent
import, unchanged live/replay behavior and historical first-contact handling.

An installed live check must separately establish advertised source support,
actual page results, durable imports and client discovery. Group-only source
evidence cannot close direct/Strangers history acceptance. Existing sends and
subscriptions must remain unchanged throughout this check.

Before each source request, the journal reserves up to 30 seconds of remaining
work. A normal result reconciles actual elapsed work. Crash recovery charges the
reservation conservatively; repeated crashes cannot reset the time bound.
Authentication waiting remains outside active work.
