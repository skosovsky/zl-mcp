# Independent release review A — MCP/skills hardening

Reviewed release: `715faf5ec23dd9b35059584d90fba11fd16ea3b6`.
Date: 2026-10-18. Reviewer A did not implement the candidate and did not consult reviewer B's verdict.

**Verdict: NOT ACCEPTED. Completeness: 38/43 = 88.37%. Unresolved implementation errors identified in the reviewed scope: 0. Remaining criteria: 5 NOT_RUN, 0 FAIL.**

This is a release review of the frozen implementation. It is not acceptance of a later architecture change. The release still gates business journal visibility on terminal callback delivery and provides optional companion wakeups. The user's subsequent proposal to separate journal availability from callback delivery is pending architectural resolution; neither the frozen release nor this report establishes that proposed behavior.

## Authoritative release and runtime evidence

- Local HEAD is the reviewed revision and the worktree was clean at review.
- GitHub commit API independently returned the same SHA with verification verified=true, reason=valid.
- GitHub Actions run 39112765094 independently returned completed/success for that exact head SHA. Both macOS and Ubuntu jobs and all their checks succeeded: reference/negative controls, lint, architecture, existing offline profile, tooling test/race/vet, root test/race/vet, nested zcago test/race/vet, CGO0 cross-builds and native startup cleanup.
- Installed executable SHA-256 independently measured: `db2c6f3f4d1289747eaefd044bda6a096b204d2dcaf93c940e818d3798fa817f`. Embedded revision matches release, dirty=false, CGO_ENABLED=0, darwin/arm64.
- Actual HTTP MCP 2026-07-28 read-only calls independently succeeded: authenticated=true, collector_state=connected, last_error=null, 27 tools, six Events including journal.ready, backend identity matches the release.
- Actual native STDIO bridge read-only calls independently succeeded: 27 tools, connected/authenticated status; backend and bridge identities both match release with dirty=false. Bridge clean exit 0.
- Private rollback backup: independently recomputed every state-manifest file hash, previous-binary hash and SQLite integrity. Current config and LaunchAgent hashes still match backup proof. Current SQLite integrity is OK, schema20; all old rows in 12 protected tables remain present by multiset hash comparison. No actual message, account, subscription, token or credential value was printed. There are zero configured recovery companions: actual-host activation was not performed.
- Source/installed skills match by complete file set and contents; research eight files and Events nine files. Static walkthrough evidence is recorded in the repository and was checked against the source routes.

The initial temporary runtime helper failed because it requested a nonexistent /readyz endpoint and then omitted the HTTP2026 method/name headers and metadata. Independent checks used the real documented protocol profile; these helper failures are not classified as backend defects.

## Independent execution during this review

- `python3 scripts/reference/reference.py --check`: PASS, 27 tools / five resources / four templates / six Events / 54 reactions / 20 CLI, 117 local links.
- Five reference negative controls: PASS.
- `git diff --check`: PASS.
- Independent tests on frozen release for docs/contracts, app/contentread, storage, events, mcpserver and service, selecting publication equivalence, recovery, actual-SDK recovery, tools-only, coverage, error sanitization, browse snapshot/budget and build identity cases: PASS.
- Earlier independent full selected-package tests and recovery race tests also passed. Control diagnostics race tests independently passed after the log-correlation correction.
- No new model evals, live Zalo writes, callback requests to production receivers, subscription changes or journal acks were performed by this reviewer. The native STDIO process was an ephemeral read-only bridge to the running owner.

## Complete fixed-denominator criterion matrix

PASS requires all components of the row; synthetic adapter behavior is not evidence that the real host is configured and resumes.

