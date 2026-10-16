# Rich content: current acceptance status

Updated: 2026-10-16. Whole-goal acceptance is **pending**. The authoritative scope
is [the task](task-rich-content-events.md); limitations do not turn unperformed
checks into successful acceptance. Historical investigations are retained in the
[implementation worklog](rich-content-implementation-checkpoint.md) and
[deployment worklog](rich-content-candidate-deployment.md).
The [native case plan](rich-content-native-cases.md) records the agreed actions;
completed results and remaining execution gates are below. No further manual
action is currently required.

## Installed candidate and preservation

| Item | Current evidence |
| --- | --- |
| Installed SHA-256 | `3fc627d79f37651933f331bd7374e9e25041b7bb91c3df9475c7ab2fbb1f1af4` |
| Schema | 17; additive content/activity/client-alias migrations |
| Service | Direct native Go binary, one account listener, LaunchAgent Standard |
| Actual MCP | Current project-pinned Go SDK client initialize/status passed; new group journals were read empty through MCP |
| Native startup smoke | Preceding diagnostic candidate passed terminal acceptance on temporary state; current native service start verified through MCP |
| Preservation after diagnostic upgrade | All prior corpus/activity facts retained; all subscription definitions, retained signed payloads and processing checkpoints compared unchanged |
| New test subscription | Separate exact-test-peer incoming call profile, no expiry; signed challenge verified; unsigned callback rejected; journal initially empty |
| Group stand | User selected an available group through the assistant; Test has two exact-group outgoing subscriptions, no expiry, signed challenge and unsigned rejection verified |
| Production consumer | Existing Ann v2 subscription preserved; no client cutover performed |
| Skills | Both validated; all six files each matched their installed copies |

The Standard policy restored readiness in a same-binary comparison after severe
Background scheduling delays. Two successful SDK checks establish current
readiness, not a long-duration latency guarantee. The original plist and stopped,
hash-verified state backups are private and retained outside the repository.

## Requirement matrix

| Requirement | Status | Evidence and remaining work |
| --- | --- | --- |
| Text/Unicode emoji/ZWJ | Native + offline proved | Exact text survived native corpus/read/resource/callback/journal; bounded Unicode projection covered offline |
| Sticker and photo without caption | Native + offline proved | Typed metadata-only messages; media bytes deliberately unavailable |
| Photo caption | Native provenance proved; refinement installed, offline verified | Controlled title exactly matches instructed caption. Restricted incoming direct single-photo author-text rule and full Unicode text-resource route pass offline; original delivered envelopes retain metadata-only semantics |
| Multiple images/album | Controlled native multi-photo batch proved | Two images sent together in Test arrived as two independent image records and survived the complete pipeline. No artificial merge; arbitrary compound-object album forms remain unverified |
| Audio/file/link/location | Native forms and original pipeline proved; refinements installed, offline verified | Five messages include audio, file, location and a recommendation-link. Exact recommened.link and controlled filename enable restricted refinements; sizes/duration/coordinates remain unavailable |
| Video/contact/other unknown forms | Explicit limitation | Honest unsupported projection; no rich native mapping claimed |
| Direct reaction and client-only target | Native + offline proved | Incoming/outgoing, target resolution and exact signed journal/callback; action remains unknown |
| Group/media reaction targets | Direct and outgoing group media native proved | Direct observations resolve to the controlled direct photo; the outgoing Test reaction resolves to one exact group photo, through source/read/resource/journal/signed callback. Incoming group remains offline evidence |
| Call discriminator | Both native forms proved | Exact direct chat.recommended actions misscall (four numeric params) and calltime (three, without reason); no values interpreted |
| Fresh call delivery | One native call activity proved; restart/ack pending | Fresh user calls produced one live incoming Activity plus one calltime message still in its original unsupported projection. Both verified against signed bodies/journal and left unacknowledged for upgrade; calltime refinement is offline verified, not yet installed |
| Other call outcomes/forms | Explicit limitation | Status/mode unknown; duration/native status/call ID null; no answered/ringing/lifecycle guarantee |
| First incoming media / no activity novelty | Offline proved | Temporary-store assertions; a new live account is not required for this storage invariant |
| Journal/signatures/retry/restart/ack | Proved for tested families; native call pending | Exact retained payloads, restart/ordered/repeated ack and independent HMAC; HTTP acceptance is not model processing |
| Resource policy/recall/TTL/control/limits | Offline proved | Actual MCP resource route with denial/expiry/recall and maximally escaped legal payload tests |
| Unknown-content counter | Verified and installed | Atomic per-store unsupported-attempt diagnostic, with exclusion/duplicate/failure semantics; root race/vet/lint passed |
| Migration/cross-ledger replay | Offline + preservation proved | Old call-as-message replay becomes silent tombstone; old callback and processing checkpoint regression; no old body rewrite |
| Downgrade | Conditional offline proof | Actual prior runtime compatible-state check and refusal for incompatible work; never restore an old database and discard new processing facts |
| v2/v3 consumer migration | Prepared | Reconciliation/dedup runbook; actual Ann cutover is not required by this goal |
| Final checks/review/publication | Pending | Freeze final source; root/nested race/vet, native/cross builds, secret scan, both final reviews, commit/push and green CI |
| Stand cleanup | Pending | After acceptance cancel only task-owned subscriptions, stop temporary receivers and remove only their exact callback allowlist entries |

