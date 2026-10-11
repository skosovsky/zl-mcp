# T08 telemetry contract

Service owns one pinned metryv0.10.1 Provider constructed through metry.New.
The process never installs global OTel providers. Instrumentation reaches app,
transport and native workers through consumer-owned narrow interfaces; only
observability/composition imports metry/OTel. Domain DTOs contain no telemetry SDK.
Nil/disabled instrumentation calls the original business function directly;
a lightweight disabled observer creates no SDK. The single safe operational
mcp_call audit belongs to the final MCP call boundary, including preflight and
post-execution projection/encode/response-budget failures.
metry.ExecutorWrap is never constructed with nil Provider.

Defaults disable telemetry/export and create no network dependency. Additive
[telemetry] TOML settings use the executable service_telemetry.input.json contract.
The enabled, exporter=none mode uses metry's owned noop export pipeline. Explicit
otlp_http selects trace+metric export with HTTPS or literal loopback HTTP endpoint,
no userinfo/query/fragment. Headers are private configuration, never labels/logs.
The trace batch queue is metry's fixed SDK queue2048 (no block-on-full option);
export timeout defaults1000ms (100..5000), shutdown defaults2000ms (100..5000).
Exporter construction/return/shutdown errors are neutral categories; SDK/global
error handling never receives raw response bodies, endpoint credentials or causes.
Exported resources are restricted to the static service/SDK fields; inherited
OTEL_RESOURCE_ATTRIBUTES cannot inject private host fields into network sinks.

One cached ExecutorWrap per supported tool observes actual Go success/error/panic.
A request host span records an independently classified finite typed business
outcome; its execution child is the metry span. No HTTP execution span is added.
The adapter never reads error/panic String/Error formatting, message/query/quote,
raw URI/token/session, account/chat/message/request/operation identity for fields.
The original error/result/panic reaches the original caller without alteration.
A nil Go error may coexist with business error, unknown, partial or cancelled.
Domain dashboards use zl_mcp_domain_calls, not technical success.

Only stable22tool names, HTTP/STDIO/resource transport, finite operation kind,
state/outcome and finite worker queue names form dimensions. Unrecognized strings
map to a bounded other/unknown category, never a fresh metric series. Signal names,
units and dimensions are frozen in evidence/T08/signal-manifest.json. Operation
age is a histogram sampled at an observed transition/work attempt: nonnegative
seconds since the existing native creation timestamp, kind/state dimensions.
Queue depth is the current native pending/active count, not a cumulative counter.

Request->operation->worker correlation keeps native IDs distinct from trace IDs.
An in-memory cache stores only opaque trace references keyed by native kind/ID,
with a fixed4096entry bound and24h expiry. Keys never enter telemetry attributes,
logs or persistence; eviction/restart may drop correlation and starts a new trace.
Workers link to a retained request reference when available. No migration or new
operation record is introduced for telemetry.

Business/upstream/native ledger logic remains owned by the existing managers.
Observation cannot dispatch/retry a command or make a grant. Export work stays
outside business outcome handling; exporter failure is a telemetry diagnostic.
The service stops and joins recording workers, shuts telemetry down with a fresh
bounded context, then closes the store. Repeated shutdown is idempotent. Existing
operational diagnostics and discovery/redaction remain within the updated safe
logging contract. One final MCP audit is retained with export disabled; app
execution and metry wrappers emit no duplicate operational call log. Audit codes
come from a finite allowlist of direct domain errors; arbitrary/wrapped errors
collapse to STORAGE_ERROR without observation invoking formatting/Is/Unwrap.

Acceptance requires in-memory trace/metric sinks, sentinel privacy including
host resource environment/error/panic formatters, nil/disabled paths, exact
technical/domain distinction, exporter failure/timeout, bounded shutdown and
worker signals/correlation. Root/vet/relevant race, architecture/negative controls,
full independent offline eval precede two independent samecandidate reviewers.
Model/live remain NOT_RUN; SDK IDs remain SDK-01. No installed service is started
or restarted. The sole accepted commit is
`feat(observability): instrument MCP execution and workers with metry`.

Native observations use the existing owners and transitions. Queue depth is a
service-owned sampler (500ms), not an extra SELECT on the command/worker critical
path: each read has a100ms deadline and failures skip the sample. Final sampling
runs after workers join, before provider shutdown/DB close. Send age starts at
the pending ledger's original updated_at (there is no created_at column); join
and history use their existing native creation stamps. Events age starts at the
original event creation stamp borrowed in the existing delivery claim SELECT.
Missing/invalid timestamps omit age samples while retaining transition counts.
No telemetry observation creates a persistence column or changes a record.

Upstream and worker spans classify their Go invocation status with finite
outcomes; asynchronous native business state remains visible through transition
signals and typed status-tool results. Only zl_mcp_domain_calls classifies typed
Result business success, including nil-error partial/unknown/cancelled/failure.

STDIO observation borrows an optional TelemetryPort from its caller context; the
bridge never constructs a second provider. An SDK connection wrapper correlates
fixed tool/resource categories with SDK-parsed IDs held only in a4096entry/24h
memory map. A writer counts actual accepted bytes, including frame newlines.
SDK-batched responses are one frame under bounded tool=other, without zero-byte
samples for buffered replies. No telemetry transport rewrites IDs or wire data.
The installed bridge has no borrowed provider across its separate process;
service-side forwarded calls are measured as HTTP. Optional in-process embedding
can observe STDIO bytes with the existing owner. Default/no observer uses the
original StdioTransport directly. No new IPC/public protocol is introduced.

Review correction contract (T08 C1): before any enabled SDK/exporter construction,
validate OTEL_RESOURCE_ATTRIBUTES syntax without invoking OTel detectors. Every
comma-separated entry must contain = and its trimmed value must successfully
PathUnescape. Reject malformed configuration with a fixed neutral error, without
logging values, changing process environment or installing a global error handler.
Disabled telemetry does not inspect this environment. Export filtering continues
to restrict valid inherited resources to the static safe fields. Fresh-process
tests cover missing =, trailing entries and invalid percent encodings.

Authorized T08 Events logging correction: replace both service metadata and opt-in
TraceLogger records with the exact safe field allowlist in callback.md. Random
per-attempt correlation IDs replace every native/receiver ID; payload and digest
are absent. Trace responses record bounded status only and do not read bodies.
No alternative full-trace mode exists. Wire Events and durable owners are unchanged.
