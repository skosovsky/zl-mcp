# L06 acceptance

Parent: L05 9a01f7e. Contract first: lint configuration and all numeric performance budgets remain unchanged.

| Leaf | Requirement |
|---|---|
| C01 | Exact golangci-lint 2.14.0, sequential root/tools full lint in make check and CI; no config relaxation or autofix. |
| C02 | Current builds require exact Go 1.27.2; measured/build versions agree. Historical Go 1.27.1 evidence replays without rewriting; OS/arch still agree. |
| C03 | Root/tools/patched-zcago tests/race/vet, architecture negative controls, three CGO-free builds, native startup cleanup and offline suite PASS. |
| C04 | Fresh five-run performance: 15000 calls, raw records/provenance, unchanged alloc/B/op/absolute latency gates; fail closed. |
| C05 | All diagnostic IDs/history preserved, successful full loaders report 0/0, nolintlint passes. |
| C06 | User edits preserved; no third_party/siblings changes, deployment/accounts/publication/model/live. Model/live NOT_RUN. |
| C07 | Independent final completeness100% and correctnessPASS for frozen candidate before atomic commit. |
| C08 | Approved zero-window shortcut preserves arrays/cancellation and all required guards, namespaces, reply/coverage/budget/nonzero ordering; regression tests PASS. |

Current status: **ACCEPTED for the authorized offline scope**, completeness100% (8/8), correctnessPASS for approved candidate095d275d…. Historical c70 candidate failed C04 and remains preserved. Atomic task commit created; post-commit root/tools tests PASS.

## Completed checks

make check PASS (exit0): pinned full lint0/0 with successful loaders/nolintlint, root/tools/patched-zcago tests/race/vet, architecture host+hidden100% with14isolated+6source controls, threeCGO-freebuilds and offline50/50 cases480/480 assertions. Final tools tests/race/vet rerun after final code edits PASS. Native startup cleanup race PASS. Version/provenance negative tests and TestMain build-failure cleanup PASS. Config checksum dec18ea507979a4c6f58ce2f47a49f784fdbd91615682fe23182057996640335 unchanged; all2790diagnostic IDs/history retained (0OPEN).

Go1.27.2 is required for new current-build; toolchain_pin distinguishes current provenance. Unversioned candidate manifests remain historicalGo1.27.1 only. Exact token checks reject1.27.20 and missing/changed current policy, not substring matching. Historical baseline numerical budgets and raw evidence are unchanged. Comparator reports both toolchains explicitly.

Historical pre-fix source manifest c70bae4463ff7af3369b9109d2ab2112f512592bc75324084c58fa5e20f207cc. Difference from preliminary49f9095… is docs/lint-findings.md status/date and report pointer only. Generated lint evidence excluded from SourceFingerprint to avoid report self-reference; source/config/development docs remain fingerprinted.

Earlier L06 loader run failed on an obsolete unused strings import; corrected before successful full lint. Two unnecessary test nolint directives were removed after nolintlint rejected them. These failed runs are not acceptance.

Historical pre-fix performance **FAIL**; independent code/evidence correctness PASS, completeness71.43% (5/7), final100% acceptance BLOCKED. Model/live NOT_RUN. No deployments, LaunchAgent, accounts, paid model evaluations or publication.

Pinned lint negative control PASS: fake2.14.00 rejected before any analyzer call (expected make exit2). Performance build PASS with exactgo1.27.2 and current policy pin.

Rollback handoff: previous accepted lint step is9a01f7e (L05); L06 changes developer checks/CI/provenance, no wire or persistence migration. Historical migration/dependency rollback handoff remains intact. No rollback/deployment has been executed.

## Prior performance and independent reviews (before approved Context fix)

Measurement completed: five runs × three kinds ×1000calls =15000calls, zeroerrors, all cleanup/protocol/provenance checks PASS. Measurement manifest PASS means complete valid observations; it does not override the comparison gate. Raw first/fourth/fifth Context excursions remain preserved. Comparison **FAIL**, exit1: Context median-of-five p95 **0.007190166s** exceeds unchanged **0.0069967799s** by2.76393%. Other eight gates PASS; all allocations <=2×baseline and B/op <=1.20×baseline. Read medianp95=0.007118416s; Search=0.006647583s. BaselineGo1.27.1 and candidateGo1.27.2 explicitly reported. Cause is not established; results do not prove a compiler or code regression, nor establish environmental causality. No retry or threshold calibration used to hide failure.

Independent completeness reviewer: C01/C02/C03/C05/C06 PASS; C04 FAIL; C07 BLOCKED =>5/7=71.43% (5/6 substantive leaves=83.33%). Independently recomputed all15rawrecords/medians/provenance. Independent correctness reviewer: code/evidence PASS, no findings for c70bae44…; full acceptance FAIL and L06 must remain uncommitted.

## Historical handoff before approved repair

