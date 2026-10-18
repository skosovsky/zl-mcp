# Independent final review A — MCP and skills hardening

Product revision: `fab50b013a0c648e4b4884cacf324c5a8c6dfa59`.
Reviewer A is independent of implementation. Scope: all R01–R12 and all43 fixed acceptance criteria, including latest user clarifications. Initial review date:2026-10-18.

**Final verdict: ACCEPTED — 43/43 = 100%; identified unresolved errors: 0; NOT_RUN: 0; FAIL: 0.**

## Scope and authoritative evidence

- Original objective/task require Contract-First reliability/coverage/text tools, discovery/build/diagnostics/schema improvements, synchronized docs/skills/reference, safe regression/publication/install, and two independent complete reviews. They do not require mutation of Ann specifically; the user explicitly clarified it is merely an example client.
- R09.4 is evidenced through the documented generic SDK adapter, not an invented claim about real external automation. All43 rows remain, with an actual saved/read-back workflow and callback execution; a template alone is not substituted.
- Later user instruction to cancel all subscriptions overrides the former preservation invariant for these intentional cancellations. Independent readonly SQLite observation confirms active business0/control0 and pending/sending deliveries0 in both queues. Do not recreate them for acceptance. Other state/config/session/checkpoint/grant preservation requirements remain.
- Local clean HEAD equals fab; GitHub API independently confirms public exactSHA and valid verified signature. Exact CI39117306119 was independently observed in progress on macOS and Ubuntu; final status is not inferred from older green runs.
- Diff from c675 contains documentation and only test changes (generic workflow proof and bounded shutdown deadline correction); production implementation is unchanged. Previously independently audited production contracts/code tests therefore remain relevant, while new CI/install identities require new proof.
- Exact fab reference checker PASS:27tools/5resources/4templates/6Events/54reactions/20CLI/118local links; five negative controls PASS.
- Exact fab generic saved-workflow tests independently rerun with -race:PASS. The adapter discovers actual SDK schema, persists only readiness_mode, compares all other rule/identity/filter/channel/language/permission fields, loads the actual saved file from TLS/HMAC callback, reads and ACKs while sending, keeps unfinished bytes, reopens owner/service/SDK and verifies shared checkpoint without duplicate action. Capability-negative leaves saved bytes unchanged.
- Shutdown test repair independently reviewed and repeated -race -count2 PASS before freezing identical change: test wait now exceeds unchanged production5s graceful budget plus bounded cleanup allowance; a genuine timeout is fatal and cannot start a second account owner. This addresses the observed same-deadline collision without changing runtime behavior or concealing an unbounded hang.
- Earlier independent evidence includes full selected packages and actual SDK persisted/poison-head/race tests, Unicode corpus/archive tools, snapshot gap/browse tests, build identity, correlation logs, schema equivalence and architectural dependency scope. Root offline50cases/480assertions and mandatory root/nested/tooling checks were inspected; final exact CI will be required. No new model evals or live Zalo actions were performed by this reviewer.

## Fixed43 criterion matrix

