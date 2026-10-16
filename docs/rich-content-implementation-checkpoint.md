# Rich content: historical implementation worklog

This file records successive historical stages, including failed and superseded
candidates. For the installed version and remaining gates, use the
[current acceptance matrix](rich-content-acceptance.md); later notes here do not
constitute whole-goal acceptance.

Date: 2026-10-15. Status: native candidate deployed; live and final acceptance
pending. The full objective remains [the task](task-rich-content-events.md).
[Deployment evidence](rich-content-candidate-deployment.md) records the concrete
binary, stopped backup, preserved data and current test stand.

## Initial implementation snapshot (historical)

- Message Events v3 and executable bounded content schemas; legacy/v2 wire bytes
  unchanged. Author text is distinct from labels and generated summary.
- Additive migrations 15/16, typed context/browse and JSON metadata resource.
- Separate activity source sequence, atomic activation/fanout, journal access,
  retention gaps/tombstones and shared retry/signing/ordered ack.
- Reaction observations share the one listener. Mixed replay preserves direct
  and group batches; durable channels apply cancellation-aware backpressure.
- Unknown reaction actions stay unknown. Call source is unsupported/unadvertised;
  video/contact and other ambiguous mappings are not inferred from names/text.
- Strict control shape recognition and control/message separation across web
  live/replay/history/preload. SDK array shapes are quarantined without invented
  deletion semantics; arbitrary object arrays stay unsupported content.
- Read-only `check-rollback` refuses unsupported active/unprocessed work,
  unfanned sources and retained activity payload that the old runtime cannot prune.
- Both skills distinguish typed content and activity from messages and preserve
  journal/authorization rules. Installed copies match; no cloud automation changed.

## Autonomous evidence

Full root test/vet, changed-package race, full nested test/race/vet, root lint and
CGO-disabled macOS arm64/Linux amd64 builds passed. A fresh full root test/vet and
build rerun passed after the final control-marker correction. Architecture checks pass at 100% compliance/coverage, including all isolated
negative controls. Full root race initially hit a shared 10-second fixture deadline
in the existing oversized-response test; its response-size assertions are unchanged,
fixture lifetime now follows the test context, and three focused race reruns passed.
That earlier candidate full root race rerun passed; the later schema-17 run and
its open failures are recorded below. Root/harness lint passed; the mandatory
offline profile passes 50/50 cases and 480/480 assertions, model/live NOT_RUN.
Existing synthetic CI checks are separate from model evals.

Regressions cover decoder union reuse/null/unknown shapes, title-less attachments,
title provenance through adapter/store/v2-v3/context/browse, message-free activity
activation/dedup/restart/retention, callback HMAC and journal equality/ordered ack,
maximum escaped legal activity recovery, and first media incoming after reaction.
Actual `resources/read` tests cover exact metadata plus namespace/policy denial,
recall and expiry before cleanup. Cross-version code from the exact prior revision
passed compatible new-schema writes/fanout/ack and an unsafe-v3 negative control
on disposable databases. It is not a launch against production state.

Independent evidence/contract reviews found and rechecked fixes for title-as-author,
mixed reaction replay, unsafe legacy fanout/retention rollback, control-marker
precedence and metadata-resource negative cases. Their narrow acceptances do not
constitute final acceptance of the entire goal.

## Remaining objective

1. Verify the agreed live batch end to end and obtain missing native-shape evidence.
   Fixtures and upstream names alone are not live acceptance. No second listener,
   new account or phone sync is required. No time limit on user test actions.
2. Complete source-specific mapping and invalid/control/retention/access checks;
   keep unavailable upstream functionality as explicit limits. Permanent mobile
   SQLite archives use a separate numeric parser, not proof of web media mapping.
3. Reconcile any agreed v2/v3 consumer cutover without gaps or repeated user actions;
   do not silently replace the existing production v2 subscription.
4. Verify new-event restart/recovery, clean up the test subscriptions/receiver and
   exact local callback allowlist, and preserve all new data/processing facts.
