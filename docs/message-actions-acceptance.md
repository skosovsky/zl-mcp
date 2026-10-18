# Explicit message actions: acceptance evidence

Updated: 2026-10-18. Native cases and code checks below are completed. Publication/CI evidence is
recorded on GitHub; clean-install and runtime proofs are kept privately.

Task: [explicit reads and reactions](task-mark-messages-read.md).
[User guide](message-actions.md) · [contracts and upstream research](contracts/message-actions-research.md).

## Native cases

Tests used the existing direct test peer and Test group, with explicit grants,
no expiry and no new accounts. The assistant sent no Zalo text messages and
changed no group membership. Sender-side observations were supplied by the user.

| Case | Evidence | Limits |
| --- | --- | --- |
| Direct incoming text receipt | Native present status 0; sender confirmed first target read | Second became read after opening the chat; watermark isolation is unproven |
| Direct heart, replace with like, remove | Native requests accepted; user confirmed one heart, one like without heart, then none | Initial chain used owner-only probe; public transport is separately covered |
| Durable direct UUID replay | Exact saved results and timestamps; no further dispatch | Historical unknown receipt was never promoted |
| Remove when already absent | Public HTTP accepted; native STDIO same-UUID replay returned exact saved result | No new UI observation for this no-op |
| Group incoming text receipt | Fresh test-1: one public seenv2 returned present status 0; user confirmed main account appeared in sender read list | No delivered-event fallback; earlier ambiguous targets are not claimed read |
| Ordinary group message read | Fresh test-2 read through connected MCP context tool; ledger stayed at 12; sender confirmed main account absent from read list | Live evidence is context reading; other non-write paths have synthetic SDK coverage |
| Group heart, replace, remove | Public clear+add, clear+add, clear accepted; user confirmed heart → like → none | Each set has two non-atomic calls |
| Incoming group photo | Exact incoming chat.photo target; public heart and remove accepted; both confirmed in UI | Caption selected the photo; no inference from nearby text |
| Outgoing group photo | Exact outgoing chat.photo target; public like and remove accepted; both confirmed in UI | Extra participant photo was correctly incoming and was not used as outgoing target |
| Accumulated own reactions | User placed three hearts; public clear+add produced exactly one own like with no hearts | Tested heart accumulation, not every catalogue encoding/count combination |
| Other participant preservation | Different actor's laughing reaction remained after own replacement and removal; user confirmed only foreign reaction remained | Native token was :>; no assumed Unicode glyph mapping |

Present native zero status is now classified as protocol acceptance on direct and
group routes. Missing/null/nonzero remain unknown. This is not a blanket promise
of sender-visible state from an API result, per-message isolation, cross-device
counters or complete upstream semantics. Initial receipts recorded as unknown
remain unknown even after later evidence establishes zero-status classification.

All calls used stable request UUIDs. No unknown operation was silently retried or
resumed. No live partial/timeout was manufactured; failure/interruption semantics
are covered by deterministic owner/storage/transport tests.

## Decoder defect found by the group case

The first group set-heart stopped at clear/unknown; add was not attempted. The
user confirmed no heart. Pinned zca-js accepts msgIds as an array or as a string
containing an array; local Go accepted only a string. Both forms now decode with
integer precision. Null elements, zero/negative IDs, fractions, malformed values
and integer overflow cannot imply acceptance. Missing/null/empty lists remain
non-accepting at the adapter.

After the fix and separate user authorization, a new operation completed both
steps; its native responses were observed as arrays of length one. The original
failed reply was not captured, so its exact shape remains unproven. Old unknown
records were neither rewritten nor automatically continued.

Diagnostics record only a closed shape, decode outcome and bounded count. No
native body, identifier, message text, session value or raw error is logged.
Encrypted synthetic wire tests cover direct string and group array responses.

## Contracts and implementation

The three public tools pass scoped runtime and typed consumer ports to the single
durable action owner. The owner-only diagnostic probe is absent from public MCP
discovery. Set/remove input never exposes raw native plans or protocol fields.
The 22 existing tool descriptors and manifest entries remain unchanged; three
were added. The reaction tool has destructiveHint=true because it clears existing
own reactions. HTTP and native STDIO expose the same 25 tools and catalogue.

- Schema 18 stores genuinely observed receipt metadata with presence/provenance,
  exact retained message identity and FK lifecycle; there is no guessed backfill.
- Schema 19 binds UUID/fingerprint/typed chat/plan, serializes dispatch per chat
  and persists stage/checkpoints before calls. Interrupted dispatch becomes
  unknown, with no retry worker or automatic partial continuation.
