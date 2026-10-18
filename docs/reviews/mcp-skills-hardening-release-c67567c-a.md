# Independent review A — persisted journal causal correction

Exact signed published revision: `c67567c54408899bc729cf4afc488ba9eaa5217c`.
Frozen-source, CI and installed-release review, 2026-10-18. Real-host saved workflow remains unavailable.

**NOT ACCEPTED. PASS 40/43 = 93.02%. Unresolved identified implementation errors:0. NOT_RUN:3. FAIL:0.**

This revision now fixes the cause: persisted journal availability is independent of pending/sending callback transport. The old delivery_terminal view is preserved as default; no companion is required for either callback/read race. Companion remains optional insurance for missing/interrupted runs; actual host activation/saved task remains unverified.

## Independent evidence

- HEAD equals c675 and worktree clean. GitHub commit API verified=true/reason=valid.
- Actual GitHub run39115464885 was independently queried: exact c675, completed/success, both macOS and Ubuntu jobs successful, no failed steps. Signature verified=true/reason=valid.
- Independent exact-source targeted race tests passed in storage, mcpserver and adapters/store. They include persisted earlyack, no-companion bothwindows,503/timeout/restart, poison nesteddata/100k/domain-invalidActivity, validoversizedv3 RESPONSE_TOO_LARGE withoutgap/ack, projectedbeforebudget and legacy/default invariants.
- Reference checker PASS:27tools/5resources/4templates/6Events/54reactions/20CLI/118links; five negative controls PASS; diffcheck PASS.
- Root full normal/race/vet, lint0issues, architecture100%/14isolated+6sourcecontrols, offline50cases/480assertions, CGO0darwinarm64/amd64+linuxamd64 and native smoke evidence inspected. New model/live evals were NOT_RUN.
- Prior715 full-code review evidence remains applicable to unchanged implementations R02–R11; all source deltas in c675 were reviewed. New c675 installation is independently verified below; no old installed identity is reused as proof.
- Causal code review: owner privacy/generation checks precede projection; serializedeventID/profile match stored identities. Pure adapter cached fullschema plus typedDTO validation runs before28KiB budget. Invalidpayload becomes bounded receiptedgap; malformed projection metadata/infrastructure errors do not. Valid oversized entry remains an error. Early processing ACK does not clear leased callback bytes or alter transport; retries retain exactID/body.
- Narrow architecture delta permits adapter-to-contract boundary and stdmaps only; no storage-to-contract import or general exemption.
- Updated source skills select persisted only when actualschema advertises it; no silent extra subscription, automaticread/react, guessedpayload, changeduserpermission or SQL/credential bypass.

## Fixed43-criterion matrix