5. Complete documentation, final independent acceptance by both reviewers,
   commit/push and inspect green CI. No new model evals, OCR/STT/media downloads.

The installed candidate is connected/authenticated and original data/checkpoints/
signed payloads were preserved. The temporary incoming-only stand uses one known
test peer; outgoing sending remains disabled. Initial live results are recorded
below; final acceptance remains open.

The additional CGO-disabled macOS amd64 build also passed.

A repeated post-review Gitleaks v8.30.1 scan of the complete publishable worktree
snapshot (tracked and nonignored new files; private runtime excluded) found zero
findings across 12,609 files. Further code/evidence changes still require a fresh
scan before publication. The Events skill scope/profile correction was accepted
by independent static review and synchronized to the installed copy.

Harness tooling test/race/vet passed. The existing connector also returned
connected/authenticated with 92 stored messages and no last error after the native
upgrade. Both installed skills were rechecked by full tree hashes after the final
scope/profile correction; all files still match repository sources.

The exact installed candidate also passed the opt-in native terminal startup/
restart acceptance on isolated synthetic state without an authenticated Zalo
session. A separate [operator cutover runbook](rich-content-migration.md) defines
the pre-v3 fanout barrier and durable cross-profile consumer reconciliation;
it does not claim an automatic or completed switch of the working consumer.

The contract reviewer accepted the operator cutover preparation after two
clarifications: the barrier proves absence of unfanned events in a consistent
snapshot rather than equating counters, and concurrent old/new triggers require
serialization or atomic source claims. This is preparation only; no consumer
cutover has been claimed.

Before the initial live batch, production sources were rebuilt after test/doc
changes and matched the installed candidate hash exactly. Subsequent reaction
codec/diagnostic source changes require a new build and deployment; the earlier
hash does not identify those changes.

## Initial live batch and retained-journal restart

The user performed four direct-message actions and placed three hearts on the
text before removing the reaction. The independent local receiver accepted
eight callbacks after checking their exact-byte HMAC, timestamp and test
subscription/peer binding:

| Native observation | Count | Projection |
| --- | --- | --- |
| Emoji text | 1 | Author text preserved |
| Sticker | 1 | Sticker metadata, unavailable media bytes |
| Photo without caption | 1 | Image metadata, no text |
| Photo with user-confirmed synthetic caption | 1 | Caption retained as metadata label; author-text promotion not yet accepted |
| Reaction observations | 4 | Three nonempty icons, one absent icon; action unknown and no global targets |

Every message callback matched its journal envelope, stored content and actual
MCP metadata resource; typed context reads succeeded. Every reaction callback
matched its journal envelope and retained activity source. This proves delivery
and preservation, not unsupported action/target semantics.

With all eight records still unacknowledged, the same native service was
restarted. Journal pages and opaque receipts were identical before/after;
receiver body-file hashes were unchanged, the collector reconnected, and the
working production v2 subscription definition was unchanged. No ack was used to
hide a missing record and no test callback was regenerated.

Open evidence gaps: inbound photo-caption provenance beyond the observed title
field, and native reaction target shape. The confirmed SDK `rType`/`source` tag
bug is independent of the missing targets. New bounded shape diagnostics must
be verified on fresh native input before attributing that gap to the upstream.

## Diagnostic follow-up

The diagnostic native build is now installed (hash recorded in the deployment
report). Root and nested module tests passed; affected adapter/events and nested
model/listener race tests passed, as did root/harness lint. Earlier sandbox-only
cache-access failures were rerun with access to the normal Go build cache; they
were not treated as successful checks.

The new SDK diagnostics retain only fixed shape categories, capped reference
counts and field presence. They are excluded from JSON and deduplication identity.
The adapter logs no raw reaction icon, IDs, unknown keys/values or native payload.
Photo diagnostics record field presence and zero/positive/negative child-count
shape, without titles, descriptions, media URLs or arbitrary type/action values.
An independent evidence reviewer accepted this narrow change. A fresh native
observation is still required; stored normalized events cannot reconstruct raw
fields that were not retained.

