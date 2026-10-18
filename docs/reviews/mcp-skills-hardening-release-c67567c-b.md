# Independent causal-readiness release review B

Reviewed public signed clean source: `c67567c54408899bc729cf4afc488ba9eaa5217c`.
**Release verdict NOT ACCEPTED:40/43=93.02%; FAIL0; NOT_RUN3. Unresolved discovered implementation defects:0.**

This review applies both MCP/server and AgentSkills best-practice skills. Actual source, contracts, schemas, generated reference, skills and targeted normal/race tests were independently checked. Reviewer A verdict was not consulted. New model/live evals were not run; no production mutations/subscriptions/acknowledgements performed.

The accepted primary architectural direction is store/ready -> callback, with independent transport receipt and processing acknowledgement. Optional companion is separate crash/missing-run insurance, not the ordinary race fix. Persisted mode shares existing durable consumer checkpoint and retained serializedbytes, preserving strict default input/output behavior.

## Frozen43 criteria

| ID | Status | Evidence |
|---|---|---|
| R01.1 | PASS | docs/contracts/journal-processing-readiness.md and executable opt-in readiness_mode=persisted: ready/transport/ack independent; default delivery_terminal strict representation unchanged. |
| R01.2 | PASS | Independent actualSDK HTTP/TLS -race TestSyntheticHostPersistedJournalAcknowledgesBothCallbackRaceWindowsWithoutCompanion: read+ack before2xx and post2xx/precommit, sending truthful, no companion/new message. |
| R01.3 | PASS | Independent -race actualSDK503/timeout+restart proof keeps processing checkpoint and exact callback ID/body; separate companion crash/missing-run recovery prior synthetic tests preserved. No real-host activation claim. |
| R01.4 | PASS | Persisted order/generation/permissions/recall/retention/cancel/410 tests; earlyack leaves workerbytes/lease unchanged; malformed nested/100k/domain-invalid activity normalized before budget to receipt-bearing gap; validoversized remains error not false gap/ack. |
| R02.1 | PASS | coverage_mode full default; compact opt-in discriminator; existing full keys; TestCoverageGapsSnapshotAndCurrentPolicy. |
| R02.2 | PASS | SDK budget tests for800+ gaps and bounded compact anchor/search/browse; independent SDK budget tests PASS; exact-c675 CI39115464885 completedSUCCESS bothOS. |
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
| R06.3 | PASS | Published manifest/baselines and generated reference match; publication test reports byte reduction; independent generator/SDK fixture tests PASS; exact-c675 CI39115464885 completedSUCCESS bothOS. |
| R07.1 | PASS | buildinfo safe version/revision/nullable dirty and SDK tests; independent installed HTTP c675 backend revision exact and dirtyfalse. |
| R07.2 | PASS | Independent installed nativeSTDIO c675 bridge and serving backend both report exact clean revision; outputdescriptors equal HTTP/manifest; unknown legacy backend path covered by tests. |
| R07.3 | PASS | Independent installed binary SHA b9111ff797af83629e029cb26f09db4ddbb612066de3a574b499fb869c7e1ce7; Go metadata exactc675 dirtyfalse CGO0 darwinarm64. CoreHTTP+STDIO2025 and separateEvents2026 read probes PASS. |
| R08.1 | PASS | include_diagnostics opt-in closed delivery reasons; legacy gap_reason and old response shape preserved; journal diagnostic tests. |
| R08.2 | PASS | State-key HMAC stable reference; independent restart/retry tests distinct attempt correlations; no raw row ID disclosure. |
| R08.3 | PASS | Preflight/dispatch failure correlation created before execution and returned in details; final_audit tests verify same local log ID. |
| R08.4 | PASS | Allowlisted logs/diagnostics inspected and tested; no payload/URL/credentials/native IDs; high-cardinality IDs not metric labels. |
| R09.1 | PASS | Two literal searches then bounded known-chat/known-period browse3x20; no synonym/translation crawl or account dump. |
| R09.2 | PASS | Entrypoints route to references conditionally; trust, explicit permissions, stable UUID, unknown/partial and no auto-read/react retained. |
| R09.3 | PASS | Both quick_validate PASS; links/reference checks PASS; static safe workflow walkthroughs direct context, deferred tools, multilingual browse, blocked/resume, duplicate/unknown/rights/hostile text; synthetic resume independently race-tested. No model-quality claim. |
| R09.4 | NOT_RUN | Source/install skills independently byte-identical17files; saved real-host task migration to explicit readiness_mode=persisted unavailable/not performed. File synchronization alone cannot satisfy combinedcriterion. |
| R10.1 | PASS | Live27tool descriptors exactly match manifest; legacy search/context identify group-only compatibility and typed alternatives. |
| R10.2 | PASS | Typed route documented; live group metadata remains separate; old consumers and resources retained with regression tests. |
| R11.1 | PASS | scripts/reference workflow lives in repository; no private runtime/network/tmp dependency; independent checker PASS. |
| R11.2 | PASS | Exactc675 CI39115464885 bothOS generatedreference and5negativecontrols steps success; independent checker27tools/5resources/4templates/6Events/54reactions/20CLI/118links PASS. |
| R11.3 | PASS | Architecture prose read independently, stale counts fixed; checked ownership/transport/storage/permissions/callback-vs-processing guarantees against source. |
| R12.1 | PASS | Independent GitHub query exactc675 CI39115464885 completedSUCCESS bothOS: root test/race/vet, lint, architecture100%, mandatory offline and tooling. Local frozen offline50/50cases480/480assertions. No newmodel/liveevals. |
| R12.2 | PASS | Same exact CI: nestedzcago test/race/vet plus tooling mandatory steps independently confirmed success on bothOS. |
| R12.3 | PASS | Same exact CI CGO0darwinarm64/darwinamd64/linuxamd64 crossbuilds and native startup cleanup PASS; installed CGO0arm64 metadata independently checked. |
| R12.4 | PASS | Public signedc675 GitHub verifiedtrue/reasonvalid; exacthead run39115464885 statuscompleted conclusionsuccess independently queried, bothmacOS/Ubuntu jobs and allrequiredsteps success. |
| R12.5 | PASS | Independent stoppedbackup17filehash verification; liveSQLite integrityOK/schema20/all12protectedtable priorrowhashes retained; config/plisthashes unchanged;18savedops, grantsclosed,nocompanion. Recovery-aware rollback guard previously independently tested. |
| R12.6 | PASS | Independent currentinstalled HTTP+nativeSTDIO27descriptors exactly matchmanifest; backend/bridge cleanexactc675; authenticated/connected119messages;safeconversation/resource reads; separateEvents2026list6profiles; persistedfield advertised. No ack/subscription/nativewrites. |
| R12.7 | NOT_RUN | No independent reviewerA100%fullscope verdict assumed; hosttask criterion remainsunproven. |
| R12.8 | NOT_RUN | This B verdict40/43=93.02%, not100%; outstanding savedhosttask prevents fullacceptance. |