| Criterion | Status | Evidence / boundary |
| --- | --- | --- |
| R01.1 | PASS | recovery-ready contract, executable subscription/payload schemas, opt-in metadata and unsupported-mode tests preserve default shapes |
| R01.2 | NOT_RUN | Actual-SDK synthetic adapter proves both race windows and exact last-event recovery, but production has no companion and real target-host continuation is not activated/verified |
| R01.3 | NOT_RUN | Synthetic long-block, restart and reminder persistence tests pass; actual target-host durable continuation remains unsupported/unverified |
| R01.4 | PASS | storage/events/SDK tests cover stable retries, bounded reminders, one-hour cap, exhaustion, cancellation, generation changes, ordered ack and no recursive business fanout |
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
| R07.3 | PASS | HTTP2026 protocol separate from release version; installed native metadata, HTTP and STDIO identity independently verified |
| R08.1 | PASS | include_diagnostics opt-in closed reasons and retained gap_reason semantics; legacy default shape tests |
| R08.2 | PASS | Domain-separated stable business/control delivery references, fresh attempt IDs and shared recovery episode reference; retry/restart/reminder and two-sink log/journal tests |
| R08.3 | PASS | Final-audit preflight/execution failures verify same client/log correlation ID, not mutation of shared domain error |
| R08.4 | PASS | Diagnostic allowlists and private-marker tests; no new payload/credentials/native IDs/addresses or unique metric labels; control episode reference remains local diagnostic metadata |
| R09.1 | PASS | Exact-chat/period browse fallback after bounded two literal queries, three pages of20 per pass, preserved continuation and partial-result reporting |
| R09.2 | PASS | Short entrypoints route into explicit references; trust, authorization, idempotency, unknown/partial and no-auto-action boundaries retained |
| R09.3 | PASS | Static10-scenario walkthrough checked against current references/contracts, including eight mandatory scenarios; frontmatter/links/dependencies valid; not represented as model eval |
| R09.4 | NOT_RUN | All17 source/installed files match; saved real-host task update and operational companion binding are not performed through a supported host mechanism |
| R10.1 | PASS | Legacy search/context descriptions explicitly group-only compatibility and typed alternative routing |
| R10.2 | PASS | Live group metadata remains separate; typed reads preferred; old consumers, tools and resources remain registered and tested |
| R11.1 | PASS | Repo-owned deterministic standard-library reference generator/checker, no temporary/private-runtime dependency |
| R11.2 | PASS | CI independently green on both OS for exact catalogue/description/fields/resources/Events/reactions/CLI/local links and five negative controls |
| R11.3 | PASS | Architecture prose manually compared to actual one-owner service, bridge, collector, data/queue and independent recovery flows; no generator-only correctness claim |
| R12.1 | PASS | Exact-head green CI on both OS includes root test/race/vet/lint/architecture and existing offline checks |
| R12.2 | PASS | Exact-head green CI on both OS includes nested zcago and architecture-tooling test/race/vet |
| R12.3 | PASS | Exact-head CI CGO0 macOS arm64/amd64 and Linux amd64 plus native startup smoke; installed native cleanCGO0 build checked independently |
| R12.4 | PASS | Public exact commit, verified valid signature, completed successful CI39112765094 on bothOS independently queried |
| R12.5 | PASS | Independently verified complete private backup/hashes/integrity, prior executable, config/agent preservation, protected-row preservation and schema20-aware rollback tests |
| R12.6 | PASS | Actual installed backend revision/clean metadata, authenticated connected safe status, HTTP27tools/sixEvents and nativeSTDIO27tools/backend+bridge verified independently |
| R12.7 | NOT_RUN | ReviewerA cannot certify100% while operational criteria remain unfulfilled |
| R12.8 | NOT_RUN | Second independent complete100% verdict is not established; no consultation of reviewerB's verdict |

## Verdict and limits

All previously identified implementation defects in reviewer A's scope are closed: infrastructure error mapping, broad-scope revoked-head starvation and control delivery correlation. No further actionable code/security/compatibility defect was found in this release audit. This is zero identified unresolved errors in the tested scope, not a mathematical absence-of-bugs guarantee.

The current release uses callback-terminal visibility plus a companion; it does not implement the newly proposed callback-independent journal readiness. The real host's automation and wakeup support remain unverified and unchanged. No denominator reduction, N/A substitution, synthetic-to-production inference or overall100% claim is made.