- Target visibility, recall, expiry, collection policy and independent read/react
  grants are rechecked before each step. Calls follow service/session lifecycle.
- Status is read-only and available without source/write grants, subject to bound
  account and current collection policy. Local persistence failure cannot invent
  upstream success.
- A set selects clear+add internally. The budget is 70 seconds; HTTP derives +10,
  and the STDIO proxy another +5. Completed steps count confirmed native calls,
  not visibly read messages. Bulk receipt status is aggregate.
- Catalogue retains all 54 pinned encodings, stable codes and display names;
  known heart/like emoji and honest name fallback avoid invented glyph mapping.
- Both source and installed skills were updated and structurally validated. Reads,
  Events and journal acknowledgement never authorize automatic read/react writes.
  Historical model evals are not claimed as evidence for these actions.

## Installation and preservation evidence

Native cases ran on development builds, not the final clean publication build.
The last native-test executable after local ad hoc signing had SHA-256
`08cb8f54b5b213a1053718dc8bef5491509a55941c645b2d167d232e4b93e9cf`.
It predates the final source classification of group status 0 and the final
malformed-response hardening. This hash identifies historical test evidence only.

Before replacement, the stopped service's account lock was held and a complete
private state backup verified by file hashes and SQLite integrity/snapshot checks.
The same registered native Go LaunchAgent was preserved. On the last update,
macOS initially refused launch with OS_REASON_CODESIGNING; refreshing local ad
hoc signing restored it. No Python runtime wrapper or extra collector was added.

Post-restart proofs preserved all 110 messages then present, eight existing action
rows, receipt/identity/client/quote records, source events, tombstones, activities
and subscription definitions/revisions. Config/plist bytes did not change.
Collector reconnected/authenticated, HTTP exposed 25 tools, and the old unknown
results remained identical. Subsequent test actions intentionally added ledger
rows; they are not a preservation mismatch.

Private evidence retains backups, hashes and runtime checks. Credentials, exact
account/message identifiers and raw private payloads are not publication files.
The final release verification must compare the clean published revision and
installed binary metadata, then verify saved results after restart and revoke
only the two temporary action grants. Ann's subscription and corpus are retained.

## Automated evidence

- Root test, full root race, vet and lint passed. Latest stable full race included
  group-zero classification and positive response-ID checks; targeted discovery
  test passed after descriptor wording changes.
- Full nested test/race/vet and tooling test/race/vet passed; targeted response
  regression race passed after decoder hardening.
- Actual SDK HTTP and compiled STDIO bridge tested read, composed set, replay,
  status after source/grant removal, scoped access and error contracts.
- With active grants, ordinary search/browse/resources and real successful journal
  read/ack after synthetic ingestion/fanout created no native action or intent.
- Owner/storage/runtime tests cover exact targets, UUID conflicts, concurrency,
  unavailable metadata, cancellation, storage failures, partial/unknown outcomes
  and interrupted SQLite reopen.
- Architecture passed host/all-build compliance and coverage 100%, closed
  exceptions, production graph, 14 isolated and six source controls.
- CGO-disabled macOS arm64/amd64 and Linux amd64 builds passed. The native arm64
  test installation was checked through real HTTP and STDIO.
- Existing mandatory offline profile passed 50/50 cases, 480/480 assertions.
  Model and live eval profiles were not run; no new model evals were added.

Earlier service race startup failures were traced to repeated schema compilation.
A bounded concurrency-safe immutable embedded-validator cache preserved strict
formats and caller-owned Document maps; subsequent full race passed without
raising startup deadlines. Toolsy keeps its own compiler. One intermediate full
run overlapped a test rename and failed generated testmain compilation; the
subsequent stable-source full run passed, with no detected data race.

## Review and release gates

Two independent agents accepted contracts/code/skills and upstream/native evidence
after the completed matrix.
Findings about stage completion, stale targets, lifecycle, deadlines, telemetry,
required ports, destructive annotation and malformed returned IDs were addressed.
Both final functional verdicts were accepted. They distinguish implementation/native
acceptance from release verification.

Temporary grants were revoked after the cases: both action allowlists are empty,
other config preserved, collector connected/authenticated, and saved accepted/unknown
results remain readable without write grants. Both final functional reviews passed.

Release checks: verify a clean
signed commit and public push with green CI; build/install from the clean published
revision; verify HTTP/STDIO, collector health, retained corpus/subscriptions and
saved action results after restart. Signing metadata and deployment proofs belong
to the private runtime record; GitHub records publication and CI evidence.