## Current verification boundary

For the preceding diagnostic snapshot, full root race tests with serialized
packages, root vet, full lint (zero issues), native terminal acceptance and
CGO-disabled native/cross builds (macOS arm64/amd64, Linux amd64) passed. The
current tree secret scan found no leaks. Nested-module test/race/vet results
remain applicable because its sources did not change in this update.
Call regressions include signed TLS callback versus actual
MCP journal, repeated ack/cancellation, source replay across reopen, exact typed
legacy-message suppression, and unchanged old delivered bytes/ack checkpoint.
Subsequent source edits are not automatically covered by these results.

The subsequent bounded media-shape diagnostics passed focused race tests and
schema tests. Ordinary content logs now use a closed native-type vocabulary:
unknown names/hostnames expose presence only, while stored content retains its
existing native-type contract. Adversarial production-logger race tests passed;
the diagnostics and unsupported-attempt counter are installed. Both independent
reviewers accepted the diagnostic scope; whole-goal acceptance remains pending.

The diagnostic upgrade used a stopped, all-file hash-verified private backup with
SQLite integrity verification. Configuration and plist bytes were unchanged.
An actual project-pinned Go SDK client initialize/status/journal check succeeded after launch;
all prior corpus/activity rows, subscription definitions, retained signed bodies
and processing checkpoints were preserved. Both temporary receivers are healthy,
and a new private media-batch baseline is ready; no user media action is claimed
from this preparation.
The installed connection subsequently emitted persistence diagnostics containing
`unsupported_content` for both direct and group namespaces; no collector error
was recorded. Actual MCP archive-source discovery also succeeded with one
retained source. Its separate mobile numeric-type projection does not prove the
missing live WebSocket media mappings and does not close their native gates.

At 19:09 UTC on 2026-10-15, the installed service was authenticated and connected
without a reconnect since 18:35:07 UTC. That connection had zero direct live
frames, zero emitted direct messages and zero decoding errors. The dedicated call
subscription had no delivery rows or receiver files. A read-only preload probe
succeeded and skipped one recognized call summary, but it returned no identity or
timestamp for that summary; it cannot establish whether either reported action
is represented. No call was acknowledged, replayed as a new notification or
classified as answered/missed from this observation. At that stage no repeat
call was requested; the later time correlation below resolves the interval.
After the diagnostic restart the call journal remained empty. Counter intervals
from the two connection generations must not be combined.

## Controlled direct media batch

On 2026-10-16 the user completed the six requested actions. They produced five
messages (photo, voice, file, recommendation-link, location) and two separate
reaction observations. All seven original source projections matched actual MCP
context/resources, journal events and independently HMAC-verified callbacks;
retained signed bytes matched exactly. Both reactions targeted the controlled
photo. Ordered acknowledgements and their idempotent repeats succeeded; both
test journals became empty.

The temporary receiver had stopped while the backend remained connected. The
original exact-peer receiver was restored with the same signing key and scope;
the existing retry queue delivered all seven records without resending Zalo
messages or changing subscriptions. This proves recovery of the tested queue,
not permanent availability of an ephemeral test receiver.

The batch established caption-title and filename-title provenance and the exact
`chat.recommended`/`recommened.link` discriminator. It did not establish album,
group, forwarded-photo semantics or numeric metadata units. Caption/link/file
refinements are installed in the hash above. Full affected-package race tests
(zalo, collector, service, events, mcpserver), vet and scoped lint passed; final
constant/formatting cleanup was followed by focused race tests and clean lint.
Their synthetic guard/resource tests are separate evidence, and do
not change the original seven signed bodies or turn those bodies into native
proof of the refined projection. Both reviewers accepted the narrow mapping
scope; whole-goal acceptance remains pending.