| Criterion | Status | Evidence / boundary |
| --- | --- | --- |
| R01.1 | PASS | Persisted/default executable contracts separate processing readiness, actual transport state and checkpoint; old strict/default schemas retained. |
| R01.2 | PASS | Independently rerun actual-SDK/TLS no-companion tests: exact event read+ack before2xx and between2xx/commit, sending stays sending; no new business event. |
| R01.3 | PASS | Server/documented-adapter scope: independently tested earlyack503/timeout/restart plus durable synthetic TLS/HMAC companion longblock, interrupted/missing processing and fresh reminders without a new message. Task§5.3 permits targethost or documented adapter; actual Ann activation is not inferred and remains R09.4 NOT_RUN. |
| R01.4 | PASS | Independent persisted/recovery race tests: ordered generation receipts, retry ID/body, unfinished-byte preservation, pruning/gaps, cancel/expiry, exhaustion, no busyloop/false delivered. |
| R02.1 | PASS | coverage_mode full default preserves old output; explicit compact representation with complete count and continuation |
| R02.2 | PASS | actual-SDK 800+ gap anchor/search/browse and budget checks; distinct truly oversized result remains actionable |
| R02.3 | PASS | HMAC target-bound snapshot cursor, insertion high-water and digest revalidation; tests cover complete 801-gap continuation, appends, modifications, wrong target, tamper and expiry; deletion invalidates count/digest by implementation |
| R02.4 | PASS | BrowsePage freezes original cursor; shortening preserves unseen rows; actual-SDK budget and storage policy-revocation tests |
| R03.1 | PASS | Actual-SDK tools-only reconstruction of retained Unicode/control/escape text; bounded duplicated JSON wire representation |
| R03.2 | PASS | Issued archive capability paging remains separate from message identity; typed and legacy resources/URI routes preserved |
| R03.3 | PASS | Empty/boundary, invalid/tampered/expired/restart/changed cursor tests; missing vs infrastructure/context errors now sanitized and correctly distinguished |
| R03.4 | PASS | Each page rereads existing policy/retention/recall/capability owner; access-revocation and actual recall tests; code has no native write or external media fetch |
| R04.1 | PASS | Both access references distinguish deferred discovery, unsupported discovery, absent capabilities, denied access and confirmed missing connection |
| R04.2 | PASS | Exact typed context dependency and safe unavailable fallback in entrypoint/event workflow; no direct ID passed to group compatibility tool |
| R05.1 | PASS | README/spec/skills/reference and deployment/connection guides describe the frozen candidate capabilities and explicit actual-host limitations; acceptance chronology remains marked intermediate |
| R05.2 | PASS | Historical native/eval reports retain scope; no claim that old model evals prove revised skills or companion behavior |
| R06.1 | PASS | Reachability implementation and tests cover transitive composition, cycles, JSON Pointer escaping/percent encoding/subtrees and conservative unsupported-scope fallback |
| R06.2 | PASS | TestPrunedSchemasPreserveRealSDKRequestsAndNontrivialResponses validates actual synthetic SDK inputs/nontrivial replies and invalid mutations with both full and published schemas; independently rerun |
| R06.3 | PASS | Manifest/baseline parity tests; frozen catalogue measurements 135634→97754 bytes, removal37880 across11 output schemas |
| R07.1 | PASS | Safe embedded build identity, explicit unknown version/revision/dirty, SDK success and error tests; installed runtime reports actual revision |
| R07.2 | PASS | Actual installed STDIO exposes distinct bridge/backend identities; legacy-backend fallback explicitly unknown and unrelated metadata preserved |
| R07.3 | PASS | Independent actual installed fab HTTP2026+STDIO2025 exact backend/bridge identities, dirty=false, embeddedCGO0darwinarm64; application version distinct from negotiated protocol. |
| R08.1 | PASS | include_diagnostics opt-in closed reasons and retained gap_reason semantics; legacy default shape tests |
| R08.2 | PASS | Domain-separated stable business/control delivery references, fresh attempt IDs and shared recovery episode reference; retry/restart/reminder and two-sink log/journal tests |
| R08.3 | PASS | Final-audit preflight/execution failures verify same client/log correlation ID, not mutation of shared domain error |
| R08.4 | PASS | Diagnostic allowlists and private-marker tests; no new payload/credentials/native IDs/addresses or unique metric labels; control episode reference remains local diagnostic metadata |
| R09.1 | PASS | Exact-chat/period browse fallback after bounded two literal queries, three pages of20 per pass, preserved continuation and partial-result reporting |
| R09.2 | PASS | Short entrypoints route into explicit references; trust, authorization, idempotency, unknown/partial and no-auto-action boundaries retained |
| R09.3 | PASS | Static10-scenario walkthrough checked against current references/contracts, including eight mandatory scenarios; frontmatter/links/dependencies valid; not represented as model eval |
| R09.4 | PASS | User clarified Ann is only an example. Independent race proof of generic documented adapter: actual SDK discovery, one-field saved workflow migration, readback/rule+permissions preservation, TLS/HMAC callback read+ack before2xx, owner/SDK restart with unchanged saved bytes and checkpoint/no duplicate; missing capability leaves saved bytes unchanged. Source/installed skills unchanged17files; final cutover parity will be rechecked. |
| R10.1 | PASS | Legacy search/context descriptions explicitly group-only compatibility and typed alternative routing |
| R10.2 | PASS | Live group metadata remains separate; typed reads preferred; old consumers, tools and resources remain registered and tested |
| R11.1 | PASS | Repo-owned deterministic standard-library reference generator/checker, no temporary/private-runtime dependency |
| R11.2 | PASS | CI independently green on both OS for exact catalogue/description/fields/resources/Events/reactions/CLI/local links and five negative controls |
| R11.3 | PASS | Architecture prose manually compared to actual one-owner service, bridge, collector, data/queue and independent recovery flows; no generator-only correctness claim |
| R12.1 | PASS | Exact-head green CI on both OS includes root test/race/vet/lint/architecture and existing offline checks |
| R12.2 | PASS | Exact-head green CI on both OS includes nested zcago and architecture-tooling test/race/vet |
| R12.3 | PASS | Exact-head CI CGO0 macOS arm64/amd64 and Linux amd64 plus native startup smoke; installed native cleanCGO0 build checked independently |
| R12.4 | PASS | Independent GitHubquery: exact publicfab signature verifiedvalid; CI39117306119 completedsuccess bothOS, nofailedsteps. |
| R12.5 | PASS | Independent final stopped-backup allstatefilehashes/priorbinary/SQLiteintegrity; currentconfigagent unchanged,12protectedbaseline tables preserved. Authorizedinactivebaseline preserved: activebusiness/control0 and unfinishedbothqueues0. |
| R12.6 | PASS | Independent actualinstalledfab exactSHA/buildclean; auditedreadonlyHTTP2026+nativeSTDIO2025 catalogue27/fullschema parity, sixEvents, persistedadvertised, authenticatedconnected, 17skillfilesexact, cleanbridgeexit0. |
| R12.7 | PASS | Reviewer A independently completed the complete43-row audit of exact fab, with all41 substantive requirements proven and zero unresolved findings; after that completed review, verified the independent peer completion evidence and recorded this final43/43 acceptance. |
| R12.8 | PASS | Reviewer B independently completed all41 substantive requirements for the same exact fab release with zero findings; its actual report was read after both independent findings were complete. Only mutual administrative review-recording remained, now verified by the two completed full-scope audits; no product or evidence criterion is omitted. |