Unchanged criteria use frozen715 code/evidence reviewed previously plus c675 diff audit; release/installed/host rows are independently reevaluated below. No denominator change or N/A substitution.

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
| R07.3 | PASS | Independent actual installed HTTP2026 and STDIO2025 safe reads: backend/bridge c675 dirty=false, CGO0 nativearm64 metadata; protocol independent of application version. |
| R08.1 | PASS | include_diagnostics opt-in closed reasons and retained gap_reason semantics; legacy default shape tests |
| R08.2 | PASS | Domain-separated stable business/control delivery references, fresh attempt IDs and shared recovery episode reference; retry/restart/reminder and two-sink log/journal tests |
| R08.3 | PASS | Final-audit preflight/execution failures verify same client/log correlation ID, not mutation of shared domain error |
| R08.4 | PASS | Diagnostic allowlists and private-marker tests; no new payload/credentials/native IDs/addresses or unique metric labels; control episode reference remains local diagnostic metadata |
| R09.1 | PASS | Exact-chat/period browse fallback after bounded two literal queries, three pages of20 per pass, preserved continuation and partial-result reporting |
| R09.2 | PASS | Short entrypoints route into explicit references; trust, authorization, idempotency, unknown/partial and no-auto-action boundaries retained |
| R09.3 | PASS | Static10-scenario walkthrough checked against current references/contracts, including eight mandatory scenarios; frontmatter/links/dependencies valid; not represented as model eval |
| R09.4 | NOT_RUN | All17 new source/installed files match complete pathsets/bytes; supported saved real-host task update is unavailable/not performed. Installed files alone do not update automation. |
| R10.1 | PASS | Legacy search/context descriptions explicitly group-only compatibility and typed alternative routing |
| R10.2 | PASS | Live group metadata remains separate; typed reads preferred; old consumers, tools and resources remain registered and tested |
| R11.1 | PASS | Repo-owned deterministic standard-library reference generator/checker, no temporary/private-runtime dependency |
| R11.2 | PASS | CI independently green on both OS for exact catalogue/description/fields/resources/Events/reactions/CLI/local links and five negative controls |
| R11.3 | PASS | Architecture prose manually compared to actual one-owner service, bridge, collector, data/queue and independent recovery flows; no generator-only correctness claim |
| R12.1 | PASS | Exact-head green CI on both OS includes root test/race/vet/lint/architecture and existing offline checks |
| R12.2 | PASS | Exact-head green CI on both OS includes nested zcago and architecture-tooling test/race/vet |
| R12.3 | PASS | Exact-head CI CGO0 macOS arm64/amd64 and Linux amd64 plus native startup smoke; installed native cleanCGO0 build checked independently |
| R12.4 | PASS | Independent GitHubAPI signature valid; exact CI39115464885 completedsuccess on macOS and Ubuntu, allchecks green. |
| R12.5 | PASS | Independent complete new stopped-backup manifest/priorbinary/integrity verification; currentconfig/LaunchAgent unchanged,12protectedtables preserved, schema20 rollback tests pass. |
| R12.6 | PASS | Independent actual new installed c675 HTTP2026 and nativeSTDIO2025 exact27descriptors/identity, sixEvents, persistedparameter advertised, connected/authenticated, cleanbridgeexit0. |
| R12.7 | NOT_RUN | ReviewerA fullscope100% gate cannot pass because R09.4 remains operationally incomplete; this report is40/43, not100%. |
| R12.8 | NOT_RUN | Second independent fullscope100% gate cannot pass while savedhosttask requirement is incomplete; reviewerB initial verdict was not consulted. |

## Findings and limits

All identified code defects are closed, including the latest malformed nestedpayload poison-head and prebudget ordering issue. Zero identified unresolved errors means zero findings in the reviewed scope, not proof of absence of all defects. No production callbacks, subscription changes, journal ACKs or Zalo writes were performed by reviewerA.

CI/install/backup/runtime proofs now pass for this exact release. The supported real-host saved-task update remains operationally incomplete; synthetic server/adapter tests do not prove that migration. The goal must not be marked complete while R09.4 remains open.

Interpretation clarification: R01.3 is technical server/documented-adapter evidence, as expressly permitted by task section5.3. The independently verified TLS/HMAC synthetic channel is a verified adapter channel; it is not a verified real-host channel. Consequently R01.3 PASS does not close R09.4, which explicitly requires the supported saved-host task update. Earlier R01.3 NOT_RUN combined these scopes; this revision separates them without changing the43-criterion denominator or dropping actual-host work.

## Independent final installation verification

- Actual installed SHA-256: `b9111ff797af83629e029cb26f09db4ddbb612066de3a574b499fb869c7e1ce7`; embedded c675 revision, dirty=false, CGO0darwinarm64.
- Independently executed the audited read-only portion of the runtime checker: HTTP2026 and STDIO2025 exact27descriptors/actualbackend+bridgeidentity, sixEvents, persistedreadiness advertised, authenticated/connected, resources readable, source/installed17files exact, grantsclosed and bridgeexit0. No callback/subscription/ACK/write operation was performed.
- Independently recomputed all new private backup state-manifest hashes, previousbinary hash and SQLiteintegrity. Current config/plist match backup; oldrows of all12protectedtables preserved; schema20; zero recoverycompanions. Private file paths/values and identities are not published by this report.
- Independent GitHubquery confirms exacthead c675 CI39115464885 completedsuccess on bothOS with nofailedsteps.

Final count for this review: PASS40, NOT_RUN3, FAIL0, denominator43 unchanged. The three open criteria are R09.4 (supported actualhost saved-task update) and the two dependent fullacceptance gates R12.7/R12.8. No newly introduced implementation error remains identified; NOTACCEPTED is due to incomplete authorized integration, not a fabricated backend failure.
