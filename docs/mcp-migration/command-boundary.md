# T07 command boundary

The accepted T06 read registry is extended to execute the five existing commands.
Public schemas, catalog, annotations, admission, safe error/result projection and
resource behavior remain the executable contracts. The fixed equal acceptance
leaves are evidence/T07/requirements.json. SDK IDs remain SDK-01; model/live
remain NOT_RUN. No deployment, account session or paid model evaluation is needed.

Messaging owns an explicit typed send request/result port; membership owns typed
join plan-token/request UUID; history owns typed import and cancellation ports;
eventaccess owns typed subscription/receipt acknowledgement. These handlers use
existing domain DTOs where exact. Legacy maps belong only to borrowed operations
adapters and wire projection. No generic application Call/map/RawJSON bus is added.

Operations adapters delegate fixed method names to the existing control/native
owners. Events ACK borrows the existing storage journal owner. Native managers
retain domain validation, ledgers, operation IDs, transactions, permissions,
receipts, workers, sessions and restart/cancellation logic. No second owner, retry,
cache, operation pipeline, speculative send or grant is introduced. Send/history
retain35s and join6s child deadlines; Events ACK keeps its original parent context.

Runtime receives principal/account/policy exclusively from trusted host authority.
Commands require the same current scoped registry and executable binder/schema
boundary as reads; foreign or stale scope is denied before effects. Native command
owners still recheck actual send permission, quote authority, approval-token binding,
request equivalence, import bounds and receipt grant/generation/order. CLI local
approval/control continues through the existing authority and domain validation;
CLI approval is not turned into a model-accessible MCP capability.

The complete22tool catalog requires both read and command ports at composition.
The five residual MCP command switch branches and concrete backend dependencies
are removed. Wire admission remains exactly once per call, independent resource
admission remains unchanged, and native transport resources retain their limits.
A03's exact temporary MCP exceptions are removed without broadening other edges.

Acceptance covers duplicate/conflicting UUIDs, revoked grants, concurrent sends/
joins/ACKs, receipt order/redelivery, ambiguous results, subprocess crash/restart,
terminal/inflight history cancellation and history no-events. Required root/vet,
relevant race suites, architecture100/100 with negative controls and complete
independent offline parity/effects evidence precede two independent reviewers.
Only their matching100% completeness and correctnessPASS permit the sole commit
`refactor(operations): route commands through toolsy preserving durable owners`.