Last accepted task commit9a01f7e (L05). Frozen L06 candidatec70bae4463ff7af3369b9109d2ab2112f512592bc75324084c58fa5e20f207cc and all user config/dependency edits remain in the working tree. No third_party/sibling changes. Required decision requested: preserve latency FAIL/handoff, or authorize a separate Context latency investigation with unchanged budgets. Until then do not commit L06, claim100% completeness, reset evidence or raise limits.

Reproduce existing comparison with `.tools/zl-mcp-harness compare-performance --baseline docs/mcp-migration/evidence/T03/performance-baseline/manifest.json --candidate docs/lint-evidence/L06/performance/manifest.json --baseline-build docs/mcp-migration/evidence/T03/baseline-build-manifest-v2.json --candidate-build docs/lint-evidence/L06/performance/build-manifest.json --output /tmp/zl-lint-L06-replay.json`. Historical raw commands/build paths are preserved; comparator validates retained records without rebuilding baseline.

Model/live **NOT_RUN**. Deployment, LaunchAgent, real accounts, paid model evals and publication were not run.

User decision: separately investigate Context latency with unchanged limits. Prior full-run FAIL remains preserved; exploratory profiling is not acceptance and does not authorize a limit change. L06 remains uncommitted pending acceptance.

Authorized L06 extension: user explicitly approved zero-window SQL shortcut and regression tests, followed by a new full performance run under unchanged budgets. Additional leaf C08: empty windows retain nonnil arrays/cancellation; access, anchor, reply, coverage, budget, effects/persistence and nonzero ordering unchanged; legacy/typed namespaces, denied/missing/invalid/canceled/closed cases and reply deletion covered. Original fullrunFAIL must remain historical evidence; revised finalcandidate requires independent reviews and fresh checks.

Approved zero-window candidate095d275d44ee81743eaa9ffd04448e8e106f80be14dc392712e348ba0ceb37e9 supersedesc70code for finalacceptance. Both manifests retained. Reviewer preliminary C08 implementation complete, correctness findings none. Added fixture initially had speculative unused nolint directives; removed all unused entries. After adding anchor Unicode/identity assertions, actual cyclomatic31 required local gocyclo/cyclop alongsidegocognit, with AAA lifetime reason. No thresholds/config changes. Final diagnostics/checks/newperformance pending.

Final candidate095d275d… completed makecheck/native PASS: root/tools/patched-zcago tests/race/vet, architecture100%14isolated+6sourcecontrols, threeCGO-freebuilds, offline50/50cases480/480assertions, fresh fullroot/toolsdiagnostics0/0 +nolintlint/loadersPASS. Configunchanged and2790IDs preserved. New fullperformance underway; originalFAILretained.

## Final repaired candidate results

Candidate095d275d44ee81743eaa9ffd04448e8e106f80be14dc392712e348ba0ceb37e9: fullfreshfive-run performance **PASS**, comparator exit0,9/9unchangedgates.15000calls/0errors. Median p95: Read0.007012042s<=0.0078683; Search0.00630125<=0.0070726337; Context0.006714417<=0.0069967799. Median allocations:1585.683/2132.891/1956.629; B/op496395.28/548765/536487.864. All objects<=2×baseline and bytes<=1.20×baseline. Raw15records/stdout/stderr/buildmanifest/comparison/checksums retained inperformance-zero/. Original performance/ comparison remainsFAIL, immutable.

No runtime diagnostic envflags, profiling hooks or overlays are included in product code. Only approved zero-window guard and regression tests were applied. Diagnostic source artifacts have.go.txt extensions, keeping them out of build/package graph. BaselineGo1.27.1/candidateGo1.27.2explicit incomparison.

Final independent reviews: completeness **100%** (8/8), correctness **PASS**, no findings, candidate095d275d44ee81743eaa9ffd04448e8e106f80be14dc392712e348ba0ceb37e9. Model/live **NOT_RUN**.

## Final handoff

L01–L06 completed as six atomic task commits (L06 commit recorded by git history). Prior accepted commits: L01 4bd21b31, L02 44b44e10, L03 114f567b, L04 9e8865e3, L05 9a01f7e. No source/config edits after final freeze or performance build. User config/dependency edits preserved in L06. Historical migration acceptance/rollback evidence remains intact; no frozen baseline build is reintroduced.

Use `make check` for mandatory product gates (no Python or model/live execution), `make bench-mcp` and `make performance-acceptance` for a fresh separately paced performance gate. Replaying final archived data requires the same compare-performance command above with candidate/build paths changed toperformance-zero/. Reverting L06 changes lint/developer provenance and the approved zero-window optimization only; no wire/persistence migration or account repair is required. No rollback/deployment/publishing executed.

Post-commit root/tools go test ./... PASS (logs retained); reviewed14sourcehashes unchanged, source-only gitdiff--check PASS. Raw command logs and unified diagnostic patch intentionally retain original whitespace; artifact-only gitwhitespace warnings are preserved, not source findings. Exactlysix L01–L06 commits; workingtree clean after evidence amendment.
