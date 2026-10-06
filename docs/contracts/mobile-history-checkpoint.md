# Atomic selected-archive history checkpoint

Status: internal journal port; public start still accepts only existing sources.

A trusted mobile driver prepares a history operation with source `mobile_archive`.
It shares the existing request UUID/fingerprint, account, typed selection, limits,
cancellation, permanent identities and silent history policy. Public request
normalization continues to reject this source until driver/producer acceptance;
this internal entry point is not a phone request or another listener. Legacy
workers exclude mobile operations rather than accidentally call group history.

Persist one private source binding: request/snapshot UUID, image SHA-256, creation
and fixed 15-minute expiry, immutable source row/period/invalid-timestamp counts.
No path, transfer URL/key, account ID or body is part of a checkpoint. The driver
obtains this evidence from its authenticated selected snapshot. Known unverified
WAL images remain ineligible. Non-WAL control images are eligible only for the
fully classified own-direct recall case below. A source cannot change image, expiry or
coverage after its first committed page. Missing/expired sources cannot be
replaced by another phone transfer under the same UUID.

Each page owns expiring normalized records and exact examined/rejected/expired/
unsupported-type/content/missing-metadata/invalid-metadata/deferred-control counts.
Examined rows equal retained records plus these mutually exclusive gaps. Unknown
metadata fields, expired quotes and unresolved quotes/mentions are orthogonal
counts and never consume additional source-row budget. Validate nonnegative
bounded counters, complete records and original expiry before writing anything.
Every submitted record must lie in the requested interval.

Use the remaining source-row budget and page size even when zero records remain
eligible. A nonterminal page needs positive examined rows and a strictly advancing
SQLite (timestamp milliseconds, signed rowid) keyset in the requested interval.
A page reaching the operation page/record limit may omit further continuation:
it stops terminal partial while preserving source_has_more=true. Source exhaustion
must agree with the frozen period-row count. Retain integer precision; neither
component passes through float64. The opaque
source checkpoint is private, not the generic cloud decimal cursor. A terminal
empty page is allowed; continuing empty source evidence is rejected.

The transaction validates operation account/revision/running state and policy,
then commits silent records, original TTL markers, immutable source binding,
raw-row counters and next keyset together. A failed checkpoint write rolls back
messages, expiry, permanent identities, novelty and all progress. A stale or
cancelled worker cannot write a page. Policy revocation cancels without records.
Restart retains the exact source/checkpoint and counts; it never dispatches a
phone request. Replaying a committed page with its old revision fails.

Status exposes `records_observed` as examined source rows and safe cumulative
`mobile_coverage` counts. Source UUID/digest/position/expiry remain private.
Available-source exhaustion stops completed only when no known source gaps exist;
otherwise partial/source_gaps. Every result retains history_complete=false.
Limits stop partial with existing page/message/time reasons. Exhaustion never
claims complete Zalo history. Historical writes produce no Events or deliveries,
do not change subscriptions/sends and preserve observed tombstones/novelty.

Mobile active work has a separate bounded 420-second allowance, including durable
in-flight reservations up to 180 seconds for phone acquisition. Legacy sources
retain their existing 120-second/30-second bounds. Authentication waiting does
not consume active work. Source loss is partial/source_unavailable, never implicit
redispatch.

The internal snapshot page adapter reads only an authenticated encrypted snapshot;
it owns and clears the selected image after each bounded read. Creation and fixed
expiry come from authenticated snapshot metadata, never from the clock at retry.
It reconstructs the reader's private cursor from the trusted operation's examined
count and exact stored position, binding it again to account, request, selected
name, digest and interval. Resume requires both previous source and position;
the first page permits neither. It compares frozen coverage before returning a
page, maps all exclusive and orthogonal counters, and lends no archive buffers
to storage. The returned normalized page is owned by the caller and must be
cleared after commit or failure. Conversion gates apply before any writable page
is returned; known WAL/control source gates have a distinct unsupported-source
error instead of claiming a malformed SQLite page. Errors return an empty result.
This adapter performs no phone or
network operation and does not itself commit progress.

Producer eligibility and the single-session acquisition/worker driver still
require integration. A synthetic snapshot/reader/converter/journal restart test
does not prove live import or permit public source selection.

## Durable acquisition binding

Before any linked phone dispatch, the trusted driver claims the mobile history
operation and reserves its bounded acquisition work. One transaction validates
account, running state, revision and collection policy, then creates exactly one
mobile backup attempt and its permanent operation link. The attempt inherits the
same request UUID, conversation, interval and record budget; its archive-byte
budget is fixed by its own request fingerprint. Link creation failure rolls back
the attempt. An unlinked existing request UUID cannot be adopted implicitly.
Retries return the linked attempt, including terminal/interrupted state, and
never create a replacement under that operation. Changed byte budgets conflict.

The dispatch guard rechecks the linked history operation's source, running state,
reserved work and the exact revision saved at binding. Cancellation, account or
policy changes and recovery/reclaim therefore prevent stale phone dispatch before
the durable `dispatching` transition. Unlinked owner diagnostics retain their
existing behavior. Recovery marks interrupted acquisition as interrupted; the
driver must read an existing authenticated snapshot or stop source_unavailable,
not create another attempt. Cancelling/failing an in-flight attempt under the
same permitted account/scope remains possible for cleanup, without granting
another dispatch. The binding contains
only operation/attempt IDs and revision, not transfer URLs, keys or bodies.

