# Architecture migration acceptance

Scope: restricted offline acceptance of T01–T09, SDK-01 and T10. Model evaluation, live account checks, deployment, LaunchAgent restart, publication and push are NOT_RUN. Historical live results are not transferred.

The final architecture, dependency lock/provenance matrix, executable public contracts and privacy contract are in this directory and `docs/contracts`. The final source digest, leaf requirements, exact commands/results and independent final reviews are under `evidence/T10`. The T10 commit SHA is supplied in the handoff after commit; its parent is `88b723bc37abdf9f3f50c69dfc4d7115af330be2`.

## Accepted task commits

- T01: `a3151c6bbee589cfaa3aeb21b147afe59e2ad321` — `test(mcp): freeze migration compatibility baseline`
- T02: `23b3e200e18ad0d0433f38fe4832207c01bb74d2` — `build: pin migration toolchain and architecture checks`
- T03: `da3d7b6f0369f85e58d17b546c933b362c342224` — `test(eval): add evaly harness and reproducible MCP baseline`
- T04: `775d3a271110b2771a9cc0ac63d3f51151481e53` — `refactor(search): extract application handlers and read ports`
- T05: `45230af90f90cf27358f41e5ff52949fd686525d` — `feat(runtime): execute search tools through scoped toolsy registry`
- T06: `a0ed93568dd3dae388e4bc65142becceb5085890` — `refactor(mcp): migrate reads archives and resources to application ports`
- T07: `bcab7133d00a34db8bfc148fee80480d05c1126f` — `refactor(operations): route commands through toolsy preserving durable owners`
- T08: `61d8b6c31ccdcbc107ae92114000ba677e53110d` — `feat(observability): instrument MCP execution and workers with metry`
- T09: `bb3eadc785366588b88bdc83c4d7693c89b96090` — `refactor(mcp): remove migration glue and verify behavior parity`
- SDK-01: `88b723bc37abdf9f3f50c69dfc4d7115af330be2` — `fix(mcp): preserve JSON-RPC IDs across SDK and wrappers`

T10 message: `chore(mcp): finalize architecture migration acceptance`. Acceptance is conditional on the machine-readable T10 report and both new independent reviews; a draft alone is not a PASS.

## Evidence and limits

- SDK-01: exact signed-int64 HTTP/STDIO/discover/resources/Events IDs, bounded invalid-ID framing, selected cancellation/reused-ID race and lost-response domain-UUID no-resend are accepted. Remote fork replacement is explicitly authorized; removal requires the same full ID01–ID07, offline and performance contract on an unmodified official SDK.
- Offline: 50/50 cases, 480/480 assertions; paired parity: 50/50 complete canonical pairs. Only two explicitly named baseline envelope-precision corrections are allowed; native/string IDs, effects and all other facts remain strict.
- Latest dependency-affected performance: SDK-01/performance-comparison-4347-v1.json, all nine gates PASS, 15,000 calls and zero errors. Original T03 allocation/byte baseline is unchanged. Objects ≤2× and bytes ≤1.20× are separate gates.
- User-approved absolute p95 limits are Read 7.8683 ms, Search 7.0726337 ms, Context 6.9967799 ms. They were calibrated from a prior candidate plus 10%; PASS under these revised budgets does not establish original-baseline latency parity. All earlier FAILs and raw runs remain archived.
- Privacy: diagnostics event_trace_file remains opt-in and rotated, with bounded approved fields/random correlation IDs only. Full trace has been removed; no texts, nested payloads, native IDs, tokens or raw errors are allowed. Delivered Events are unchanged.
- Missing model-driver runs remain NOT_RUN/INCONCLUSIVE with nonzero exit. Offline scripted success is not evidence of model recall, model behavior, paid evaluations or live production safety.

Rollback and separately authorized live checks: [rollback.md](rollback.md).
