# Independent final product review B

Product revision: `fab50b013a0c648e4b4884cacf324c5a8c6dfa59`.
Review mode: read-only. Both MCP Server Best Practices and Agent Skills Best Practices were applied. Reviewer B did not consult reviewer A's verdict when reaching the implementation findings.

**FINAL VERDICT: ACCEPTED — 43/43 PASS (100%), 0 FAIL, 0 NOT_RUN, 0 unresolved errors in the reviewed scope.**

Exact-revision CI, installation and runtime evidence have been independently verified. Both independent reviewers completed their entire substantive audits before exchanging completion information; the mutual administrative acceptance rows are now recorded from those completed reports.

## Scope and authoritative baseline

The full R01–R12 scope and fixed 43-criterion denominator are preserved. The user clarified that Ann is an example client: saved-workflow migration is accepted through the documented actual MCP SDK adapter, with persisted configuration and callback execution, rather than requiring changes to a specific inaccessible external host. This clarification is recorded in the task and acceptance documents.

The user's later explicit instruction to cancel all subscriptions overrides the earlier subscription-preservation requirement for that intentional cancellation only. The final deployment must preserve the resulting inactive baseline and must not recreate production or test subscriptions. Corpus, sessions, operation outcomes, configuration and processing checkpoints remain protected.

## Complete criterion matrix

