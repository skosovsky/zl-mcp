# Independent release review B

Revision: `715faf5ec23dd9b35059584d90fba11fd16ea3b6` (public signed clean checkout).
Installed SHA256: `db2c6f3f4d1289747eaefd044bda6a096b204d2dcaf93c940e818d3798fa817f`.
Verdict: **NOT ACCEPTED — 39/43 = 90.70%; FAIL0; NOT_RUN4.**
Unresolved concrete code/documentation defects discovered by B: **0**. This does not mean all requirements completed.

Both best-practice skills were applied. Source/contract/skills/doc review and targeted normal/race tests were independent; reviewer A verdict was not consulted. No production subscriptions, acknowledgements, permissions or native Zalo state changed by reviewer. Read-only authenticated MCP and temporary native STDIO bridge were used; secrets/messages/real subscription IDs omitted.

## All43 criteria

| ID | Status | Independent evidence |
|---|---|---|
| R01.1 | PASS | docs/contracts/subscription-recovery-ready.md; strict opt-in subscribe/list/read schemas; legacy shape preserved. |
| R01.2 | NOT_RUN | Both windows pass actual-SDK HTTP/TLS synthetic adapter, independently normal/race; real target-host activation and last-event continuation unproven. New architectural direction pending. |
| R01.3 | PASS | TestSyntheticHostRecoveryActualSDKClosesBothRacesAfterRestart: virtual45s >30s retry budget; persisted owner/SDK/worker restart; no second business insertion. |
| R01.4 | PASS | Recovery storage/worker/SDK tests: immutable retry IDs, fresh bounded reminders, cancellation/generation/ack suppression, gaps and exhausted states; independent race PASS. |
| R02.1 | PASS | coverage_mode full default; compact opt-in discriminator; existing full keys; TestCoverageGapsSnapshotAndCurrentPolicy. |
| R02.2 | PASS | SDK budget tests for800+ gaps and bounded compact anchor/search/browse; complete CI success exact revision. |
| R02.3 | PASS | coverage_gaps.go authenticated snapshot high-water/digest; append excluded, update/delete expires,24h expiry; coverage tests. |
| R02.4 | PASS | BrowsePage freezes insertion snapshot; reduction rereads current policy; storage and SDK browse budget tests. |
| R03.1 | PASS | actual SDK Unicode reconstruction and duplicated wire byte-budget tests; corpus owner rechecks; independent package test PASS. |
| R03.2 | PASS | Archive capability separate from typed message IDs; issued archive URI full paging and resource compatibility; actual legacy group encoder URI regression PASS. |
| R03.3 | PASS | contentread tests: empty/exact scalar limit, tamper, foreign identity/URI, expiry/restart; safe explicit errors. |
| R03.4 | PASS | Existing owner used per page; policy/identity/recall/retention read path; SDK recall test and no native/media commands. |
| R04.1 | PASS | Both service-access references distinguish visible/deferred/unsupported host/explicit missing connection/access denial without invented discovery API. |
| R04.2 | PASS | Events entrypoint and workflow name exact typed context; direct ID never fed to group compatibility; unavailable context stays unknown. |
| R05.1 | PASS | README/spec/skills/reference/client/deployment/recovery/contracts inspected; current capabilities and operational host NOT_RUN distinction explicit. |
| R05.2 | PASS | Historical native/eval reports kept scoped; revised skills not declared validated by historical model evals. |
| R06.1 | PASS | Reachability graph follows transitive local pointers/cycles/escaping; unsupported scopes fail closed; publication graph tests. |
| R06.2 | PASS | Independent TestPrunedSchemasPreserveRealSDKRequestsAndNontrivialResponses11/11: valid real SDK projections and invalid required/nested/enum/format variants against both validators. |
| R06.3 | PASS | Published manifest/baselines and generated reference match; publication test reports byte reduction; exact revision CI succeeds. |
| R07.1 | PASS | buildinfo safe version/revision/nullable dirty; withBuildIdentity on real results; installed715faf5 clean identity independently observed. |
| R07.2 | PASS | Native installed STDIO bridge exposes own revision and backend revision independently, both exact clean715faf5; unknown legacy backend tested. |
| R07.3 | PASS | Installed go build metadata CGO0 darwinarm64 revision715faf5 dirtyfalse; HTTP+STDIO tools negotiate2025 independently of2026 Events profile. |
| R08.1 | PASS | include_diagnostics opt-in closed delivery reasons; legacy gap_reason and old response shape preserved; journal diagnostic tests. |
| R08.2 | PASS | State-key HMAC stable reference; independent restart/retry tests distinct attempt correlations; no raw row ID disclosure. |
| R08.3 | PASS | Preflight/dispatch failure correlation created before execution and returned in details; final_audit tests verify same local log ID. |
| R08.4 | PASS | Allowlisted logs/diagnostics inspected and tested; no payload/URL/credentials/native IDs; high-cardinality IDs not metric labels. |
| R09.1 | PASS | Two literal searches then bounded known-chat/known-period browse3x20; no synonym/translation crawl or account dump. |
| R09.2 | PASS | Entrypoints route to references conditionally; trust, explicit permissions, stable UUID, unknown/partial and no auto-read/react retained. |
| R09.3 | PASS | Both quick_validate PASS; links/reference checks PASS; static safe workflow walkthroughs direct context, deferred tools, multilingual browse, blocked/resume, duplicate/unknown/rights/hostile text; synthetic resume independently race-tested. No model-quality claim. |
| R09.4 | NOT_RUN | Source/install file sets independently identical8+9files; saved target-host task migration and companion binding not performed/accessible. |
| R10.1 | PASS | Live27tool descriptors exactly match manifest; legacy search/context identify group-only compatibility and typed alternatives. |
| R10.2 | PASS | Typed route documented; live group metadata remains separate; old consumers and resources retained with regression tests. |
| R11.1 | PASS | scripts/reference workflow lives in repository; no private runtime/network/tmp dependency; independent checker PASS. |
| R11.2 | PASS | Exact715 CI both OS execute checker and5negativecontrols; independent27tools/5resources/4templates/6Events/54reactions/20CLI/117links PASS. |
| R11.3 | PASS | Architecture prose read independently, stale counts fixed; checked ownership/transport/storage/permissions/callback-vs-processing guarantees against source. |
| R12.1 | PASS | GitHub run39112765094 exact715faf5: root test/race/vet, both-module lint, architecture, mandatory offline and tooling all success on Ubuntu/macOS. |
| R12.2 | PASS | Same exact CI: nestedzcago test/race/vet and tooling checks success both OS. |
| R12.3 | PASS | Same exact CI CGO0 darwinarm64/darwinamd64/linuxamd64 builds and native startup cleanup success; installed cleanCGO0arm64 metadata independently checked. |
| R12.4 | PASS | Public GitHub commit715faf5 verification verified=true reason=valid; exact head CI39112765094 completed success bothOS independently queried. |
| R12.5 | PASS | Independent17backup-file SHA verification; live SQLite integrityOK/schema20; all12protectedtable prior row hashes retained; config/plist hashes unchanged; send/read/react grants empty. Optional-table-aware rollback gate tests independently PASS. |
| R12.6 | PASS | Independent installed HTTP+nativeSTDIO27descriptor exact manifest; backend/bridge715faf5 dirtyfalse; authenticated/connected119 messages, safe list/resource reads; Events6 inclcontrol read-only discovery. No subscribe/ack/native writes. |
| R12.7 | NOT_RUN | Reviewer A full100% verdict not supplied/assumed. B does not consult A before own verdict. |
| R12.8 | NOT_RUN | This independent B verdict39/43, not100%; known operational host gates remain. |

## Remaining operational scope

R01 synthetic server/adaptor proof does not prove real target-host wakeups. No companion is active in production; saved host task was not migrated. This blocks R01.2 and the combined R09.4 criterion; source-installed skill matching alone cannot turn that row PASS. Both final100% verdict criteria also remainNOT_RUN.

The user has subsequently selected architectural decoupling of durable journal processability from callback delivery. This report audits frozen715 only; it is not acceptance of that not-yet-implemented architecture or authorization to substitute the old companion route for it. Re-review affected contracts/code/tests/skills and deployment after implementing that decision.

## Closed findings

Legacygroup URI fallback, stale reference counts, real-valid-fixture equivalence gap, broad revoked-head progress, and downgrade recovery guards were all rechecked and closed. No mathematical universal proof of correctness or model behavior is claimed.

## Runtime verification note

The root helper initially assumed nonexistent /readyz and conflated negotiated SDK protocol with2026 Events headers. B independently verified HTTP+STDIO using negotiated2025-06-18 for core tools and separate pinned2026 metadata/Mcp-Method for read-only events/list. The404/400 helper errors were not backend failures. Runtime readiness is evidenced by authenticated connected status and successful exact-revision reads, not invented health endpoints.
