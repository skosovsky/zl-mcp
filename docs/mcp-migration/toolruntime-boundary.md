# ADR: scoped toolsy execution for first reads (T05)

Status: contract fixed before implementation. Only four T04 search/context tools
move through internal/toolruntime; native SDK AddTool, annotations, HTTP/STDIO,
resource handlers and safe projection remain MCP-owned. Other reads, archives,
commands and Events retain their existing paths until the declared later tasks.

Public toolsy v0.20.0 Registry/RegistryView.Execute and NewProxyTool consume the
same raw executable input/output schemas from docs/contracts. Host AssertFormat
validation remains mandatory. Compatibility binding changes only the previously
characterized limit/before/after numeric strings and existing default semantics;
explicit zero, null/missing, additional properties and opaque IDs stay distinct.
Malformed or format-invalid calls never dispatch an application handler.

The trusted host resolves subject/scope from its local transport/composition and
current borrowed account/policy owner. These values cannot be supplied in JSON
arguments or learned from returned messages. The scoped execution gate rejects a
bare direct registry/view call without valid host authority before the handler.
Current read access continues to be checked by the store adapter on every read;
prior search is never a grant for resource access. Authorization factoring must
retain existing domain errors and validation/cursor error precedence. Capability
view and invocation-local environment are separate from durable operation ledgers.

Each accepted invocation calls one application handler once, with inherited
context/deadline and one existing admission decision. Search budget shrinking
retains the invocation-local native page capability from T04 and never executes
another tool call/FTS query. MCP applies the original64KiB wire projection/output
schema/resource links after execution; native20KiB context and1MiB resource caps
remain intact. Existing domain errors remain available through runtime wrapping;
internal details never become new wire messages. No OperationProfile, filejournal,
result caching, automatic retry, second DB, shared session or extra worker is added.

Registry/view composition is immutable and concurrent; call/environment/results
are invocation-owned. Runtime imports app/domain/contracts and public toolsy,
never SDK or concrete storage. Service/fixture retain storage ownership and bind
the trusted runtime to their borrowed read adapters. Root and nested dependency
pins remain governed by the accepted T02 matrix; sibling libraries and third_party
are outside scope. Frozen baseline still takes accepted T03 overlay from its SHA.

Acceptance uses42 fixed equal leaves in evidence/T05/requirements.json, actual
SDK invocation counters, fail-closed direct calls, invalid formats, wire/catalog
parity, independent resource permission checks, race, architecture and offline
evaluation. Two independent reviewers must accept the same candidate before the
sole task commit. Model/live remain NOT_RUN; known SDK precision repair is SDK-01.

Implementation detail: host subject contains the local principal and existing
account digest; scope contains a deterministic digest of the current collection
policy. Both carry private runtime ownership. An unbound corpus remains supported
for existing local/test reads and is isolated by that ownership; no account grant
is manufactured. Store preflight is shared with the original native search path
so invalid query/date/cursor/selection ordering is identical. Preflight selects
no messages and never runs FTS. Domain/technical errors survive the policy wrapper.
Policy evaluation rereads identity/scope and native access immediately before the
single app dispatch; Store checks access again when selecting the actual data.

T09 dependency update: official toolsy v0.20.1 replaces v0.20.0 after the
upstream performance fix. Existing execution, validation, authorization and
BYOT contracts above remain mandatory; no isolated diagnostic patch is adopted.
The dependency matrix records the immutable origin and fresh acceptance evidence.

T09 final pin: official v0.20.2, origin36eb5b661ab4569051e5b17175dc44f3dd132b9c,
replaces v0.20.1. Execution/privacy/persistence contracts are unchanged. Performance
acceptance uses explicit user revision2026-10-12: allocations<=2.0 and B/op<=1.20;
p95<=1.15. Earlier failures retain their original criteria.


SDK-01 user-authorized dependency update selects official toolsy v0.20.5 and
remote SDK fork v1.8.1-0.20261012184819-4347cd438f3c. T09's accepted v0.20.2
measurements remain historical evidence. No local toolsy/SDK patch or vendor
is used; runtime policy/schema/authority and durable owners remain unchanged.