| Criterion | Status | Independent evidence |
| --- | --- | --- |
| R01.1 | PASS | The processing-readiness contract and executable schemas separate retained readiness, true transport state and processing ack; explicit persisted mode preserves the old delivery-terminal default. |
| R01.2 | PASS | Actual HTTP MCP SDK and signed TLS callback tests read and ack before 2xx and between 2xx and delivery commit, without a companion or another message. Sending remains sending until the worker commits receipt. |
| R01.3 | PASS | Early ack survives 503, timeout, database/service/SDK restart and retry with identical callback bytes and IDs. Separate durable companion tests cover a missing or interrupted run without pretending it is the primary race fix. |
| R01.4 | PASS | Ordered, generation-scoped receipts, cancellation, policy/retention/recall checks and unfinished outbox bytes remain protected. Poisoned entries become explicit receipted gaps before budgeting; valid oversized entries stay bounded errors without false ack. |
| R02.1 | PASS | Full coverage remains the default with its original complete representation; compact mode is explicit and reports exact snapshot count, truncation and continuation. |
| R02.2 | PASS | SDK budget tests cover anchor, search and browse with at least 800 gaps. Compact replies remain bounded while preserving usable message reads. |
| R02.3 | PASS | Coverage continuation uses signed typed identity, insertion high-water mark, row digest and 24-hour lifetime. Appends are excluded; mutation, deletion and expiry are explicit, not silently mixed snapshots. |
| R02.4 | PASS | Browse reduction retains its original insertion snapshot and advances only past returned rows. Storage and SDK tests verify no missing or duplicate rows and renewed policy checks. |
| R03.1 | PASS | Tools-only SDK tests reconstruct complete retained Unicode text across scalar-aligned pages, including escaped wire payloads and the duplicated MCP text representation. |
| R03.2 | PASS | Archive capabilities remain distinct from message IDs. Archive paging and old resources work; the real legacy event encoder URI is supported without confusing a colliding direct-chat identity. |
| R03.3 | PASS | Empty and boundary-sized text, invalid targets, tampered or foreign cursors, expiry, process restart and changed content are tested with explicit safe failures. |
| R03.4 | PASS | Each page re-enters the existing authorized owner; policy, retention, recall and archive capability checks remain current. No upstream writes or media fetching are introduced. |
| R04.1 | PASS | Both skills distinguish visible tools, deferred discovery, unsupported host facilities, explicit absence of a connection and permission denial, without inventing a universal discovery API. |
| R04.2 | PASS | The exact typed context tool is named for conversation events. Direct IDs are never passed to group-only compatibility tools; unavailable context remains an explicit limitation. |
| R05.1 | PASS | README, technical specification, contracts, system reference, connection/deployment guides and skill documentation describe the implemented paths and preserve operational limits. |
| R05.2 | PASS | Historical native and model-evaluation reports keep their original boundaries. Updated skills are not declared validated by older model runs. |
| R06.1 | PASS | Schema publication follows transitive local references, cycles and escaped JSON Pointers; unsupported reference scopes retain the original document rather than changing validation semantics. |
| R06.2 | PASS | All 11 pruned output schemas have real SDK request/response fixtures validated before and after pruning. Required-field, nested, enum and format mutations are rejected by both validators. |
| R06.3 | PASS | Manifest, migration snapshots and generated reference match the executable contracts. Publication tests record the removed schema bytes and verify determinism and caller ownership. |
| R07.1 | PASS | Safe build identity and SDK tests pass. Independently observed installed backend revision fab50b013a0c648e4b4884cacf324c5a8c6dfa59 with dirty=false; unknown metadata remains explicit rather than invented. |
| R07.2 | PASS | Independent native STDIO bridge and backend metadata both report the exact clean fab50b release. HTTP and STDIO descriptors match; the legacy unknown-backend case remains covered by tests. |
| R07.3 | PASS | Installed SHA256 7efb9d2d40e458a6c5a9d8e4148968b26abafecf46d11002cbddb345ee3b24f8; embedded revision fab50b, dirty=false, CGO=0, darwin/arm64. Core HTTP/STDIO and pinned Events protocol probes are independently successful. |
| R08.1 | PASS | Delivery diagnostics are opt-in with a closed reason enumeration. Existing gap reasons and legacy response keys remain unchanged. |
| R08.2 | PASS | A keyed stable delivery reference links retries and restart to the journal, while individual attempt correlation IDs remain distinct. No native or raw storage identity is disclosed. |
| R08.3 | PASS | Correlation IDs are created before preflight/dispatch and the same ID appears in client failure details and the corresponding safe local audit log. |
| R08.4 | PASS | Allowlisted telemetry and diagnostic tests exclude content, credentials, callback addresses, raw errors and native IDs. Unique diagnostic IDs are not metric labels. |
| R09.1 | PASS | Empty literal search may fall back to bounded browsing of an already known chat and period, without translation/synonym crawling or an account dump. |
| R09.2 | PASS | Skill entrypoints route conditionally to references while retaining trust boundaries, explicit action authorization, stable request IDs and unknown/partial handling. |
| R09.3 | PASS | Frontmatter, links and dependency names validate. Static walkthroughs cover deferred discovery, typed context, multilingual empty search, bounded browse, recovery, duplicates, permission failure and hostile content. Actual SDK scenarios verify the stateful parts without model evals. |
| R09.4 | PASS | All 17 source/installed skill files independently match after final cutover. Exact-fab saved SDK workflow migration and old-catalogue negative control passed under race: one-field migration, preserved rule/permissions, readback, callback execution and restart. This follows the recorded user clarification, without claiming an Ann task was changed. |
| R10.1 | PASS | Legacy search/context descriptions explicitly identify group-only compatibility and name the preferred typed alternatives. |
| R10.2 | PASS | Typed reads are the preferred route; live group metadata stays a separate source, and existing legacy consumers/resources remain supported. |
| R11.1 | PASS | The reference generator/checker is committed in the repository and needs no private state, network, temporary helper or running service. |
| R11.2 | PASS | Exact-fab CI run 39117306119 independently reports the reference check and five negative controls successful on macOS and Ubuntu. Local checks confirm 27 tools, 5 resources, 4 templates, 6 Events, 54 reactions, 20 CLI commands and 118 links. |
| R11.3 | PASS | Architectural prose was reviewed against ownership, transports, state, permissions and delivery/processing semantics separately from generated catalogue matching. |
| R12.1 | PASS | Exact-fab CI run 39117306119: root test/race/vet, lint, architecture controls and mandatory offline profile all successful on both operating systems. Offline evidence is 50 cases/480 assertions; no new model or live evals were run. |
| R12.2 | PASS | The same exact-revision CI independently confirms nested zcago test/race/vet and architecture tooling test/race/vet successful on both operating systems. |
| R12.3 | PASS | The exact-revision CI confirms CGO-disabled darwin/arm64, darwin/amd64 and linux/amd64 builds plus native service startup cleanup. Installed metadata independently confirms the clean native CGO-disabled build. |
| R12.4 | PASS | Public fab50b commit signature independently verified true/reason valid. CI 39117306119 has the exact head SHA and completed successfully with all required macOS/Ubuntu steps successful. |
| R12.5 | PASS | Independent verification of 17 stopped-backup file hashes and current SQLite integrity; all 12 protected table baseline rows preserved, schema20, 18 saved operations, config/plist hashes unchanged, write grants closed. The authorized inactive baseline is preserved: active business/control subscriptions and pending/sending rows in both queues are all zero. |
| R12.6 | PASS | Independent installed HTTP/native STDIO calls verify exact clean fab50b backend/bridge, 27 manifest-identical descriptors, authenticated/connected status, 119 messages and safe conversation/resource reads. Events discovery returns six profiles; persisted is advertised; subscription listing is empty. |
| R12.7 | PASS | After B froze its own independent findings, B inspected A's completed exact-fab 43-row report: all 41 substantive criteria independently PASS, zero unresolved errors, and no remaining product or deployment action. Both completed audit processes satisfy the mutual final-recording gate. |
| R12.8 | PASS | B completed an independent audit of every one of the fixed 43 criteria, verified exact public/installed fab plus CI and preserved state, and independently confirmed the completed peer review. Final B acceptance is 43/43, 100%, zero unresolved errors. |

