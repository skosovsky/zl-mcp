# Conversation completion: acceptance evidence

Status: work in progress. Tracks [the combined task](task-conversation-completion.md). No model evals are used.

## Installed schema-11 checkpoint: 2026-10-05

The latest candidate was built with `-trimpath` from a hash-verified isolated
643-file source snapshot. Root and nested-module race/vet checks passed before
the snapshot; CGO-free macOS arm64 and Linux amd64 builds succeeded, and the
snapshot secret scan found no leaks. The installed arm64 SHA-256 is
`0d21c0dc17ad04b967b0e361a375a8c1f2515d111cb3f06e74582af399cd8646`.
This binary was built from an isolated source snapshot before signing
[`fc88336`](https://github.com/skosovsky/zl-mcp/commit/fc8833646f502357ea5a2bccd85bb53c999ede48).
Its 468 Go/module source files match that published commit; it is not a release
artifact built with committed VCS metadata.

A private-copy migration 8→11 passed before installation. The existing agent
was then stopped, account-lock release verified, and the full state, config,
previous binary and plist backed up privately and checked before replacement.
The same agent resumed authenticated/connected; installed database integrity
passed at schema 11. All 47 previous messages and identities, two send operations
and four novelty records were unchanged. Subscription IDs, filters, active
states, generations, watermarks and lifetime fields for both rows were preserved;
callback and secret values were not read or compared. No send, login, phone
request or subscription change occurred.

An actual connected-client status call returned authenticated/connected with
47 messages and no last error. Local discovery exposes 18 tools and all three
event profiles. External tool availability still lists only 15 tools, omitting
history import/status/cancel. Experimental mobile ports remain unavailable as a
production import route; real Strangers archive, v2 scenarios, fresh approved
sends and final full-scope acceptance are still open in
[the current gate report](conversation-completion-open-gates.md).

Follow-up on that same installed candidate: a real STDIO client initialized
the installed `serve` bridge with protocol `2025-11-25`, listed 18 tools and
read connected/authenticated status from the existing service. Initialization
advertised tools/resources/logging and no Events. The bridge was then closed;
it created no second collector.

Read-only HTTP and actual connected-client checks both traversed a retained
direct dialogue over an explicit RFC3339 interval without a keyword, then read
the next page and context for the first anchor. Continuation preserved the
snapshot and excluded that first-page message; context returned the matching
anchor. Private IDs and message text were not included in the report. These
checks prove current installed reads, not deep Strangers recovery or Events
filter acceptance.

## Local browsing implementation

The executable input/output schemas were added before the storage and MCP dispatch implementation. `zalo_list_conversation_messages` provides exact typed local chronological browsing without a keyword, bounded pages, RFC3339 `[since, until)` and signed snapshot continuation. Existing search behavior remains unchanged. The [browsing contract](contracts/conversation-browsing.md) describes snapshot versus current coverage, resource retrieval, empty reasons and limits.

Added storage regressions exercise same-time ties, both orders, timezone offsets/exclusive upper boundary, a late historical insertion excluded from an existing traversal, continuation after reopening SQLite, colliding direct/group IDs, direction, Unicode truncation, empty reasons and permission checks. An MCP client calls the new tool without a query and reads the returned escaped resource URI, validating the embedded output contract through the actual handler. Tool-count discovery expectations now include the additive tool.

This stage uses temporary synthetic databases only. No installed binary, session, configuration, subscription or runtime data was changed. Strangers protocol discovery, directory expansion, explicit history import, live v2/first-incoming checks, installed skills and final publication remain unfinished. Do not interpret source capabilities as capabilities already available in the connected client.

Validation for this checkpoint: root `go test ./...` and `go vet ./...` passed; focused `go test -race ./internal/storage ./internal/mcpserver ./docs/contracts` passed. Both repository skills now describe conditional use of the additive browse capability, without assuming the installed client already exposes it. Complete root/nested race checks, clean checkout validation, deployment and client reload remain later gates.

Checkpoint commit `0079c5edbd9b62aa07f7664244435254518f02c3` was pushed to public main. [CI run](https://github.com/skosovsky/zl-mcp/actions/runs/37149944708) completed successfully for Linux and native macOS. Tree and checkpoint-commit secret scans found no leaks; both repository skills passed structural validation. These checks do not establish installed browse availability or resolve the remaining scope.

The subsequent [native-client static-code investigation](conversation-discovery-history-research.md#installed-native-client-static-code-follow-up) found local Strangers classification and a known-thread preview request candidate. It does not yet establish full inbox enumeration, a working direct-history API or recovery of the reported missing conversation. No installed runtime changed during this investigation.

## Offline continuation and directory source checkpoint (before installation)

The native-client investigation identified response continuation fields omitted
by the pinned Go listener. The new source patch preserves queue origin and
`more`/`lastActionId`, emits metadata once after mixed batches, and sends a
bounded continuation only after the batch has persisted. Root replay-pager and
nested listener/model race tests passed. These synthetic results do not establish
that the reported missing historical conversation will be recoverable.

The additive directory schemas now allow aliases, relationship metadata,
metadata provenance and an explicit `has_stored_messages` flag. A transactional
contact page merge respects collection policy and does not create message,
Event or first-incoming records. Local catalogue reads use accent-insensitive
candidate matching, including Vietnamese đ/d, without identifying people by
name. Storage tests verify metadata-only entries, actual-message upgrade,
policy exclusions, atomic rollback and reopening the migrated database.
Root storage/MCP/contracts tests passed. Installed upstream refresh, source diagnostics,
deployment and live acceptance are still pending; this checkpoint
must not be described as a working complete inbox or complete history import.

The source now includes a guarded contacts-page adapter and cancellable background refresh in the existing service session. Synthetic tests cover short/repeated/oversized pages, 20-page termination, partial-error retention and cancellation. The MCP diagnostics resource validates its safe payload against an embedded JSON schema; a real in-memory MCP client verifies alias discovery and metadata-only entries through the updated tools. This remains uninstalled source evidence.

## Installed checkpoint: 2026-10-03, revision 2699d2e

[CI](https://github.com/skosovsky/zl-mcp/actions/runs/37151538751) passed root and nested-module test/race/vet on Linux and native macOS, plus the configured CGO-free cross-builds. The installed arm64 Go binary SHA-256 is `b6f72e447ed104ca052f5f45bc2db5193d44d5f1932fe5f7fdaad2982d80b0b4`. The existing single LaunchAgent was retained. After bootout, installation waited for account-lock release, verified a private full-state/config/binary/plist backup outside iCloud, then replaced the binary atomically and bootstrapped the same agent. No login, message send or subscription mutation was performed.

SQLite migrated from version 6 to 7. The collector is authenticated/connected. All 23 messages and permanent identities, first-incoming facts, both subscription rows and both send-operation rows match the stopped-service backup exactly. The existing ambiguous send remains `unknown`. The first contacts refresh observed/permitted 14 unique IDs and ended on a short page; the catalogue now contains 16 typed entries. This proves source refresh, not completeness of all dialogues.

The installed authenticated HTTP endpoint advertises 15 tools, including keyword-free browse, and all three existing event profiles including v2. Actual browse returned two messages and a continuation page with the same snapshot; the diagnostics resource reported the completed contacts refresh. Token values and message bodies were neither logged nor published.

Live queue metadata reported `queue_exhausted` after one page for both direct (12 records) and group (2 records) queues. All replayed records were duplicates, with no persistence/decoder error. This verifies the terminal metadata path only; a live `more=true` continuation has not been observed. The reported Strangers dialogue still does not match exact/accent-normalized catalogue queries. Available replay and friends metadata did not recover that conversation or its September history.

The currently exposed connected-client tool catalogue still lacks the new browse tool (and the send/status tools). Server-side discovery alone does not prove refreshed plugin discovery. Repository skills are structurally checked; no local copies of either project skill were found in the Codex skills directory. Remaining gates include client capability refresh, explicit history import, Strangers recovery and the uncompleted v2/new-recipient live scenarios. The goal remains in progress.

## History adapter checkpoint (source only)

Revision 6a17fad introduced the bounded group-cloud page candidate; its
[CI](https://github.com/skosovsky/zl-mcp/actions/runs/37152344493) passed.
The subsequent domain adapter normalizes exact numeric identifiers/timestamps,
checks the requested typed group identity, preserves nullable continuation and
filtering evidence, and rejects a whole malformed/oversized page without yielding
partly importable records. Direct history and missing advertised group-cloud
sources produce an explicit unsupported result.

Synthetic adapter tests verify own/incoming direction, exact IDs above 2^53,
untrusted text preservation, invalid/mismatched records and unchanged empty
storage. Normalized history records carry no ordinary persistence source;
Store.Put rejects them. A session-guard regression verifies cancellation of an
in-flight history request and rejection of another request after shared auth
loss. Targeted root/nested race tests and root vet passed. This is not installed
or live history acceptance and does not recover the reported Strangers dialogue.
The user decision on notifications during explicit backfill remains pending;
ordinary live/offline replay semantics have not changed.

The next directory source checkpoint adds bounded known-ID profile reads behind
the shared session guard. Pure/stub regressions reject foreign or conflicting
response identities, duplicate/ambiguous version keys and invalid batch sizes;
verify that missing/unchanged profiles are not fabricated; and protect the
caller's exact IDs from the pinned SDK's mutation of its variadic argument slice.
Only directory fields cross the domain boundary. Targeted root race tests passed.
At that adapter-only checkpoint, background scheduling and installed acceptance
were pending; it does not establish discovery of unknown Strangers IDs.

Known-ID profile enrichment is now wired into the same background directory
worker, with a separate diagnostic profile result. It scans at most 20 batches
of 100, preserves policy exclusions, advances past missing/excluded pages and
does not fabricate records for unchanged/missing upstream profiles. Storage
regressions exercise excluded-page continuation and cached-ID suppression;
collector regressions exercise completed/missing/foreign responses and the
2,000-ID limit. Root storage/collector/Zalo/MCP/contracts race checks passed.
This worktree checkpoint remains uninstalled and unpublished while the prepared
Git commit is waiting for its configured SSH signer.

Known profiles older than one hour become refresh candidates without deleting
identity or message history; a regression checks fresh-cache suppression and
stale-cache eligibility. Both repository skills now distinguish profile metadata
from messages/Events and interpret the separate profile-source diagnostics.
Structural validation passed for both; no model evals were run.

An additional cancellation regression stops profile refresh after the upstream
response but before persistence. It verifies that no profile is written and the
final diagnostic reports `cancelled_or_deadline`, rather than a storage fault or
a lingering `running` state. Root tests/race/vet and nested zcago race tests/vet
passed on this worktree. CGO-free builds produced a macOS arm64 Mach-O binary
and a Linux amd64 static ELF binary. The redacted tree secret scan found
no leaks. These checks do not establish installed or connected-client acceptance.

A worker-level race regression exercises `contactsLoop` with the guarded source,
then causes shared authentication loss while a profile request is in flight.
It verifies final `partial/auth_required` diagnostics, rejects a subsequent
profile read before it reaches upstream, persists no profile and stops the
worker on shutdown. This proves the synthetic lifecycle/authentication boundary,
not a real-account profile lookup or unknown-Strangers discovery.

The [explicit history-import design draft](contracts/history-import-operation.md)
now separates the page adapter from a durable MCP operation: request conflicts,
account/policy boundaries, atomic page checkpoints, restart/cancellation,
continuation evidence and honest coverage are specified as implementation gates.
Notification/novelty semantics remain a pending decision. No executable import
schema, MCP import tool or historical persistence capability is claimed.

## Natural incoming acceptance: 2026-10-04

The user's client screenshot confirms assistant notifications from a peer outside
the previous list. A read-only database check matched that dialogue without
publishing its identity or message bodies: six records arrived through `live`
between 06:24 and 06:28 UTC, five incoming and one outgoing. All six corresponding
delivery rows are `delivered/accepted`; the first required two attempts and the
others one. The screenshot independently confirms assistant handling, rather
than relying only on callback acceptance. One known first-incoming fact is
retained for the dialogue.

The effective subscription remains v1 `zalo.conversation.message.created` with
`scope=direct`, `direction=all` and no first-incoming-only filter. This establishes
natural direct ingestion and delivery beyond the old recipient list, not live
v2 incoming-only/first-incoming filtering or discovery of all old Strangers
history. No subscriptions, send permissions or service state were changed for
this inspection. The prepared history-normalization commit completed as 8208b39
after the user confirmed its configured SSH signature.

## Silent historical import checkpoint: 2026-10-04 (worktree only)

The user selected explicit old-history ingestion without Events/notifications.
Executable request, lookup and status schemas now enforce fixed silent policy,
bounded pages/records and typed operation identities, preserving nullable source
evidence and `history_complete=false`. Schema regressions reject callback/cursor
injection, notification overrides, ambiguous dates and over-limit requests. The
schemas are deliberately not advertised as unimplemented tools.

The storage page port validates a whole normalized page and commits it
atomically with permanent typed identities. It uses shared message persistence,
retains `history` provenance, creates no Events/delivery jobs and does not invent
a collector start time. Historical direct identities seed unknown novelty while
preserving any existing known fact. Regressions verify malformed-page and SQL
failure rollback, retained live text/facts, duplicate counts, readable historical
text, and no new Event when replay restores a historical identity after
retention/reopening. Storage/contracts race checks passed. Both skills describe
silent import without claiming client availability.

The durable operation journal, checkpoint worker and MCP handlers are not yet
implemented. This port is not installed/live import acceptance and does not
establish support for direct or Strangers history.

The next worktree checkpoint adds SQLite migration 8 and a durable history
ledger. Request normalization preserves exact typed IDs, canonicalizes UUID/date
instants and applies the executable limits before fingerprinting. Equivalent
retries return the saved operation; conflicts and concurrent active work for the
same conversation are rejected. The journal binds operations to the stored
account, retains exact decimal cursors, accumulated work duration and revisions,
and keeps terminal cancellation/unsupported outcomes on retry.

Page records, permanent identities, progress, nullable source evidence and cursor
history commit in one transaction. Regressions cover failure of the final
checkpoint update after message insertion, restart on a committed checkpoint,
stale revisions, cancellation, repeated/missing cursors, filtering and page/record
limits. Auth pause preserves work budget; explicit account-locked recovery (not
ordinary Store.Open) requeues interrupted work and cancels revoked scopes.
Migration failure rolls back its DDL and version marker. Storage/domain/contracts
race checks and targeted vet passed. Typed status results validate against the
executable output schema. No historical Events are emitted.

This is uninstalled source evidence. The worker and MCP registration still need
implementation and installed/live verification; schema 8 has not been applied
to the running service by these tests.

## Guarded worker and MCP checkpoint: 2026-10-04 (uninstalled source)

The import worker and three explicit MCP tools are now implemented. One sequential
worker shares the collector's guarded authenticated session and resumes the
account-bound journal. Synthetic tests cover exact continuation, remaining-record
fetch bounds, auth pause/resume and cancellation while an upstream page is in
flight; the late result cannot persist. The production HTTP service test uses
one restore/listener, imports a synthetic group page, reads its context and
repeats the UUID without a second fetch. Events diagnostics remain empty.

A real in-memory MCP client verifies discovery, flat contracts, annotations,
status, context and full-text resources. This exposed a missing `history` value
in shared Message output schemas; all affected Message enums were synchronized
without widening catalogue metadata enums. Import is a non-destructive,
idempotent local write with upstream access; status is a local read and cancel
is a local idempotent write. Targeted history/service race tests passed.

Earlier paragraphs record earlier checkpoints, not the current capability
catalogue. Source registration is now 18 tools. Deployment, actual client
refresh, live group-cloud acceptance, v2 filter checks and remaining authorized
send acceptance are still separate gates. Direct/Strangers historical recovery
remains unsupported; natural incoming delivery does not prove old-history
recovery. No installed state or subscription was changed by these tests.

The pre-request journal also reserves bounded source work. Regression tests
simulate four interrupted 30-second reservations across reopen/recovery and
require terminal time_limit without invented records. A normal auth pause
reconciles actual work and releases unused reservation rather than charging
authentication waiting. This closes the crash-before-checkpoint budget gap.

## Installed checkpoint: 2026-10-04, revision b27ee34

Revision `b27ee340a769597d40210aa4369354ddaabfe145` is published. Its
[CI run](https://github.com/skosovsky/zl-mcp/actions/runs/37199168176) passed on
Linux and native macOS. Root and nested-module race/vet checks, no-CGO macOS
arm64/Linux amd64 builds, a clean-checkout test/build, skill validation and local
Markdown links passed. Gitleaks found no secrets in the tree or all 16 published
commits. The clean installed build reports that revision with `vcs.modified=false`.

The sole installed LaunchAgent was stopped, its account lock was acquired with a
bounded wait, and a full private state/config/plist/binary snapshot was copied
and hash/integrity verified before replacement. The service restarted with the
same session/config/token. SQLite migration 8 completed. Comparison against the
stopped snapshot confirms retention of all 47 messages and message identities,
both subscription rows and both send-operation rows. No send permission or
subscription was changed by deployment.

Installed local HTTP MCP advertises 18 tools and all three Events profiles. Typed
context and keyword-free browse work on retained direct messages. The connected
client's existing `zalo_get_status` tool independently reports authenticated and
connected, with 47 records. Its actual `zalo_list_conversations` resolved one
requested direct conversation, and `zalo_list_conversation_messages` returned
five records with has_more=true without a search word. Identities and bodies
were not included in this report. This verifies client browse access in addition
to local HTTP. This session's external catalogue still lacks the
three new import tools: local discovery is not evidence of external refresh.

A bounded import of the existing authorized test group used one page and at most
five source records. Its durable result is `failed/upstream_unavailable`, zero
pages/records/insertions; no historical messages were added. One diagnostic
retry and a later status read hit the diagnostic client's 10-second timeout;
a read-only SQLite check confirmed the saved terminal operation and unchanged
corpus. Repeating the status/start reads with a 40-second diagnostic deadline
then completed in 2.69 seconds and returned the same operation/state. No
replacement UUID or extra page was requested. This establishes installed
operation persistence and exact live retry, not live history source success. The safe stop category does not distinguish transport,
API or response-normalization errors yet; the actual failure needs further
source diagnosis. The collector remains connected.

Remaining gates include upstream page diagnosis and successful available-history
acceptance, external import discovery, v2 filters and remaining explicitly
authorized send checks. Older Strangers history remains unresolved. The goal is
not complete.

## Source diagnosis and preload checkpoint (unpublished worktree)

Safe source diagnostics now retain an optional numeric API code and emit only
operation UUID/fixed category. Normalization errors have a typed invalid-page
identity; worker results distinguish them from unavailable sources. Tests verify
private underlying error text is absent from logs/exported errors while typed
causes remain intact. Root race/vet and no-CGO cross-build checks passed.

A preload SDK candidate uses only the advertised conversation service, the
existing session and an empty serialized thread mapping. Its executable raw-page
schema and guards preserve exact JSON records and nullable category availability,
reject whole malformed/oversized pages and bound both wire and expanded response
JSON. Synthetic request/response tests pass; no production caller is wired, no
live preload request has run, and no catalogue/history completeness is claimed.
The dependency patch provenance records the pinned source reference.

Publication of this checkpoint has not happened: the configured SSH agent
rejected commit signing. The previous published revision and installed b27ee34
service remain intact. This is a signing-provider refusal, not an approval-review
rejection; signing has not been bypassed. The user was asked when available to
confirm the next signature while independent source work continued.

## Preload normalization and directory boundary (unpublished worktree)

The optional source adapter now normalizes typed metadata and available recent
messages without float conversion. It requires a bound account, explicit source
classification, unique typed identities and valid timestamps. Foreign direct
ownership and ambiguous outgoing peers reject the whole snapshot; sender zero
and the actual account ID both retain correct self direction. Plain direct quote
metadata is retained where supported. OA/page records are counted, never guessed
as direct conversations. Records keep empty ingestion provenance and no novelty.

A session-guard test proves shared auth cancellation and prevents another source
call after auth loss. The atomic metadata merge requires account binding, obeys
selected/all policy, preserves existing stronger names/provenance and generates
no messages/identities/Events/novelty/subscriptions/send operations. Tests cover
typed collisions, excluded peers, malformed-page and SQL rollback, and real
catalogue outputs against executable schemas including preload_catalog summary.
Focused preload race checks passed in domain adapter, storage, collector and
contracts. No periodic refresh or MCP caller invokes the candidate yet; live
page shape and read effects remain gates before enabling it. Source review and
synthetic tests do not prove old Strangers history was recovered.

## Owner-only source probe checkpoint (unpublished worktree)

A native `probe-preload` CLI command now reaches the guarded source through the
existing owner-only Unix socket. It accepts no extra arguments and returns only
bounded counts, availability and safe failure categories/API code. Neither the
MCP catalogue nor the public membership Call exposes it. Executable private
schemas reject URL/token injection; responses omit underlying private data.

The synthetic production-service test proves one restore/listener and one source
request while every corpus/catalogue/identity/Event/novelty/send/subscription
count remains zero. The command has a bounded 35-second client deadline and
30-second source work; private HTTP response budget is 40 seconds. No new
background process or Python wrapper is introduced. This finishes the offline
path needed to validate the preload endpoint after deployment. Signing and
installed live verification are still pending; no source message was imported.

## Installed preload protocol evidence — 2026-10-04

The native owner-only probe was deployed as a checked local candidate based on
45275b8 with modified source. Each replacement used a verified stopped-service
backup, exact source archive/manifest and binary hash. This is not a published
release; signed publication remains pending. The latest candidate SHA-256 is
36b4794e1836f41a5a81ccf62174129b21726ed1dd2719943a51e7d889581117.

The actual endpoint first failed whole-page normalization. Closed diagnostic
reasons isolated the failure to actionId. The SDK models this as an optional
string; allowing empty optional actionId/cliMsgId/userId/realMsgId fields then
made the actual page normalize successfully. Required message ID, sender,
recipient and timestamp still reject empty values. Tests cover both boundaries
and ensure unknown diagnostic text cannot escape through the probe.

The successful bounded live read returned 57 metadata entries, four direct
messages and two group messages, with no unsupported OA/page messages in that
response. These are one response's counts, not a completeness guarantee or
proof that the requested older Strangers thread was recovered. No page was
persisted. Every output explicitly reported metadata_persisted=false,
messages_persisted=false, catalog_complete=false and history_complete=false.

After deployment/probing, SQLite integrity remained ok; every previous message,
permanent identity, subscription and send-operation row remained present.
The local corpus remained 47 messages and message_events remained zero. No send,
join, subscription change, second login or second collector was used. Service
status reported no error after startup. Relevant root packages passed test/race
and vet. Remaining gates include safe production catalogue refresh/silent source
import, concrete older-thread recovery and truthful coverage, supported group
pagination, actual client discovery and previously listed v2/send acceptance.

## Installed catalogue refresh and actual client — 2026-10-04

The verified source is now connected to a bounded metadata-only background
refresh at session startup and every five minutes. It shares the session guard;
no extra listener/login was added. Partial snapshots retain prior entries,
selected policy is enforced, and source messages are not implicitly imported.
The existing catalogue-diagnostics resource exposes an executable `preload`
section with safe status/times/counts. Failed refreshes preserve previous success
and stored metadata. Tests prove metadata refresh creates no messages, identities,
novelty facts, Events, deliveries, subscriptions or send operations, and closes
with the existing service session.

The installed checked local candidate SHA-256 is
eba9ceb6c54ea6b156339efbed787023ff7c2edead6db590e156932681a315fe
(base 45275b8, modified source; signed publication remains pending). A full
verified pre-update backup retained the original 47 messages/identities and two
subscriptions/send operations. Startup refresh observed and permitted 57 source
entries; 39 catalogue rows had preload_catalog as their primary provenance.
Source messages were not imported and completeness remained false.

The actual connected client successfully listed a 50-entry catalogue page,
including 34 preload entries, with has_more=true and preload_catalog in its
source summary. The specific requested peer could be selected through this
client, but a keyword-free browse of the requested September 30 local day
returned no_records_in_period, zero messages and history_complete=false. Its
current locally known metadata does not prove its earlier Strangers state.
Therefore directory access is verified, while older-thread recovery remains
incomplete. An explicit limited import of available preload messages and a
proven deeper-history source are still required; the catalogue refresh does
not substitute for either. Full root tests, changed-package race/vet and the
native no-CGO build passed; research-skill validation and diff checks passed.

## Explicit preload import — 2026-10-04

The import contract now accepts optional source=conversation_preload. Omitted
source and explicit group_cloud preserve legacy effective arguments and journal
fingerprints. Changing the source for an existing UUID conflicts. A preload
operation uses the existing durable worker/account guard, selects only the typed
dialogue within record limits, saves history silently and always stops
partial/source_window_limited with null continuation and history_complete=false.
The storage boundary rejects source evidence inconsistent with the chosen source.

Tests prove stable typed selection, no mutation of a raw snapshot, missing versus
empty source categories, a durable direct-source operation without group fallback,
silent storage, UUID deduplication and default-source compatibility. Production
HTTP service tests exercise both group paging and direct snapshot imports through
one restored session/listener, tool polling/retry/context and Event diagnostics.
Root tests, affected-package race/vet, no-CGO macOS arm64/Linux amd64 builds and
both skill validators passed.

The installed local candidate SHA-256 is
7beba3c5e876b408df3fc2b4f0e7b8e9248c7ee482b68f59c82fd69dffcef0c8
(base 45275b8, modified source; publication remains pending signature). The local
HTTP catalogue exposes the source parameter. The requested peer's September 30
local-day operation observed one source record, outside that interval: zero
insertions and partial/source_window_limited. A separate broader September-to-
October interval observed the same already stored message: one duplicate, zero
insertions. Retrying each UUID returned its original operation. This proves
source selection and live duplicate handling, not recovery of the missing older
message or new-record live insertion. The corpus remains 47 records/identities,
the subscriptions and send ledger remain two each, SQLite integrity is ok and
message_events remains zero. Remaining deep-history/client-discovery/v2/send and
publication gates are not closed by these checks.

## Group-source codec failure resolved — 2026-10-04

A new bounded live group read was initially reported as API error code zero.
Code review proved the SDK also manufactures this code for local parse failures.
Resolving the encrypted envelope separately from the group leaf identified
invalid_group_history_hasMore on a second live read, without exposing values.
The executable source contract now accepts booleans or exact integer 0/1 flags,
retains missing/null as unknown and rejects other shapes. Nested encrypted wire
and normalization tests cover this boundary and safe fixed field diagnostics.

After deployment, the first attempt hit a distinct network failure. A fresh
bounded operation then successfully read one group-cloud record, one page and
available_source_exhausted, with zero insertions. The record was already stored.
This proves real group source readability and duplicate handling after the codec
fix, not complete history or a live multi-page traversal. Root tests and changed-
package race/vet plus nested group API race/vet passed. The checked installed
local candidate SHA-256 is
2c580bc625a4024611c2fe5b3006026bce3353220c9816a9cf386ee6485b9153
(base 45275b8, modified source; signed publication is still pending).

SQLite integrity remained ok, with 47 messages/permanent identities, two
subscriptions, two send operations and zero message_events. No send, join,
subscription mutation or second authenticated session was introduced. Native
static routing evidence shows group cloud admission is gated to group-prefixed
IDs or special Send to Me/OA paths; ordinary direct history still needs a proven
independent source. The specific missing September 30 direct message remains
unrecovered, and source exhaustion must not be presented as full coverage.

## Phase-aware group candidate — 2026-10-04

Native static backward paging was traced to returned lastMsgId/isOld, rather
than a phase inferred from cursor magnitude. The source candidate now exposes
an optional phase-aware SDK method and guarded application port. The worker
uses durable status.is_old for the next request. The journal keeps decimal
operation cursors compatible, but keys seen old-phase continuations separately.
A missing production phase stops as partial/missing_continuation, including
resumed saved operations that lack phase. No schema or request-fingerprint
migration is introduced; historical writes remain silent.

Synthetic checks cover explicit recent/old encrypted HTTP routing, exact
cursors beyond 2^53, recent/0 to old/0, old/0 repetition after reopening the
database, missing production phase and worker propagation. Race checks passed
for storage, worker, collector, adapter, service and the SDK group suite. Wire
and decompressed JSON bounds also reject oversized valid-prefix padding.
These checks do not prove live old-page availability. This phase-aware source
is not installed yet; the existing installed candidate/state are preserved.

## Installed phase candidate — 2026-10-04

The full root and nested-module race suites and vet passed. No-CGO Darwin
arm64 and Linux amd64 builds passed. The phase-aware Darwin candidate
SHA-256 ae2e50da7faf5aced1ab11155383476d88268249945e702a7081f2bd2cb9111d
was installed through the existing LaunchAgent after verifying a stopped-state
backup at the private migration checkpoint group-phase-candidate-20261004T133831Z.
Its exact modified source snapshot contains 508 files and archive SHA-256
99dad57a6a86b4e1b36c05f0beaf79dcc009b67562278dcf6cadbf15831f5c59.
Post-update SQLite integrity passed; retained messages/identities=47 each,
subscriptions=2, send operations=2 and message Events=0. The actual connected
client reported authenticated=true, connecting and last_error=null during
session restoration. Final connected state and live deeper traversal still need
the pending source check; installation alone does not establish them.

The bounded live group check completed with one observed record, no insertion,
is_old=false and source_has_more=false; stop=available_source_exhausted. There
was no source failure. This verifies the installed recent route, but the source
did not offer an old-phase continuation, so getoldv2 live acceptance remains open.

The subsequent actual-client status confirmed authenticated=true and
collector_state=connected, last_error=null, stored_message_count=47. Session
restoration after the candidate update is verified.

A second previously collected group was checked with page_size=1, max_pages=3
and max_messages=3. It also returned one terminal recent-phase record, zero
inserts and no source failure (has_more=false, is_old=false). Neither real
group offered phase continuation; the old-route live gate remains open.
No notification Event was produced by either explicit historical import.

The optional repository skills are not copied into either local
~/.codex/skills or ~/.agents/skills; no existing installed copy was overwritten.
Repository revisions and structural validation do not prove that an external
agent has loaded them. The actual client's skill discovery remains separate
from MCP discovery and needs evidence from that client.

## Offline mobile header component — 2026-10-04

The primary native format boundary was narrowed further: read_header limits
names to 128 bytes, and its checksum state constants match XXH32 seed zero.
An independent internal/mobilebackup plaintext-header parser now implements
the [bounded candidate contract](contracts/mobile-backup-header.md). It reads
only the header, checks checksum and table occupancy, retains exact decimal
filenames, and rejects traversal, duplicates, invalid count/size, truncation
and resource overflows without leaking input in errors.

Eleven checksum vectors spanning short inputs/stripes/tails were generated
independently with the official xxHash v0.8.3 reference implementation; only
synthetic byte tables and expected digests are retained. Parser race and vet
passed. There is no new dependency, native addon execution, payload extraction,
mobile transfer, archive key or real account fixture. The component is not
wired to the service or exposed as an MCP source. Its success does not close
real decryption, decompression, schema/identity mapping or the missing older
direct-message gate. The installed phase candidate remains unchanged.

## Offline mobile block transform — 2026-10-04

Static inspection confirms that format 1 requests a hexadecimal textual key,
uppercases it, uses AES-256 with the text bytes directly, and passes a zero IV
into each 65536-byte decrypt call. The CBC function copies that IV into local
storage rather than changing the caller's IV. The bounded internal transform
is defined by [the block contract](contracts/mobile-backup-blocks.md), without
ciphertext-span guessing or alternative-key/cipher fallback.

An independently encrypted OpenSSL synthetic vector crosses the chunk boundary
and decrypts to the exact expected bytes, including a valid synthetic header.
Tests cover wrong/invalid keys, byte alignment, budget overflow sentinel and
pre-cancelled input without reads. Race/vet passed for the offline package.
No archive was transferred, no real key was obtained, no compressed payload was
extracted and no corpus/subscription/send state was changed. No additional
dependency or service integration was introduced. These component checks do
not prove a real mobile archive decodes or contains the missing older message.

## XZ boundary and current root checks — 2026-10-04

Native stub resolution identifies `lzma_stream_decoder` with concatenated-XZ
flag 0x08. This establishes the compressed container independently of the
unlicensed reference fork. Go candidate v0.5.17 was fetched with checksum
verification, but source inspection shows DictCap is a minimum, not a memory
ceiling, for XZ filter decoding. It was not added to go.mod or the runtime.
The required allocation guard is described in the research report; no real
archive or message recovery is claimed.

After the offline header/block additions, root `go test -race ./...` and
`go vet ./...` passed. `git diff --check` also passed. These checks cover
the current source and synthetic fixtures, not external-client discovery,
live mobile transfer, missing old-message recovery or pending outgoing sends.

## Bounded offline XZ component — 2026-10-04

The independent [XZ contract](contracts/mobile-backup-xz.md) now has a Go
implementation. Before starting the pinned BSD-3-Clause XZ reader, it walks
every concatenated stream's index/footer/header, validates structural CRCs and
exact block positions, and rejects unsupported filters or dictionary sizes
above 64 MiB. Total streams/blocks and declared/actual decoded sizes are bounded.
Failure returns no partial output or underlying decoder detail. No archive file
is extracted and no corpus/Event operation is performed.

Independent Python/liblzma vectors cover empty output and all four supported
check types; XZ Utils 5.8.3 supplies a multiple-block vector. Recomputed block
CRCs with excessive dictionary declarations are rejected by preflight alone.
Concatenation, padding, truncation, content integrity, cancellation and budgets
are checked. Preflight fuzzing passed 115577 executions in a bounded five-second
run. Root race/vet passed; no-CGO macOS arm64 and Linux amd64 builds passed.
These are offline format checks, not proof of real mobile archive compatibility.
The installed service remains the prior phase-aware candidate; the offline
decoder is not wired into its source registry or exposed through MCP.

## Preload overflow guard and mobile transport audit — 2026-10-04

The preload reader previously passed LimitReader directly to JSON resolution.
That could accept a valid encrypted response prefix followed by excessive
whitespace. Both wire and expanded bodies now use budget+1 reads, explicitly
reject overflow and only then resolve JSON. Independent plain/gzip regressions
exercise otherwise valid encrypted pages with excessive padding. SDK race/vet,
root race/vet and no-CGO macOS arm64/Linux amd64 builds passed. The current-tree
redacted gitleaks scan found no leaks. The updated reader is not installed yet.

Static mobile transport inspection confirms syncmsgmb controls are currently
ignored by the Go listener and need an existing-session correlated route.
Native confirmation distinguishes rejection/restoring/confirmation and matches
the request key/host. Native nonzero sequence advancement also differs from the
reviewed PR. No phone transfer or cancellation was requested and no encrypted
archive/key was downloaded. These findings specify the next implementation
boundary; they do not close real mobile history or missing-message acceptance.

## Optional existing-listener sync receiver — 2026-10-04

The [mobile control contract](contracts/mobile-sync-control.md) is implemented
as an optional concrete SDK extension. It preserves the public Listener
interface and registers one receiver correlated to account/key/host on the
existing listener. It does not start a listener or send a transfer request.
Sync controls accept bounded object/string data, preserve exact decimal IDs and
confirmation state, and exclude sensitive fields from JSON/default formatting.
Unknown actions are ignored. Invalid sync data does not discard other controls
in its frame. Active-receiver failure and queue overflow are fixed separate
errors; inactive receivers do not alter collector error behavior.

Synthetic tests passed for actual 1/601/0 router dispatch, foreign correlations,
confirmation/restoring states, uint64 precision/bounds, URL syntax, redaction,
whole-data rejection, stale destination clearing, queue order/overflow and
unsubscribe races. Nested and root race/vet passed; no-CGO macOS arm64/Linux
amd64 builds passed; current-tree redacted gitleaks scan found no leaks.
These checks exercise source code, not real phone controls. No receiver is
registered by the service yet, no HTTP transfer/cancel request was made, no
archive/key was obtained and the installed binary remains unchanged.

## Initial mobile HTTP request candidate — 2026-10-04

Native public-key generation confirms RSA-2048 SPKI DER/base64. The optional
[initial request/cancel contract](contracts/mobile-backup-request.md) now has
SDK implementation, preserving the public API interface. It uses current
session/file service, fixed initial zero sequence/retry values, validated public
key, ten-second budget, no application redirect/retry, and bounded strict
acknowledgement parsing. It preserves explicit HTTP 401 and numerical upstream
codes while excluding raw response/network detail. Ambiguous dispatch results
are unknown, never proof of failed execution or authorization to repeat.

TLS synthetic encrypted-wire checks passed for initial and cancel routes;
input validation, pre-cancellation, missing source, redirect avoidance, HTTP
auth/server errors, malformed acknowledgements and plain/gzip valid-prefix
overflow were checked. Root/nested race and vet passed; no-CGO macOS arm64 and
Linux amd64 builds passed; current-tree redacted gitleaks scan found no leaks.
No real mobile call, user prompt, cancellation, download or key exchange was
performed. The methods are not connected to service operations and the installed
binary remains unchanged. The missing historical message gate remains open.

## Guarded mobile offer operation candidate — 2026-10-04

The [bounded offer contract](contracts/mobile-backup-offer.md) now joins
ephemeral RSA generation, preregistered existing-listener receiver, one initial
HTTP dispatch and bounded asynchronous waiting. Unknown acknowledgement waits
for the original correlated response without retry. Confirmation/restoring/
rejection remain distinct. Format-1 metadata/account is checked before RSA
PKCS#1 v1.5 key decryption; the internal offer is redacted and not a tool result.
The collector session guard cancels active waiting and refuses new dispatch
after authentication failure; error paths return no partial sensitive offer.

Synthetic acceptance covers replies during HTTP dispatch, unknown acknowledgement
followed by a valid offer, exact sequence/key rendering, rejection, queue failure,
unsupported format, wrong account, malformed ciphertext, pre-cancellation and
shared authentication revocation. Root race/vet, no-CGO macOS arm64/Linux amd64
builds and current-tree redacted secret scanning passed. No live request or
archive download occurred. This adapter is not called by service operations;
durable operation state, trusted-local entry point, download-host policy,
archive/database validation, silent import and real recovery remain open.

## Mobile attempt journal candidate — 2026-10-04

The [journal contract](contracts/mobile-backup-journal.md) and executable
mobile_backup_attempt request/status schemas now have a schema-9 SQLite ledger.
It normalizes scope/window/budgets, preserves UUID identity, binds account/policy,
limits one active account attempt and 10000 retained attempts, and CAS-records
the public request key before dispatch. It stores no archive URL/private key/key
text/message payload. Startup recovery is explicit and makes active attempts
interrupted; the original UUID cannot redispatch. offer_ready concerns only
the phone attempt and never claims successful import.

Tests passed for normalized retries/conflicts, parallel-attempt rejection,
stale/invalid transitions, reopen/recovery, terminal UUID preservation, key
redaction, status-schema validation, revoked policy/account, ledger capacity
and transactional migration rollback. Root race/vet and no-CGO macOS arm64/Linux
amd64 builds passed; later capacity/schema test additions also passed focused
race checks. Current-tree redacted secret scan found no leaks. Existing message,
Event/subscription and send tables are not mutated by journal actions.

The source can create schema 9; the installed service still has its previous
schema/build and has not been migrated. No real attempt was created. The journal
is not yet joined to the adapter's pre-dispatch hook, service/CLI operations or
download/import pipeline. These remain required before a live phone request.


### Mobile backup journal/adapter binding (offline, 2026-10-04)

The internal offer adapter now accepts an error-returning observer. Before HTTP,
it passes the generated public key to BeforeDispatch; storage commits dispatching
with revision CAS. A failed commit prevents HTTP. Progress commits also return
errors: waiting ends and even a valid offer is discarded after failed persistence.
Cancellation is checked again after the dispatch commit. The storage observer
serializes callbacks and retains the latest committed revision; a competing stale
observer cannot dispatch the same attempt again.

Synthetic checks cover predispatch failure (zero requests), acknowledgement-state
failure, offer-ready failure, cancellation during commit, durable progress and
stale-observer rejection. This is internal wiring only: the installed service and
schema remain unchanged. No phone request, download, import or Events occurred.
The trusted-local operation entry point and complete archive validation/import
pipeline remain open.


### Bounded archive download candidate (offline, 2026-10-04)

Added a trusted-host-only HTTPS downloader with exact correlated size checks,
120-second total bound, no redirects/proxies/cookies/credentials and fixed errors.
The transport resolves at dial time, rejects mixed public/private DNS answers and
dials validated addresses. The same unchanged network policy now serves Events
through an internal shared package. No actual Zalo download host is presumed: an
independently verified host allowlist is required before production use.

Synthetic tests cover exact/chunked bytes, short/oversized bodies, mismatched
Content-Length, encoding/status rejection, pre-network URL/budget/cancellation
checks, close/read bounds and production transport properties. Mobile-backup and
Events race tests and targeted vet passed. This component is not service-wired;
no phone action, archive download or installed-state change occurred. Full archive
framing, SQLite/account/plain-noise mapping and silent message import remain open.

Validation after the download/shared-policy change: root `go test -race ./...`,
root `go vet ./...`, explicit mixed public/private DNS rejection tests, CGO-free
macOS arm64/Linux amd64 builds and `git diff --check` passed. A redacted current
tree gitleaks scan found no leaks (2.46 MB). These checks do not establish real
archive compatibility, deployment, external discovery or publication completion.


### Complete-container correction and offline splitting (2026-10-04)

Static native evidence corrected an erroneous earlier interpretation: the length
and XXH32 scope include the compressed payload, not merely the file table. Earlier
header-only acceptance statements are superseded by this checkpoint. The old
ReadHeader candidate was removed; its synthetic fixture is now explicitly only a
table-layout fragment, and the OpenSSL vector proves only block transformation.

ReadContainer verifies the entire bounded declared region, then parses the bounded
file table. ReadPlainArchive validates exact plaintext EOF, XZ allocation/container
checks and exact decoded-size sum before returning redacted, clearable in-memory
file slices. No path is opened and no message/Event is persisted. Tests cover
checksum corruption both in table and payload, truncation, declared/container and
expanded budgets, extraneous trailing bytes, invalid XZ, file-size mismatch, exact
large IDs, boundaries and owned-buffer clearing.

Encrypted outer framing/trailers and real SQLite/account/plain-noise mapping remain
open. Offline file splitting is not a verified history source; no installed binary
or retained data was changed.

Validation for the correction: full root race/vet, CGO-free macOS arm64 and
Linux amd64 builds passed. Redacted current-tree gitleaks found no leaks
(2.47 MB). This does not close runtime/client/publication or real-archive gates.


### Exact typed archive identity mapping codec (offline, 2026-10-04)

Native evidence located a separate getnuid mapping step rather than permitting
filename IDs to be reused as session conversation IDs. Added bounded input and
plaintext response codecs: exact JSON uint64 integers including values >2^53,
typed direct/group arrays, positional count validation, optional g normalization
only for groups, duplicate/ambiguous/overflow rejection and redacted formatting.
The codec accepts no partial mapping and performs no network/import/Event work.

Synthetic tests cover maximal uint64 values, same source ID in distinct typed
categories, empty unrequested categories, duplicate fields/targets, mismatched
counts, null/exponent/fraction/leading-zero/overflow forms, extra JSON and input/
reply budgets. Targeted mobile-backup race checks passed. Authentic current-
session request/reply validation, archive account binding and real SQLite row/
BinNet mapping remain open; installed state is unchanged.


### Existing-session encrypted identity request (offline, 2026-10-04)

Added the optional SDK GetMobileIdentityMapping path with exact request integers,
current session reuse, fixed observed /api/znoise endpoint, one dispatch, 10-second
budget, wire/expanded/decrypted caps and strict success-code presence. Duplicate
outer/inner envelope fields are rejected; HTTP 401 is the shared auth sentinel.
Synthetic TLS tests validate encrypted form payload precision and encrypted
response extraction, missing/failed codes/data, duplicate fields, oversized data,
redirect/no-retry, pre-network invalid IDs and cancellation. Returned plaintext
still requires the typed positional codec before any identity can be used.
No real mapping request, session/listener replacement, archive import or runtime
change occurred; root service/collector wiring and real mapping acceptance remain
open.

Validation: full root and nested zcago race tests/vet passed, along with CGO-free
macOS arm64/Linux amd64 builds. A subsequent focused test also rejected a gzip
response with a valid encrypted JSON prefix and oversized expanded suffix. These
are synthetic protocol checks, not evidence that the live Zalo route has mapped
any account or archive ID.


### Guarded identity request/codec integration (offline, 2026-10-04)

Connected the optional SDK mapping request to root Client and collector session
guard. Invalid requests never reach the SDK; the entire bounded reply must pass
typed positional validation before returning mappings. Client rejects account
changes and shared authentication failure cancels active mapping. Guard rejects
even an otherwise successful mapping/backup offer returned after revocation;
owned plaintext response buffers are cleared and upstream error details hidden.

Tests cover exact typed success, partial/mismatched replies, pre/in-flight
cancellation, account changes, missing account, shared-auth cancellation and
late successful responses after revocation. No real network request, service
operation entry point, SQLite import or Events was added. Installed state and
publication remain unchanged; real mapping/archive acceptance still open.

Validation after guarded integration: full root race/vet, macOS arm64/Linux amd64
CGO-free builds and diff checks passed. Redacted current-tree gitleaks scan found
no leaks (2.51 MB). No external client discovery, deployment or publication gate
is closed by these offline checks.


### Standalone SQLite candidate inspection (offline, 2026-10-04)

Added bounded read-only inspection of one caller-selected archive file. Official
SQLite header rules informed the standalone-file precheck; WAL versions are
currently rejected pending a real checkpoint/export contract. A private random
scratch directory holds only a fixed 0600 filename, opened immutable mode=ro with
query_only/trusted_schema/temp-store policy and per-connection length/SQL/column/
expression/attachment limits. Integrity/schema checks precede a bounded
[timestamp since, until) ChatContent query with one has_more probe.

Synthetic SQLite fixtures verify exact large IDs, inclusive/exclusive dates,
examined/rejected counts, has_more, source-byte preservation, private scratch
cleanup and no row JSON exposure. Header corruption/alignment/WAL, views, missing
columns/generated columns, cancellation and row budgets are rejected. Tests
initially caught a missing BinNet JSON exclusion; that field was corrected and
mobile-backup race tests passed.

Rows remain opaque candidates: BinNet is not decoded and neither SQLite success
nor mapping success binds a real archive to the current account. No messages,
Events or corpus writes occur. Real schema acceptance, encrypted outer framing,
account/operation binding, trusted-local service entry and selected silent import
remain open. Normal-return scratch cleanup is tested; crash cleanup still belongs
to the eventual operation layer.

Validation: full root race/vet, CGO-free macOS arm64/Linux amd64 builds and diff
checks passed. Redacted current-tree gitleaks scan found no leaks (2.52 MB).
These results cover synthetic standalone SQLite inspection, not real archive
compatibility, completed import, runtime deployment or publication.


### SQLite keyset continuation (offline, 2026-10-04)

Added ReadSQLitePage with private continuation bound to exact source SHA-256,
filename and millisecond period. Timestamp/rowid ordering traverses ties and
advances over rejected content rows; changed page size preserves the filter.
Cursor mismatch, out-of-period anchors, shadowed rowid aliases and inexact paging
key types fail before returning an invented continuation. Terminal pages return
no next cursor. A five-row tied-timestamp fixture with two rejected identities
traverses three pages without repeats or lost valid rows.

This remains an internal immutable-file reader, not a public authenticated cursor,
service operation, actual source acceptance or corpus import. Account binding and
real archive/row semantics remain mandatory before selected silent persistence.

Per-batch retained text/BinNet is additionally capped at 8 MiB, independently of
individual cell and row-count limits. A 17-row synthetic fixture exceeds that cap
and returns no partial rows or continuation; the same file remains readable with
a smaller page size. Scratch locations must be absolute and trusted.

Final checks for this continuation: mobile-backup race tests and targeted vet
passed, as did diff checks. Existing runtime, subscriptions and corpus were not
used or changed; these offline results do not close live source/import gates.


### BinNet structural TLV candidate (offline, 2026-10-04)

Native static evidence established 4-byte big-endian tags/lengths and repeated
fields. Added independent bounded parsing (256 KiB, 1024 entries), cancellation,
whole-stream length validation, copied owned buffers and redacted JSON/formatting.
Literal synthetic bytes check repeated/unknown tags, zero-length structural
entries, precision of tag bits and clear/input ownership. Malformed suffixes,
truncation, excessive length/count and pre-cancellation reject successful prefixes.

This does not yet decode quotes, mentions or attachment semantics. Structural
TLV success is not a valid-message or real BinNet compatibility assertion. No
message/Event import or runtime change occurred; operation binding and actual
archive acceptance remain open.

Validation: mobile-backup race tests, targeted vet and diff checks passed. A
5-second two-worker structural TLV fuzz run completed 14395 executions without
failures. These checks do not establish nested semantics, real metadata fixture
compatibility, deployment or message recovery.

### Quote scalar candidate (offline)

Native static getter/property evidence now has a separate scalar contract.
Literal synthetic fields cover an ID above 2^53, MaxInt64, signed negative/zero
values and unknown byte fields. The internal decoder preserves presence and exact
integers; malformed widths and duplicates fail without a successful prefix.
This is not real archive acceptance or account-bound quote import. Installed
service, subscriptions and historical notification policy are unchanged.

Validation for this checkpoint: `go test ./internal/mobilebackup -count=1`,
`go test -race ./internal/mobilebackup`, `go vet ./internal/mobilebackup` and
`git diff --check` passed. These checks cover offline candidate components only;
no live send, phone backup request, deployment or publication occurred.

### Quote byte-field candidate (offline)

Native getter/property branches establish msg/attach/fromD/quoteStatus labels.
Independent literal fixtures verify UTF-8, empty presence, embedded NUL, ownership,
redaction, unknown-field counts, malformed UTF-8 and duplicate fields. Failure
clears the successful scalar prefix too. `go test -race ./internal/mobilebackup`,
`go vet ./internal/mobilebackup` and `git diff --check` passed at this checkpoint.
The parser deliberately performs no JSON interpretation or domain quote import.
Live service and notification delivery remain unchanged.

`FuzzQuoteWholeResult` additionally passed 5 seconds with two workers and 37,485
executions, checking malformed-input handling and absence of partial results.

### BinNet envelope candidate (offline)

The contract and internal envelope connect top-level tag 7 to the quote decoder.
Literal fixtures cover nested exact IDs/text, unknown quote fields, repeated
unsupported attachments, duplicate quotes, malformed prefix/suffix, cancellation
and redaction/owned-buffer cleanup. Partial metadata is counted explicitly rather
than represented as complete. SQLite row conversion and account-bound silent
import remain open gates; this candidate is not wired into installed production.

Validation: `go test -race ./internal/mobilebackup`,
`go vet ./internal/mobilebackup`, `git diff --check` and the 5-second/two-worker
`FuzzBinNetWholeResult` run passed (37,815 executions). A hand-written fixture
initially declared the nested length as 36 instead of its 34 bytes; correcting
that fixture made the whole-envelope check pass. No parser relaxation was made.

The complete root module also passed `go test -race ./...` after this checkpoint,
including service/history import/Events/storage and protocol adapter tests. This
is offline acceptance and does not close the outstanding live gates.

### Prepared offer orchestration (offline)

A contract and internal runner connect the attempt journal to the existing source
observer. Synthetic tests verify journal-before-request ordering, no redispatch of
an offer_ready attempt, dead-context finalization after dispatch, preservation of
ambiguous outcomes, rejection of premature success and fixed errors without source
text/credentials. Concurrent tests cover both a retry after dispatch and two
observers attached before dispatch: the loser cannot finalize the winner's state.
Targeted `go test -race ./internal/mobilebackup -run PreparedOffer -count=1` passed.
The operation remains internal; no trusted CLI/MCP route or installed service has
been changed and no phone backup request was sent.

The complete mobilebackup package subsequently passed `go test -race
./internal/mobilebackup`; `go vet ./internal/mobilebackup` and `git diff --check`
also passed after orchestration and ownership changes.

### Service startup mobile-ledger recovery (offline)

Production service startup now invokes mobile-attempt recovery under its existing
state lock before HTTP/CLI readiness and session restoration. An integration test
seeds a dispatched attempt and reads its interrupted state from a separate read-only
connection in the readiness callback; identity/public key/revision remain intact.
No corpus, message Events, sends or subscriptions are created. Fresh unbound stores
start normally; missing account metadata with existing attempts is rejected.
Targeted service/storage race tests passed. Phone-request execution routes and real
archive import remain absent; installed runtime has not been redeployed.

Validation caveat at the startup-recovery checkpoint: the first combined service/
storage race run returned a service-package failure whose diagnostic lines were
truncated by the tool output limit; storage passed. A subsequent full service race
run captured as JSON passed (25.125 seconds, no failed tests). The first failure's
cause remains unverified and is not represented as a diagnosed or fixed defect.
`go vet ./internal/service ./internal/storage` and diff checks passed.

The repeated combined service/storage race command captured to JSON also passed
(service reused its preceding successful result; storage ran in 39.242 seconds).
The unexplained first-run failure remains a validation caveat for final acceptance.

### Mention scalar subset and service stability follow-up — 2026-10-05

The native-supported repeated mention subset has a contract and independent literal
fixtures for ordering, signed values, explicit zero presence, unknown fields,
redaction/clearing and whole-result failure on duplicate/wrong-width nested data.
`go test -race ./internal/mobilebackup` and `go vet ./internal/mobilebackup` passed.
No UID mapping or text-offset-unit assumption is made; service deployment/import
remain unchanged.

A full uncached three-repeat `go test -race -count=3 -json ./internal/service`
passed in 76.828 seconds with no failed tests. The earlier truncated first-run
failure did not reproduce; its cause is still unverified, not declared fixed.

The updated BinNet envelope additionally passed a five-second/two-worker fuzz run
with 49,777 executions and no failure. `git diff --check` passed.

### Existing-session mobile service port (offline) — 2026-10-05

The internal service port uses current collector sessionGuard rather than restoring
an independent backup session. An actual collector-lifecycle test verifies one
restore and dispatch, durable offer_ready evidence and rejection of operation-ID
retry. Stopped lifecycle/missing session checks perform no source/store work.
Targeted service race tests, service vet and diff checks passed. No public/trusted
CLI route is enabled yet; actual archive receipt/import and deployment remain open.

### Format-1 complete offline stage assembly — 2026-10-05

Native reader evidence establishes retained decrypted byte count and declared-end
checksum/XZ framing, with no PKCS unpadding. The independent internal assembly
connects existing block decryption to exact container/XZ/file validation, reports
bounded opaque trailing bytes and clears temporary decrypted data on every path.
Full fixtures were generated independently with official XXH32, Python/liblzma
and OpenSSL; they include aligned, zero-tail and nonzero-tail complete containers.
Targeted race tests, package vet and diff checks passed. No native code, actual
archive, account messages, download or installed runtime was used/changed.
Sender padding conventions are not inferred; real mobile archive acceptance and
selected account-bound import remain required.

The complete mobilebackup package passed `go test -race ./internal/mobilebackup`
after full encrypted-stage assembly and fixtures; diff checks passed again.

### Exact mobile archive file selection (offline) — 2026-10-05

The contract/internal selector preserves exact IDs beyond 2^53 and MaxUint64,
separates direct/group collisions and accepts mapping reply order independently
of source file order. Tests reject missing/extra/duplicate/ambiguous mappings,
invalid filenames/zero/overflow IDs, empty/duplicate file declarations and cancelled
contexts. A source ID equal to the requested session ID without a corresponding
mapping is unavailable, never a fallback. The selector returns a borrowed index
without opening other databases. Targeted race tests, vet and diff checks passed.
Real archive/account association and selected row import remain open; no installed
service or live data was changed.

The complete mobilebackup package passed `go test -race ./internal/mobilebackup`
after selected-file routing; `git diff --check` also passed.

### Selected staged fetch and revocation scope (offline) — 2026-10-05

Independent encrypted archive fixtures now exercise download→decrypt→container/XZ
validation→complete identity mapping→selected-file transfer together. Tests verify
one download/map, private result redaction/clearing, invalid selection before
network, ciphertext budget rejection, missing/extra mappings, hidden source errors
and cancellation. Collector scope tests establish that a shared authentication
failure cancels non-SDK stages and rejects later scopes. Targeted race checks for
mobilebackup/collector and diff checks passed. Service wrapper source ownership
and cancellation are internal only; actual trusted endpoint/host/phone archive,
SQLite conversion and silent corpus import remain unverified.

Follow-up full affected-package validation captured to JSON identified a concrete
service failure: TestServiceSendAndReplyUseCollectorSessionAndPersistAcrossRestart
expired its five-second startup readiness wait under parallel race-package load.
The test now records readiness duration, allows a bounded 15-second setup wait and
drains service shutdown on timeout instead of leaving cancelled startup running.
Send/idempotency/quote/restart assertions and production timing remain unchanged.
This does not establish the cause of the earlier truncated failure or diagnose a
production defect.

A fresh uncached `go test -race -count=1 -json ./internal/mobilebackup
./internal/collector ./internal/service` passed all three packages (10.234,
56.996 and 40.121 seconds respectively). The sending integration's four observed
startup readiness durations were 1552, 399, 312 and 372 milliseconds. Vet for all
three packages and diff checks passed at this checkpoint.

### Mobile row-page preparation (offline) — 2026-10-05

Contract/internal preparation follows native status/type evidence and preserves
raw payload kind and exact string IDs. Synthetic tests verify complete sender
mapping, incoming/outgoing identity, separate control/type/metadata counters,
private output ownership/redaction, malformed/partial/ambiguous mappings and whole
failure after cancellation. Direct third-party senders cannot enter the selected
peer's candidate page. Whole-page scalar/interval validation and aggregate/request
budgets precede mapping. Targeted race tests, vet and diff checks passed.
This is not domain message conversion or real source acceptance; no installed
service, corpus or Events were changed.

The complete mobilebackup package subsequently passed `go test -race
./internal/mobilebackup`; package vet and diff checks passed after the request,
record-count and aggregate-budget tests.

### Checked archive TTL calculation (2026-10-05)

Read-only installed-client tracing establishes millisecond TTL, preserved through
format-1 import, and server timestamp plus TTL expiry. Added an exact integer
helper and whole-row-page expiry validation before sender mapping. Zero TTL stays
undeclared; missing timestamp, negative TTL and addition overflow fail without a
clock fallback. Tests cover integers beyond JavaScript's exact range, the maximum
int64 sum, invalid values and no mapping on an invalid page.

`go test -race ./internal/mobilebackup`, `go vet ./internal/mobilebackup` and
`git diff --check` passed. This is synthetic calculation/page validation, not real
archive acceptance or deployment. Expired-content filtering, ongoing deletion,
quote expiry and undo/delete control restoration remain required before durable
historical import. Historical import must not emit notification Events.

### Candidate expiry eligibility (2026-10-05)

Added `PreparedRowPage.ApplyExpiry` with an explicit clock and all-before-mutation
validation. Synthetic tests cover the exact expiry boundary, future/non-expiring
survivors, independent quote expiry, clearing both parsed quote payload and raw
BinNet, retained coverage counters, repeat checks, invalid clock/missing quote
timestamp/negative TTL/overflow without partial mutation. This does not close
real archive, deletion-control, persistent cleanup or deployment gates.

### Composed selected-archive page processing (2026-10-05)

Added an internal bound reader composing selected immutable SQLite keyset paging,
whole-page sender mapping and candidate expiry checks. A synthetic SQLite archive
contains tied timestamps, an expired row, a rejected sender, a deferred control
and ordinary survivors. Traversal advances through an empty candidate page and
reports all five examined rows rather than only two survivors. Request budgets
count source rows and preserve has-more without an over-budget continuation.
Tests reject changed request UUID, account, conversation and exact file bytes;
failed mapping/cancellation return no prefix, and scratch/source ownership is
checked. Selected archive request binding comes from the existing fetch stage.

This is an internal composition with no public route, download, phone request,
persistent corpus import or notification Events. It does not close real archive,
control/attachment semantics, durable expiry or deployment acceptance.

Validation for this composition: `go test -race ./internal/mobilebackup`,
`go test -race ./internal/service -run Mobile`,
`go vet ./internal/mobilebackup ./internal/service`, and `git diff --check` passed.
These package-scoped checks are not the final full-tree/release verification.

### Full-tree offline checkpoint (2026-10-05)

After selected-archive page composition and expiry changes:

- Root `go test -race -json ./...`: 16 package completions, 581 passing test events,
  no failed package or test events; process exited successfully.
- Nested `third_party/zcago` `go test -race -json ./...`: 6 package completions,
  101 passing test events, no failed package or test events; successful exit.
- Root and nested `go vet ./...`: both successful.
- No-CGO `cmd/zl-mcp` builds for macOS arm64 and Linux amd64: both successful.
- Gitleaks 8.30.1 current-tree scan (2.68 MB): no findings. Main history scan:
  17 commits, no findings. No new commit was created; repeat scans after further
  source changes and verify any new commits before publication.
- Relative Markdown file links under docs/skills: no missing targets. This check
  does not validate remote URLs or heading anchors. `git diff --check` passed.

Private test logs, exact file-hash manifest and candidate binaries are under the
local temporary directory, not included in publication. This checkpoint neither
changes the installed service nor closes actual client/live/archive/signing/CI
gates. The old installed schema remains separate from source schema 9.

### Skill installation checkpoint (2026-10-05)

Updated research history guidance to depend on actual operation coverage rather
than a stale claim that phase-aware code still needs installation. Clarified the
Events processing-state reference with silent historical import versus recovered
live/replay examples: sent_at alone cannot determine subscription eligibility.

Both source skills passed skill-creator structural validation. Default local
Codex skill directories were absent; installed both complete source directories
using the documented manual-copy layout and verified every file by SHA-256
(research: 5 files; Events: 3 files). Installed copies also passed structural
validation. This establishes files on disk, not discovery/reload in the current
client or behavioral/model evaluation. Client-loaded copies remain an open gate;
MCP configuration, subscriptions and the service were not changed.

### Actual client discovery checkpoint (2026-10-05)

The current desktop session's available-skills catalogue now includes both
`researching-zalo-groups` and `handling-zalo-message-events` at their installed
local paths. This proves this client's skill discovery after the file install;
no behavioral/model evaluation was performed and other clients are not inferred.

Read-only calls confirm 18 local MCP tools, including the three history-operation
tools, and all legacy/conversation-v1/conversation-v2 event names in local
`events/list`. The actual connected client still exposes 15 tools; the three
history-operation tools are absent from its callable catalogue. Its real
`zalo_get_status` call succeeds and reports authenticated/connected, 47 retained
messages and no last error. No subscriptions were changed or sends/phone requests
made. Local v2 discovery does not prove external client Events discovery.

### Local mobile attempt ledger wiring (2026-10-05)

Added executable envelope/response contracts and owner-only control routes for
preparation, exact status and revision-checked cancellation of prepared attempts.
Added bounded stdin CLI commands preserving raw JSON for server duplicate-key
validation. CLI commands reuse the service socket and do not prepare runtime
directories, restore a session, download, import messages or create Events.

Targeted synthetic race tests for mobile/service/CLI/contracts passed after fixing
test inputs to omit optional budgets rather than submit explicit zero. Explicit
zero remains invalid under the input schema. Coverage includes stable UUID retry,
argument conflict, cancellation CAS/state boundary, account ownership, unknown
fields, duplicate keys, trailing JSON, body budgets and no unrelated corpus/event/
send/subscription writes. Local routes are absent from MCP tool names. A production
Unix-socket lifecycle test and full affected-package check are tracked separately.

No command was run against the user's installed service, and no phone request or
new installed binary was created in this step. Actual mobile dispatch/import and
real archive gates remain open.

The production owner-only Unix control-socket test passed in auth_required with a
known account: preparation/read/cancellation reuse one service, and socket mode
is 0600. Full `go test -race ./internal/service ./cmd/zl-mcp ./docs/contracts`
passed (service 27.113 s), as did affected-package vet and diff checking. Gitleaks
current-tree scan (2.72 MB) found no leaks. This supersedes only the affected-source
checks; earlier full-tree build hashes refer to the preceding source checkpoint.
No phone dispatch or deployed CLI acceptance is implied.

### Current-session scoped archive transport (2026-10-05)

Added an optional SDK request/body-consumer port, root adapter and shared-session
guard, plus a bound downloader copy used by the internal service fetch path.
The supplied validated transport is retained; cookies come only from the current
jar for the exact URL. Archive responses cannot update the authentication jar;
no redirect, ordinary API transport fallback, cookie getter or second listener is
added. Full body consumption stays inside guard cancellation/account ownership.

Synthetic tests verify cookie host/path scope, original request/client/jar
preservation, no redirects, injected credential-header rejection (including
noncanonical casing), exact downloader body/encoding/host bounds, and clearing
failed/account-changed/revoked/cancelled results. Targeted root race checks for
mobile/download paths and nested SDK archive race tests passed; affected-package
vet checks are recorded separately. These are transport invariants, not proof that
a real offered archive can be downloaded or decoded. No phone operation, cookie
extraction, runtime change or actual archive download was performed.

Root affected-package vet and nested API vet passed. The final nested archive
cancellation test also passed under race. Source-tree Gitleaks and diff validation
must remain paired with the exact source checkpoint before deployment/publication.

### Atomic expiring operation pages (2026-10-06)

Added the trusted storage port CommitExpiringHistoryOperationPage. It derives one
message list from bounded expiry records and reuses the existing operation's
owner/source/interval/budget/revision transaction. Silent message/identity writes,
original TTL markers, due-expiry cleanup and source checkpoint now commit together.
The prior unexpiring page path retains its original behavior.

Synthetic storage/history-worker race tests passed. New tests cover checkpoint
failure rollback across messages/identities/TTL, restart with preserved progress,
duplicate records without lifetime extension, cancellation preventing writes,
invalid out-of-interval TTL evidence and rejection of dual message lists, and
atomic hiding of already due records. Root vet, no-CGO macOS arm64/Linux amd64
builds, diff checking and current-tree secret scanning passed.

This is an internal persistence prerequisite, not production mobile import.
No public source enum, archive ownership/cache, phone request, downloader host,
real archive acceptance or mobile continuation is added. The installed recovery
service remains unchanged. Full mobile content/control and selected archive
checkpoint integration are still open; the original goal is not completed.