After the diagnostic upgrade, all eight previously verified events were acked in
subscription order through actual MCP calls. Each ack was repeated immediately
and returned the same success. Both test journals then read empty with no delivery
block. Only the two local test subscriptions were acknowledged; the working v2
consumer was untouched. The local receiver/subscriptions remain active without
expiry for the targeted follow-up, and outbound Zalo sending remains disabled.

The contract reviewer independently read the private aggregate live/restart
reports and accepted delivery/preservation across restart only. This does not
claim live retention expiry, complete reaction semantics or final goal acceptance.

The next controlled heart/removal pair was performed from the main account,
as confirmed by the user. Both native observations were ingested as outgoing
for the exact test peer. Diagnostic shape showed one reference with positive
global/client IDs, native type/source present and no unknown content fields.
Each retained source has one target. The activity fanout checkpoint passed both
records, and neither created a delivery for the incoming-only test subscriptions.
This confirms outgoing reference decoding and direction filtering. It does not
resolve the missing targets in the earlier incoming observations; an incoming
pair from the test peer is still needed with the diagnostic build installed.

The user then performed the pair from the test peer. Two incoming observations
were received, independently signature-verified and matched against the actual
MCP journal and activity source. Both native reference arrays contained one
positive client ID but no positive global ID. This identifies an application
projection gap: the normalizer previously discarded client-only references.
It does not establish that a global ID was lost by the SDK.

The follow-up fix carries bounded private client hints to first persistence and
resolves only a unique visible match in the exact typed conversation. No new v1
wire fields or migration are needed. Guards cover other peers/namespaces,
ambiguity, recall/TTL, global-ID precedence, bounds and immutable replay.
The observed target text already has local client metadata. General media/group
coverage remains limited by available metadata.

Independent review found an upgrade deduplication issue in the initial fix:
adding client hints to an ID-less fingerprint changed earlier candidate IDs.
Compatibility now also checks the old fingerprint/tombstone before insertion.
Fixtures cover old-source replay after metadata appears and an expired source.
The evidence reviewer accepted that correction. Earlier signed events with empty
targets remain unchanged; this is a fix for newly persisted observations, not
retroactive enrichment or an already completed native live check of resolution.

The fix is now deployed (see deployment report for its exact hash). The affected
adapter/storage/events race suites and root/harness linter passed. Upgrade kept
the two pending incoming records and receipts byte-for-byte unchanged, retained
all ten verified callback files and the working v2 definition, and the collector
reconnected. A fresh incoming reaction remains the explicit live gate for the
new lookup; the earlier pair only proved the pre-fix failure and delivery.

The subsequent incoming heart from the test peer passed that live gate. Native
diagnostics again showed one positive client reference and no positive global
reference. The persisted activity and independently verified callback contained
one resolved global target, matching the original synthetic emoji message in
the exact local conversation. The actual MCP journal envelope matched both.
The event was acked and the ack repeated successfully; both test journals then
read empty without a delivery block. Production subscriptions were untouched.
This accepts the supported direct-text client-only lookup, not complete media/
group target coverage or add/remove semantics (`action` still stays `unknown`).

## Call investigation readiness

[Primary-source research](rich-content-upstream-evidence.md) separates outbound
call API claims from evidence of received call observations. The installed
diagnostic build now keeps bounded numeric protocol header counts and ignored
action/control counters. No raw payload or arbitrary action names are logged.
Tests and two narrow preparation reviews passed; runtime readiness and a private
baseline were verified. Counter comparisons must keep the same connection
generation. Call classification and the call Events profile remain unavailable;
the next agreed live action must establish what the existing listener receives.

## Private client aliases for media and groups: staged, not installed

While the call stand remains on its verified diagnostic binary, the worktree now
preserves native `cliMsgId` for all converted message kinds/namespaces. Additive
migration 17 stores exact bounded private aliases against the message sequence,
with FK cascade. This metadata is excluded from JSON and is independent of quote
eligibility; media/group aliases do not become send/quote anchors.