## Independent exact-source checks

- Clean checkout and public signed commit: fab50b013a0c648e4b4884cacf324c5a8c6dfa59; GitHub signature verified true, reason valid.
- Diff from c675 contains only documentation, the generic saved-workflow test and the independently reviewed service-test shutdown deadline fix. No production implementation, executable schema or skill input changed.
- Exact-fab saved-workflow race proof: PASS, 3.639 seconds.
- Exact-fab unified conversation service restart/cancellation race test: PASS, 7.839 seconds.
- Reference checker and all five negative controls: PASS.

## Findings and limitations

No unresolved implementation finding remains. Earlier findings concerning legacy text URIs, pruned-schema fixtures, revoked broad-subscription heads, downgrade guards and malformed payloads before budgeting were fixed and independently rechecked.

The service-test correction accounts for its five-second graceful HTTP shutdown budget plus cleanup, still requires completed termination before restarting the account owner, and fails immediately on timeout. It changes no production deadline or success criterion.

The generic saved-workflow proof is deliberately a temporary, documented client adapter. It is not an Ann/Codex automation API, a model evaluation or a claim that every external host already loaded the new schema. Other hosts require their own supported catalogue refresh and saved-workflow migration. This is an operational limitation, not an additional Ann-specific acceptance requirement after the user's clarification.

## Final independently verified installation

The installed product is the signed public fab50b release, SHA256 7efb9d2d40e458a6c5a9d8e4148968b26abafecf46d11002cbddb345ee3b24f8, with clean embedded VCS metadata and CGO disabled. Final exact-head CI is green on both platforms.

The reviewer used only token-in-memory read-only HTTP calls, a temporary native STDIO bridge, safe aggregate SQLite queries and private backup hash comparison. This verified catalogue/identity parity, authenticated connected status, persisted discovery, 119 messages, 18 saved operation outcomes, 17 source/installed skill files, 17 backup files and all 12 protected tables against the post-cancellation baseline. No production ack, subscription, permission or Zalo write was performed by reviewer B. Active business and control subscriptions and pending/sending deliveries are all zero by direct independent queries.

A later public annex may document this product revision using Markdown-only reporting changes; it must not be presented as a different installed executable revision or alter executable contracts, skills or production code without renewed verification.

## Final independent acceptance

Reviewer B accepts the complete agreed R01–R12 scope for product revision fab50b013a0c648e4b4884cacf324c5a8c6dfa59: **43/43 criteria, 100% completeness, 0 unresolved errors**. No denominator was reduced and no substantive criterion was declared inapplicable. The documented generic-adapter acceptance follows the user's explicit clarification; cancellation of all subscriptions follows the user's later explicit instruction and is independently preserved.

The final two review rows record the completion of two already independent full technical/operational audits. Peer completion was inspected only after B's own implementation findings were final, so no peer conclusion was used to derive B's technical judgment. This mutual recording requires no additional product action and does not substitute for missing technical evidence.

There are no required product, CI, installation or verification tasks remaining in this agreed scope. Documented limitations remain: partial upstream history and collection during Mac sleep, optional companion integration for missing-run insurance, external host-specific catalogue/workflow setup, possible notification/ack crash duplicates, and no claim of mathematical absence of every defect or improved model behavior. These limitations are documented rather than hidden acceptance failures.

## Addendum: subsequent annex CI and test synchronization

A subsequent publication of the documentation annex (`703`) exposed an Ubuntu race-test failure in `TestNativeHistoryWorkerKeepsLedgerAndRequestLink`: the test saw the committed completed ledger state and cancelled the worker before its final transition observation. This is a test synchronization race, not a Go data-race report or evidence of lost ledger data. It does not change the frozen fab50b product verdict above.

Reviewer B independently inspected and accepted the test-only correction: before cancellation, wait for both completed ledger state and the end of the actual synchronously exported `zl_mcp.worker` span. Span export occurs after the operation callback and its transition observation. All transition, upstream-call, request-link and privacy assertions remain unchanged, as does the original three-second deadline. No production Go implementation, executable JSON contract or skill input changes.

B independently ran the corrected test under `-race` for 20 repetitions: PASS in 8.340 seconds. The root's 50 corrected repetitions passed in 21.778 seconds with lint reporting zero issues. The root's 50 repetitions of the old test also passed locally in 21.842 seconds; therefore this addendum does not claim a local reproduction of the CI failure. The CI log and source ordering establish the observed cancellation window.

The combined current publication must still obtain a successful CI run for its new exact revision. That later publication result is not assumed or claimed by this addendum before it is available. No new runtime, production subscription, acknowledgement or Zalo action was performed for this review.