## Independent checks atc675

- GitHub commit verification:true/valid; clean localHEAD exactlyc675.
- `go test -race ./internal/mcpserver ./internal/adapters/store ./internal/storage -run 'Persisted|Projected|JournalProjection' -count=1`:PASS(11.824s/1.397s/10.777s).
- Reference checkerPASS:27tools,5resources,4templates,6Events,54reactions,20CLI,118links.
- Reviewed pure per-entry projector: storage owns policy/identity/profile/receipt; adapter cached compiled authoritative contract+DTO validates after thoseguards and before28KiB budget; invalidentry becomes bounded explicitgap, infrastructure failure remains error. No docs/contracts import in storage.
- ActualSDK poisonedhead scenarios cover data={},100k malformeddata,and schema-valid/domain-invalid activity; gapACK releases followingvalidrecord while rawoutbox payload/state unchanged. Validoversized negative control remains RESPONSE_TOO_LARGE without falsegap/ack.

## Remaining gates and verdict

All release/installation/runtime gates now have independent proof. The only substantive uncompleted requirement is the saved real-host automation migration inR09.4. Updated source and installedskills17files match byte-for-byte, but actual savedtask has not been changed through a supported host tool; no production companion was created. Neither old host behavior nor synthetic proofs establish that savedtask now requests persisted mode. This cannot be markedPASS by localfile edits or private API bypass.

Consequently both full100% review gates R12.7/R12.8 remainNOT_RUN. Frozen denominator43 is unchanged:40PASS,0FAIL,3NOT_RUN=93.02%. Errorsfoundandunresolved0 does not imply objective achieved. Final verdictNOTACCEPTED.

## Final independent installed proof

Public source and current installed backend: c67567c54408899bc729cf4afc488ba9eaa5217c, cleanCGO0darwinarm64. InstalledSHA b9111ff797af83629e029cb26f09db4ddbb612066de3a574b499fb869c7e1ce7. CI39115464885 exacthead successbothOS verified separately.

Independent token-in-memory read probes confirmed HTTP/nativeSTDIO exact27 descriptors, exactbackend+bridgeidentity, statusauthenticated/connected119messages, safeconversation/resource reads, separate6profileEvents discovery. Privatebackup17fileSHA verified; all12protectedtables baseline rows preserved; liveSQLiteintegrityOK/schema20;18savedoperations; unchangedconfig/plist; send/read/reactgrantsclosed; recoverycompanions0; installedskills17fileexactmatch. No secrets/content/nativeIDs were printed and no production acknowledgements/subscriptions/actions were performed by reviewer.

All earlierB findings including legacypaging, schemafixtures, broad revocation progress, rollbackcontrol compatibility, and poisonhead-beforebudget have been closed through independent rechecks.

## Host catalogue boundary

Independent inspection of this chat's loaded connector metadata found `mcp__codex_apps__zalo_mcp_zalo_read_subscription_events` still declares only limit/subscription_id, the old pending-blocks description and terminal delivery-state output. It cannot pass readiness_mode under its currently exposed schema. In contrast, direct installed HTTP discovery matches the new manifest with explicit persisted support. This demonstrates this chat's cached-catalogue discrepancy; it does **not** establish the exact catalogue or savedtask state in Ann's separate run. A supported host catalogue refresh and savedtask reread/update must be proved there; unavailable host mechanisms are an honest operational blocker, not a reason to bypass private APIs or claim migration.