First insertion and matching-replay enrichment share the message transaction.
The typed conversation, global ID, sender, timestamp and visibility must match.
An alias cannot be overwritten; conflicting repeats increment a bounded private
counter without retaining the conflicting IDs. Resolution checks all matching
aliases and legacy quote metadata, refuses distinct targets or contradictory
metadata, and still does not rewrite an already retained activity/callback.
Historical import remains silent and does not grant quote authority.

Focused storage/native-path tests and race passed for direct media/group target
resolution, malformed IDs, typed collisions, contradictory metadata, rollback
of a partial message write, immutable aliases and recall/TTL/age-retention cascade.
The extended opt-in cross-version check executed the actual old revision on
temporary schema-17 data: it preserved an existing alias, physically removed an
alias through the old deletion code's FK cascade, and wrote legacy quote metadata
that the restored candidate still resolved. Both reviewers accepted the narrow
implementation. This is offline evidence, not native group/media acceptance.

Full root tests, root vet and root/harness lint passed. Architecture passed at
100% host/all-build compliance and coverage with all 14 isolated and 6 source
controls; only the standard `unicode` dependency was added to the domain's closed
allowlist. A Linux amd64 CGO-disabled build passed. Both installed skills still
match all six repository files each. The installed database remains on the prior
schema until a separate verified upgrade; no production restart or migration was
performed during this work. Read-only MCP checks still showed connected/authenticated
with empty, unblocked test journals. The call comparison helper refuses counter
attribution across changed connection generations; a preparatory comparison
showed only a heartbeat header and does not establish that the user performed
the requested call. Call confirmation and its live interpretation remain open.

## Disposable schema-17 migration proof

A consistent SQLite backup of the installed schema-16 database was migrated in
an explicitly marked private temporary directory, then deleted. The opt-in
`TestRichUpgradeOnDisposableSnapshot` invokes the actual `Store.migrate` path;
it does not invoke runtime initialization or open a Zalo session. All 37 original
data tables retained identical typed-cell multiset digests, all prior migration
markers were preserved, version 17 was added, the new alias table was empty and
`PRAGMA integrity_check` returned `ok`. The original database was opened read-only
for the backup; no installed schema upgrade or service restart was performed.
This proves row preservation through the migration, not equality of all SQL
schema definitions or complete service startup through `OpenWithPolicy`.

The snapshot guard rejects unmarked paths, public permissions, symlinks,
hard-linked database files and oversized/invalid markers. Guard/digest tests
passed; root lint passed after moving this migration-only proof away from the
runtime constructor. The contract reviewer accepted that narrower scope.

The subsequent full root race run failed at several existing startup/readiness
and request deadlines across evalfixture, MCP server and service tests, including
the five-second startup wait in
`TestServicePreservesMCPWhenAuthenticationIsRequired/restore`. No race detector
report was found in that log. A separate lifecycle defect in that test was fixed:
failed startup assertions now cancel and join the service before temporary state
is removed and the next subtest starts; successful shutdown is not joined twice.
If cleanup itself times out, temporary state is retained for diagnosis. Three
focused race repetitions passed after the join fix, and a final focused race run
and root lint passed after adding that preservation guard.
No readiness timeout was increased. The full race run with packages serialized
(`go test -race -p 1 ./...`) subsequently passed, including all previously failing
packages. This supports environment/load sensitivity but does not by itself prove
the cause of the initial parallel-run timeouts; the failed log is retained.

## Confirmed two-call observation

The user subsequently confirmed two incoming voice calls from the existing test
peer: one unanswered and one answered. The listener connection generation still
matched the saved baseline. One new direct record was retained as
`chat.recommended` / `unsupported` / `metadata_only`; shape diagnostics identify
an attachment with title, description and action present, type absent and
childnumber zero. One additional independently verified callback was retained.
No activity observation or decoding error was recorded. The only unknown-header
increase was the heartbeat header; six ignored control records were also counted.