## Internal worker and session port

The mobile worker consumes only its mobile queue under the service's account lock.
Its caller performs history/acquisition recovery once before starting workers;
the mobile loop never resets a running legacy worker. Public source selection
remains disabled until the production port and source acceptance are verified.

A trusted session port lends one account/session-scoped callback across receive,
download/save and all page reads. It returns authentication loss explicitly and
never creates another listener/session. The callback must follow parent and
upstream-session cancellation. Ports receive typed domain requests/checkpoints,
not paths, credentials or public tool arguments.

The production service implements this port using the current collector guard.
It holds the membership session lease across all stages, binds authenticated
archive transport and identity mapping to that account, and clears private scratch
on callback exit. Parent/service cancellation stops the borrowed scope; upstream
session loss returns authentication_required. No new listener or login is created.
The private snapshot store lives at `STATE_DIR/mobile-snapshots`, under the same
service state lock, and closes only after collector and worker shutdown.

History recovery completes synchronously before either worker starts. The legacy
worker consumes an already recovered queue; it must not recover again while mobile
work is running. Every collector session starts one mobile loop and waits for it
to stop before clearing the membership port. A worker storage failure terminates
the session with a fixed public error, without exposing private source errors.

An operation without a link reserves at most 180 seconds for its one phone offer,
then creates the acquisition binding before invoking the receiver. It reconciles
actual offer work transactionally, reserves at most 120 seconds for authenticated
download/snapshot publication, and reconciles that stage separately. Each page
has a reservation of at most 30 seconds and commits its work with its records and
checkpoint. All phases use remaining active work within the 420-second allowance;
the port receives the exact bounded stage context. Completion of non-page work
updates revision/work/reservation without message, source or page counters.

A linked operation only reads its existing snapshot, even if its attempt is still
prepared or terminal. It never calls the receiver again. Missing/expired source
stops partial/source_unavailable; unsupported source stops unsupported; malformed
pages stop failed/invalid_source_page. Authentication loss pauses without polling
the unauthenticated source; parent shutdown leaves running work recoverable.

The source also supplies local terminal-snapshot cleanup independently of its
upstream lease. Cleanup runs before queue processing and after each finished
operation, allowing a restart to remove an image left after a terminal journal
commit. It never removes paused/running sources or renews spent source identity.
The service also runs this cleanup during startup before collector authentication;
an unavailable phone session is not required to discard an owned terminal image.
Stale/cancelled revision results stop without additional source work. Error paths
clear owned pages/offers and never expose source error text or credentials.
Historical imports remain silent; source-row limits include rejected/expired-only
pages and source exhaustion retains history_complete=false.

## Atomic own-direct archive recalls

The paired live export establishes direct type-36/status-3 rows using the same
global ID as the recalled own message. A trusted source page may carry private
recall targets with exact typed conversation, ID, mapped current-account sender
and original source-row timestamp. It carries no text or presumed recall time.
`own_recalls` counts these rows as mutually exclusive examined records, rather
than deferred gaps. `source_controls` freezes the whole-file control count in
`mobile_coverage`; legacy checkpoints without it mean zero controls.

New source reads with controls first produce a [whole-source control prelude](mobile-backup-control-scan.md).
All type-33/36 controls must be verified before ordinary pages can commit. The
bounded scan supports own-direct type-36/status-3 controls across page and
requested-period boundaries. It remains limited to 5,000 controls and 30 seconds;
incoming/group, other-status, type-33 and WAL sources remain unsupported.
Legacy first-page source/checkpoint evidence retains its original bounded
semantics; missing prelude fields never grant a new whole-source proof.

Validate all recall targets, interval/keyset bounds, exact sender/account and
unique IDs before writing. A whole-source prelude uses validated global source
bounds for its targets; ordinary records still obey the requested interval and
keyset. Apply tombstones and existing record removal before
ordinary historical records, in the same transaction as identities, novelty,
expiry, source binding and operation checkpoint. Use local observation time for
`deleted_at`; the row timestamp is not evidence of the time of recall. Same-page
or later replay/history cannot resurrect a tombstoned message. No new Events,
deliveries or subscription/send changes result from this import. Existing pending
deliveries for a removed message follow the normal deletion policy.

The immutable source control count is recovered alongside its digest/keyset from
the same committed operation payload. Subsequent pages cannot change that count,
introduce another recall for an already fully classified source, or lose the
proof that all source controls were committed before ordinary records. Rollback,
restart, cancellation, revocation and stale-revision rules apply to recalls too.
Public mobile input stays disabled pending producer/WAL and remaining semantics
acceptance; internal synthetic success does not establish live import support.

Terminal or authentication-paused history closes any still-active linked attempt
in the same transaction that publishes its inactive status. Shutdown cannot
observe a terminal operation while its linked transfer remains active. Cleanup
failure rolls back both state changes; a pre-dispatch failure cannot block later
account transfers. Separate cleanup is idempotent recovery for legacy inactive
operations, not a required second step of a new terminal transition. This trusted
account-owned cleanup also works after collection revocation, but requires the
history operation to be inactive, grants no read/network authority and never
overwrites terminal acquisition evidence. Parent shutdown leaves running evidence
for startup recovery. Cleanup failure remains a storage error, not source success.
