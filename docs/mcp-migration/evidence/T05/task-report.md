# T05 scoped toolsy execution

Status: ACCEPTED offline; completeness42/42=100%, correctnessPASS with zero findings on the same candidate. Mandatory checks PASS.
Parent:775d3a271110b2771a9cc0ac63d3f51151481e53.
Candidate source digest:4860680b8d2b0b4e878d6332248a40c316e59da983438c892fe9a905353369c6.
Source-only tree:92cf51946c3a08c81a9564ad9d6684c3bb0947e4.
42 equal leaf requirements and ADR fixed before implementation.

Public pinned toolsy v0.20.0 is now a direct runtime dependency; root module sums
match the T02 dependency matrix, no sibling replacement/vendor change. RegistryView
executes exactly four first-slice tools; native SDK registration/annotations/catalog,
transport/resource code and wire projection remain authoritative. Runtime imports
app/domain/contracts and toolsy, never SDK or concrete storage.

Trusted host binding carries local principal/account and deterministic policy
revision with private owner capability. Zero/foreign/direct view authority is denied
before app dispatch. Scope is re-resolved immediately before execution; JSON subject
injection is rejected by original executable schemas. Unbound local corpus reads
retain original support and private owner isolation. Account binding is not changed.
Native preflight extraction preserves exact query/date/cursor/policy error ordering;
preflight selects no messages and executes no FTS. Context access checks are shared
with original context reads. Current Store checks remain authoritative at selection.

Compatibility binder preserves numeric-string limit/before/after and existing
missing/default/explicit zero behavior. Host AssertFormat stays enabled. NewProxyTool
consumes original input/output schemas; domain errors survive policy/handler wrapping,
technical causes remain private to existing safe MCP envelopes. Invocation-owned
RunEnv/result holds T04 native page continuation; shrink never dispatches another
handler. No OperationProfile/filejournal/result cache/session/auto-retry added.

Meaningful tests use borrowed real store and actual SDK: four handlers once each,
invalid-format/denied calls with zero selections, independent full resource, actual
SQLite admission-statement trigger counters proving exactly one admission per call
and a separate one for resource. Runtime checks prove zero/foreign/direct denial,
revoked revision, current direct/group access, technical error preservation,
normalization, big IDs/Unicode/explicit zero, cancellation and32 concurrent distinct
calls through the same immutable view with no arguments/results crossing.

Root tests/vet, relevant MCP/storage/contracts, race, architecture100/100 with10
isolated/all-build source controls PASS. Candidate offline50/50 cases480/480 asserts;
frozen baseline25/25 cases240/240 asserts. Build manifest source digest matches;
frozen baseline binary is byte-identical to T04 (original business/T03 overlay).
Raw logs/traces/effects retained. Initial compiler attempts and corrections disclosed.
No T05 performance acceptance claimed; T03 baseline/T09 comparison remain mandatory.

Model/live NOT_RUN; SDK precision gate DEFERRED_TO_SDK-01; ID01-ID07 not claimed PASS.
Offline scope only. T01-T04 not reopened. No deployment, LaunchAgent, live account,
paid model eval, publication, third_party or sibling library changes.

Sole commit only after reviewers100%/PASS:
`feat(runtime): execute search tools through scoped toolsy registry`.