The baseline spans both actions and does not contain exact call start/end times.
One retained message therefore cannot be assigned to either call with confidence,
nor do these counters establish answered/missed transitions. Original native
attachment fields were not retained by the installed diagnostic build. Call
classification remains open; this result is not evidence of two call Events or
of no incoming call support. The privacy-safe aggregate and user confirmation
are stored outside Git. No further call or phone synchronization was requested.


The retained unknown record after the two calls was subsequently verified through
actual MCP: its independently signed callback, journal envelope, stored metadata
and full resource matched, and typed context was readable. The first resource
read timed out; a fresh status read showed connected/authenticated and a read-only
retry succeeded without restarting the service. The single test record was acked
and the same receipt acked again; both test journals became empty/unblocked.
This accepts delivery of an explicitly unknown record, not call classification.

## Maximum v3 recovery envelope

Independent contract review found that 16 maximally escaped content elements could
fit the callback limit but exceed journal recovery's 28 KiB page. New v3 encoding
validates the complete content first, then caps the envelope at 26 KiB by omitting
only trailing elements from a copy with `content_truncated=true`; the complete
metadata remains at the resource URI. Existing signed payloads remain immutable;
old v3 may omit this optional marker and v2 wire behavior stays unchanged.

Regression tests passed for maximum schema-valid fields and array size, maximum
escaped identities/event ID, source immutability, callback/journal semantic equality
and ack. Invalid 17-element input is rejected before projection. Actual MCP
resource reads preserve 16 maximum-length escaped labels while enforcing namespace,
collection, recall and expiry. Focused race, discovery baseline and root lint
passed; the contract reviewer accepted this fix with the schema-refresh caveat.
Full root non-race verification after this contract change passed with packages
serialized; changed-package race and vet passed. The earlier full serial race
result predates this projection change. Both installed skills were rechecked
against all six repository files each after the content-truncation guidance update.

## Exact-record preload diagnostic: installed

A read-only availability probe through the installed service's existing session
returned available preload with five direct and two group records. It persisted
no messages. The aggregate-only owner command could not identify which record
was present, so the candidate now accepts an optional exact typed
conversation/message selector. It returns match/ambiguity and a bounded ephemeral
shape. It does not expose the selector, text, URLs or param values, and does not
start a listener, import history, request phone sync or emit Events.

The initial lexical token filter was rejected by evidence review because a
private string can contain only letters. It was replaced with fixed token/key
allowlists; unknown values expose only presence/counts, with no raw strings or
hashes. Both reviewers accepted the scoped change; diagnostic words are not call
classification. Adversarial ASCII privacy tests and the collector/native shape
race tests passed. An existing service fixture hit its five-second startup wait
in one focused race run; a bounded goroutine snapshot was added to future timeout
failures without changing that limit. Three isolated diagnostic repetitions then
passed in approximately 1.8–2.2 seconds each. This does not establish the original
timeout cause. Full frozen-source root non-race tests, vet, lint and native terminal acceptance passed.
An earlier full run was invalidated by source edits during its build and is not
used as current-code evidence. The candidate was installed directly as the Go service, retaining schema 17.
A fresh stopped-service private backup was hash-verified and its copied database
passed integrity checking. Post-upgrade comparisons preserved the preexisting
corpus, all subscription definitions, retained signed payloads and processing
checkpoints. The working v2 subscription was not changed.

The exact retained record matched once without ambiguity. Its params were an
object with allowlisted keys `callId`, `calltype`, `duration`, `reason`; two
unknown keys were withheld. The action was present but outside the fixed known
vocabulary. This is evidence of call-related structure, not an accepted mapping
of missed/answered calls, status or duration units. No additional call, sync,
listener or history import was requested.

Readiness was observed after upgrade, but intermittent MCP read latency remains
open: status and journal reads sometimes exceed a ten-second client deadline.
The same behavior reproduced through the official Go MCP SDK. A reversible
process background-policy experiment did not resolve it and the original policy
was restored. No database corruption or specific lock leak has been established.
Do not treat a successful readiness sample as stable acceptance.