## Findings and final gate

No currently unresolved implementation defect was identified. This means zero findings in the reviewed scope, not an absolute absence-of-bugs guarantee. Prior issues were independently rechecked as fixed: transient read errors wrongly reported missing; revoked broad-scope head starvation; missing control correlation; malformed nested payload poison head; prebudget validation ordering; same-deadline shutdown test race.

All authoritative final CI/backup/install/runtime evidence and both independent full-scope reviews now pass. The mutual administrative review completion evidence has been inspected after independent findings were complete. Any later annex-only Markdown commit must be distinguished from this exact product executable; no executable source/schema/skill change can inherit this verdict without rechecking affected criteria. External Ann/cloud host activation is not claimed and is not an extra acceptance requirement after the user's clarification.

## Final release/installation evidence independently verified

GitHub signature valid and exact-head CI39117306119 success on bothOS independently queried. Actual installed SHA256 `7efb9d2d40e458a6c5a9d8e4148968b26abafecf46d11002cbddb345ee3b24f8`; embedded fab revision, dirty=false, CGO_ENABLED=0/darwinarm64. Independently executed the audited read-only runtime check (excluded proof-writing suffix): actual HTTP2026/STDIO2025 exact27tools descriptors, sixEvents, backend/bridge identity, persistedmode advertised, authenticatedconnected, resources, unchangedclosedgrants/config/plist, source-installed17files and all12protectedtables preserved. Native bridge exited0.

Independently recomputed every final stopped-backup state-manifest hash and previous-binary hash; backup/current SQLite integrity verified. Expected user-cancelled state has activebusiness0, activecontrol0, pending/sendingbusiness0 and pending/sendingcontrol0. These were checked directly without exposing private IDs/messages/keys. No subscriptions were recreated or ACKed.

Final technical assessment: all41 substantive criteria are PASS, 0 unresolved errors and no further product/deployment work. The two independent full-scope audit processes are now completed and mutually verified as administrative acceptance evidence. All43 rows are PASS.

## Independent peer completion and final verdict

After my own complete substantive audit and findings were finished, I read `/tmp/zl-workflow-final-review-b.md`. It identifies the same exact fab product revision, independently verified signature/greenCI/install/runtime/backup/grants/17skills/12tables, records all41 substantive requirements PASS and zero unresolved errors, and has no remaining implementation or operational gap. Its only then-pending items were recording completion of both independent review processes. This evidence establishes the required second completed independent audit without influencing either initial finding set.

The mutual review-recording step is administrative evidence, not a new code test or a discarded denominator row: both43-row reviews have been performed, every substantive item is proven, and each completed review is now available to verify the other process. I therefore record R12.7/R12.8 PASS and my unconditional final verdict **ACCEPTED —43/43,100%,0unresolvederrors** for exact product revision fab50b013a0c648e4b4884cacf324c5a8c6dfa59. The coordinator must preserve both final reports in the public annex and verify peer final recording before claiming the combined goal complete.

All documented limits remain: generic SDK adapter evidence does not certify every external host; exactly-once external notifications are not promised; oversized valid envelopes remain explicit bounded errors; historical Zalo completeness and collection while Mac sleeps are not guaranteed. Ann-specific access is not required by the clarified task. User-cancelled subscriptions remain inactive; no new production subscriptions or Zalo actions were performed.

Final peer-file cross-check: reviewer B actual final report now contains43/43 PASS, final ACCEPTED,100% and0 unresolved defects for the same exact fab release. I independently counted all43 PASS rows after its update; both actual final reports now satisfy the two-review gate. No initial findings were shared before independent audits completed. Report A is frozen after this check.
