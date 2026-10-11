# T03 task report — accepted offline candidate

Parent: `23b3e200e18ad0d0433f38fe4832207c01bb74d2`.
Parent tree: `53106222d3a2fd6bcb7098803a3d0d3f29b15fd6`.
Candidate source digest: `ef92dd136f7e5ac40e057f741a162a5e90703988b8090ade6ba785f033354a48`.
Leaf requirements: 52, unchanged denominator. Independent A52/52=100%; B=PASS,
zero open findings, same source digest. Both verdicts saved before the sole task commit.

Public evaly v0.4.2 owns the typed sealed dataset, target, isolated lifecycle,
capture and deterministic graders. All 25 scenarios use real MCP SDK entry and
existing storage/messaging/history/join/archive owners. The forwarding archive
constructor borrows existing owners; production persistence and business logic
are unchanged. Durable upstream effect journals are read independently by the parent.
Watchdogs bound output and wall time, kill process groups and Wait, including after
normal parent exit. Cleanup runs after failures and restart failures.

Final candidate: 50/50 cases and 480/480 assertions over two fresh isolated repeats.
Frozen business baseline: 25/25 cases and 240/240 assertions. Exact paging IDs,
progress and Events ordered identity/receipts survive redelivery and ACK/restart.
CLI assertion failure/missing child return1; time exhaustion/unknown profile/model
preflight return2 with reports. Root tests/vet, harness race/negative controls and
architecture compliance/coverage100% plus production dependency isolation PASS.

The baseline sources remain original T01 HEAD
`8d913c48b797f54f277ffc1106e6bf28be9e697d`. Build manifests disclose the evaluation-only
fixture/driver overlay and forwarding constructor, pinned Go1.27.1/dependencies and
binary hashes. The original T01 binary remains separate for rollback/behavior.
Final build differs from the review digest only by the executable CLI-negative
checker added and run afterwards; all Go sources match. Performance retains the
frozen v2 measurement binary with identical benchmark/client/fixture/Go/dependency
closure (`measurement-source-parity.json`); final model-only preflight changes do
not enter measurement. No architecture or toolchain improvement is claimed.

Isolated performance: five warmed runs, each1000 calls per read/search/context,
concurrency1 per kind, three parallel independent kinds, admission enabled,
exporters disabled, Apple M1 Max/darwin/arm64, Go1.27.1. Each kind5000 measured calls,
errors0. Median-of-run p95 read3.563834ms, search3.375625ms, context3.545208ms;
allocations/op867.640,1144.270,1066.053 respectively. T09 ratios1.15p95/1.20alloc
fixed before baseline. Initial benchmark overlapping checks is diagnostic only;
isolated acceptance run is retained independently. Denied sandbox CPU metadata
read and successful read-only host metadata are recorded; no thresholds changed.

Model/live/full paired model acceptance: NOT_RUN. The model adapter is protocol
scaffolding with180s/30call bounds and synthetic admission/preflight tests. Only
normal-search protocol setup exists; the public full-suite preflight never launches
a driver or accepts a subset. Full model scenario lifecycle and model ID/settings,
prompt/skill revisions and token measurements require a separately authorized run.
Tokens are null with an unavailable reason. This report accepts only offline scope.
SDK numeric JSON-RPC ID defect remains characterized, DEFERRED_TO_SDK-01; ID01–ID07
are not claimed PASS. T01/T02 remain accepted and unchanged.

Initial failures, reviewer findings and resolutions retained. Raw synthetic traces,
independent effects, skipped/failed cases, commands/log hashes, source/build hashes,
performance samples and machine metadata are indexed. No live account, model cost,
deployment, LaunchAgent restart, publication, sibling library or third_party changes.
User specification amendments preserved byte-for-byte.

Sole task commit only after both final passes:
`test(eval): add evaly harness and reproducible MCP baseline`.