At that checkpoint, a further owner-only diagnostic extension was staged: `param_shapes` reports only
JSON types and emptiness for the four fixed keys, without native values, lengths,
nested keys or hashes. Missing keys and explicit null differ; number/boolean
emptiness is null. That stage is superseded by the successful live result below.


## Parameter-type diagnostic: intermediate startup failure (superseded)

The optional four-key descriptor is implemented, with closed output schema and
AAA privacy/shape tests (including valid JSON numbers beyond float64 range).
JSON decoding uses Number for type inspection. Focused decoder/collector race
tests, schema tests and scoped lint passed. The extension does not classify calls.

After a verified stopped-state backup, the built diagnostic candidate was
installed once. It did not reach application readiness: a bounded native sample
showed dyld/CoreFoundation initialization, including the user-directory lookup,
before Go execution. The identical binary's help-only invocation completed
quickly outside LaunchAgent. A reversible process-priority check did not restore
readiness and the original policy was restored. The previous verified binary
was restored without restoring or deleting runtime data. At the next observation
the old binary was also running but had not updated its stopped-state heartbeat.
This does not establish a candidate-code cause or resolve the earlier MCP latency.
At that intermediate observation the new descriptors had no successful live
result and native readiness was blocked. Both observations are superseded by
the Standard-policy result below. No additional call or phone approval was used.


### Native readiness restored with Standard policy

The restored binary subsequently progressed into Go JSON Schema validation, but
received only about 0.43 CPU seconds over 120 wall seconds under Background.
With the same binary and only LaunchAgent ProcessType changed to Standard, the
collector connected and the official SDK completed initialize/status/two journal
reads in approximately 0.008/0.091/0.174/0.004 seconds. All other plist fields
were compared unchanged and the original plist retained privately. This supports
a scheduling cause for the observed native stall; the preceding short
process-priority experiment was insufficient to reproduce the policy change.

The type-diagnostic candidate was then installed with that policy. SHA-256:
`b41c96917436a56b17bcf578a694a7959b50b66459037557fe47d45696447618`.
Schema remains 17. Corpus, subscription definitions, retained signed payloads
and processing checkpoints were rechecked as preserved. Official SDK readiness
passed again (status about 1.46 seconds; both test journals empty/unblocked).
The exact retained record matched without ambiguity: all four allowlisted
parameters have JSON number type; values remain private. No new call or history
sync was used. Classification of the two calls and their statuses remains open.

The installation template now explicitly uses Standard and the migration guide
explains the older Background setting. These observations establish current
readiness, not a long-duration performance guarantee.

Current-code full root non-race tests (serialized packages) and root vet passed
after the parameter-type extension. Focused race and scoped lint passed; the
LaunchAgent template passed plutil and git diff whitespace validation passed.
This is not final whole-goal acceptance or publication.


## Narrow direct call classifier: implementation, not yet deployed

The exact owner-only source probe confirmed `recommened.misscall` on the retained
`chat.recommended` record. Both independent reviewers accepted this evidence
for a direct call-summary observation with unknown outcome, not for a
missed/answered mapping. The new classifier requires the exact direct/action/type
combination and numeric shape of all four known params; it publishes none of
their values. Unverified neighboring actions/groups are not classified as calls.

The shared live/replay route emits only Activity; historical/preload conversion
skips known call records silently. Call subscription schemas and storage both
restrict scope to direct or exact-direct. Existing message identities are checked
atomically before a first call write: previously retained projections produce an
expired empty-payload tombstone, not another notification. The migration tests
cover restart/replay, typed collisions, fresh outbox deduplication and preservation
of an old delivered payload and acknowledgement checkpoint.

The signed TLS receiver/actual MCP journal/ordered repeated ack/cancellation
scenario now runs for both reaction and call. Focused race checks passed. The
full frozen-source root/vet/lint/build pipeline is running; no native call
classifier has been deployed or freshly delivered yet. A dedicated exact-peer
call test receiver is being prepared before requesting any further action.
The working Ann v2 rule and subscription are unchanged.