The stopped media-refinement backup passed all-file hash and SQLite integrity
verification. After upgrade, all prior corpus/activity facts, subscription
definitions, retained signed bodies and processing checkpoints were unchanged;
the seven controlled payload hashes were checked explicitly. Only the exact
group receiver URL was added to configuration; plist bytes were unchanged.
The two intentional new Test subscriptions were created after this preservation
comparison and accept only outgoing records. Their subsequent group actions are
recorded below.

## Controlled group multi-photo batch

The user then sent two images together from the main account to the selected
existing Test group and reacted to one. The source emitted two independent
`chat.photo` records, each with one image element, and one outgoing reaction.
All three stored projections matched actual MCP context/resources, journal
envelopes and independently HMAC-verified callbacks; retained callback bytes
matched exactly. The reaction's single target was one of those exact visible
group message IDs. Ordered acknowledgements, idempotent repeats and empty final
journals were verified. There was no extra message record for the reaction.

This closes the observed multi-photo and group-media-target cases. It does not
establish incoming group forms or every possible compound/forwarded album
representation. The separate
call gate and final whole-goal review/publication are still pending.

## Call time correlation and fresh stand

The user confirmed that both original calls were from the test peer to the main
account at approximately 00:12 Vietnam on 2026-10-16. The retained unknown
`chat.recommended` message is timestamped 00:12:30.512, matching that interval.
The narrow handler's first connection was only at 01:35:07.825 and its dedicated
test stand was ready at 01:35:57.204. Comparing those original calls against the
later stand's empty interval was an incorrect correlation, not evidence that the
new handler lost them.

The original unknown-message callback and acknowledgement remain immutable.
The subsequently recognized old call summary becomes a silent compatibility
tombstone, preventing a duplicate notification. A single summary does not prove
which of the two actions it represents or either outcome.

After this clarification, the existing exact-peer incoming call receiver was
restored with the same key and subscription. Unsigned requests were rejected;
a fresh signed verification challenge succeeded. MCP confirmed connected status
and an empty, unblocked call journal. The original baseline was retained and a
new connection/sequence/counter baseline saved. One new unanswered voice call was
requested only after this readiness check, without expiry or another service
restart. Native call delivery remains unproved until that action is observed.

The user subsequently made both a fresh unanswered and a fresh answered call.
The same connection generation received two direct frames without decode errors:
one live incoming call Activity and one still-unsupported recommendation message.
The Activity matched its HMAC-verified callback, stored source and actual MCP
journal, retaining unknown status/mode and null call ID/native status/duration.
The second record's bounded log established exact action `recommened.calltime`
and numeric callId/calltype/duration, with no reason field. Its context/resource,
journal and signed body were also verified. Both processing receipts are held
without acknowledgement for the pending restart comparison.

The calltime refinement is now specified and tested autonomously, separately
from that native input evidence. It retains the old misscall fingerprint, uses
the actual action to distinguish observations, and preserves the original
calltime-as-message body through the existing compatibility tombstone policy.
No extra user call is requested solely to relabel that original record.

Only boolean/counter results and SHA-256 body/receipt digests were retained for
this comparison. A proposed full journal/callback proof export was rejected by
automatic approval review and was replaced by in-memory comparison, without
creating that export. The existing runtime/test-receiver records remain under
their original storage policies.

No new model evals, OCR, speech recognition or automatic media downloads have
been run. The test account and receivers are retained until the remaining agreed
checks finish; test actions have no expiry window.

## Final verification and rollback correction

The complete `make check` passed locally on 2026-10-16: root and patched zcago
module tests/race/vet, both lint scopes, architecture/negative controls, three
CGO-disabled platform builds and the existing synthetic offline evaluation.
No model evaluation was run. A subsequent independent audit found that a legacy
suppression marker `{}` could indefinitely obstruct downgrade. The correction
stores no payload for new markers and narrowly canonicalizes the preceding exact
expired call marker without linked deliveries during maintenance. Real retained
payloads still block downgrade; the guard remains read-only. Post-correction
full storage/events race, three platform builds, lint (zero issues) and vet passed.
The final formatting-only test adjustment passed focused race regressions again.
Final deployment and publication remain pending.
