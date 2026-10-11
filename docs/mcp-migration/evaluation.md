# Offline evaluation contract (T03)

The versioned dataset uses the existing synthetic corpus and independently generated
mobile SQLite archive vectors. Every case declares its permitted tools, effects,
expected evidence and 60-second deadline. The parent owns an isolated temporary
state directory per trial and runs the actual MCP SDK server/client inside a child
process. The child reuses existing storage and messaging/history/join/archive owners.
The fixture does not implement replacement business decisions.

Evaly v0.4.2 owns the typed dataset/target/lifecycle/capture/graders. Host policy
requires every expected case, every grader/assertion, complete evidence and successful
cleanup. Saving an evaly artifact never implies acceptance. Exit0=complete PASS,
exit1=assertion/infrastructure failure, exit2=incomplete/inconclusive. Reports survive
failures. The external watchdog terminates and Waits children; stdout/stderr each
have a bounded budget. Parent finally removes state, including after a hard kill.
Synthetic upstream effects are recorded durably before responses, independently of
result text; partial effect journals remain in failed reports.

Baseline behavior uses frozen T01 business sources at
8d913c48b797f54f277ffc1106e6bf28be9e697d. An explicit evaluation-only source overlay
adds the same extended fixture/driver and a narrow constructor forwarding to the
unchanged service archive reader. This permits scenarios absent in the old fixture
without replacing the frozen business implementation. Overlay paths/hashes and both
compiler/binary manifests are retained. The original T01 binary remains separate.

Offline runs require no account credentials or external network. Model execution is
an explicit opt-in profile, requires a separately authorized host agent command and
budget, uses180-second/30-call case budgets, and otherwise remains NOT_RUN. The CLI
does not silently invoke a model or treat skipped model evaluation as PASS.

Performance uses five independent warmed runs with1000 successful calls per read,
search and context kind, concurrency1 per kind, admission enabled, exporters disabled.
Baseline/candidate must use the same Go, machine, fixture, concurrency and rate pacing.
Timing excludes pacing and includes the complete SDK request/result projection;
allocation measurements record the complete process allocation delta over the same
measured calls. Background activity and pacing configuration are recorded explicitly.
Record p95 per run, median-of-run p95, allocations/op, CPU/OS/errors/bytes/counts.
The fixed T09 thresholds are1.15 for p95 and1.20 for allocations (zero baseline =>0).
No threshold adjustment after measurements. Tokens are null with an unavailable reason.

## Opt-in model host driver contract

The model adapter is **protocol-only scaffolding**. Full paired model acceptance
and all 25 scenario timelines remain **NOT_RUN**. The public CLI preflight preserves
that gate and never executes a driver for the full offline dataset. A subset cannot
produce full model PASS. Only `normal-search` is prepared for protocol smoke; every
other scenario is rejected before driver launch. Model ID/settings, prompt/skill
revisions and token usage are unavailable until a separately authorized full run.

`--profile model --model-driver /absolute/executable` identifies an explicitly supplied
host-owned executable. Without that flag the model gate is NOT_RUN. No default
provider, credentials or account are selected. The case child exposes a fresh
synthetic MCP server on a loopback URL and passes `--mcp-url URL --case-file FILE`
to the executable; FILE is the sealed case contract. The executable owns the agent,
prompt and provider configuration. It must finish within 180 seconds and write a
bounded JSON result on stdout. Server middleware records actual tool/resource
requests and results, enforces the case tool allowlist and rejects every tool call
after 30 before dispatch. Violations fail the case even when the driver exits zero.
The parent watchdog kills/waits the entire case process group and removes state.
Model evidence is distinct from mandatory scripted offline acceptance. This task
only tests the protocol and budgets with synthetic drivers; paid/model execution
remains NOT_RUN until separately authorized.
