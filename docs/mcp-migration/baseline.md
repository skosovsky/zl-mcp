# T01: compatibility baseline

Frozen source: `8d913c48b797f54f277ffc1106e6bf28be9e697d` (before implementation).
The user-owned specification and spec-review were initially untracked; neither is modified.
Initial HEAD/dirty states of zl-mcp, toolsy, metry, evaly and the patched zcago source
are recorded in `evidence/T01/initial-state.json`. `third_party/zcago` is a tracked nested
module of this repository, not an independent Git repository. No sibling checkout is changed.

## Executable authority and inventory

`manifest.json` contains the actual 22 SDK tool descriptors (names, descriptions, schemas,
annotations), their handler/permission/effect/owner, timeout, response/error and binding
contracts. Discovery is frozen in `internal/mcpserver/testdata/migration/`; the test compares
`tools/list` 1:1 with `contracts.Names()` and compares the complete JSON including SDK metadata,
ordering, aliases, schema definitions and annotations. Four static resources and three
resource templates are captured through SDK discovery, with MIME, URI, handler, admission,
policy, payload limits and result references in the manifest. Discovery `cacheScope` metadata
is not a permission grant: resource reads still have independent current-policy checks.

History aliases map import input to history_import_request and status/cancel input to
history_import_operation; all use history_import_operation output. `contracts.Compile`
asserts formats; success output schemas do not imply error-schema validation.
All tools share the existing SQLite 10/s burst20 admission bucket, before validation.
No second admission is introduced. Entire serialized tool results have a 64 KiB budget.
HTTP bodies have a 256 KiB budget. Text/full archive record resource payloads have a 1 MiB
budget; JSON diagnostics payloads have a 64 KiB budget. Collection/capabilities resources
have no additional explicit byte budget in their present handlers: this is characterized,
not a newly invented limit. Bounded pages retain cursors, partial coverage and empty archive
page continuation. Only read handlers may reread during response trimming; write effects
must never repeat to fit the response.

Binding only applies strconv.Atoi to strings in limit/before/after; schema validation
follows. Defaults are limit=20, before=after=5; explicit zeros follow the specific schema.
Tool errors are `isError=true` with text JSON domain error including code/next_action;
protocol and resource errors remain protocol errors. Exact negative tool results are frozen
for every catalog entry. Input characterization freezes malformed object/array, missing/null,
unknown properties, zero/fractional/out-of-range bounds, RFC3339/UUID, Unicode, size and IDs.
Opaque IDs remain strings, including IDs >2^53. HTTP JSON-RPC IDs are characterized separately.

## Known baseline limitation

The pinned SDK v1.8.0 rounds numeric JSON-RPC ID `9007199254740993` to
`9007199254740992`; the string form stays exact. This is a baseline SDK limitation,
not a precision guarantee. Numeric small IDs and null/missing IDs have separate wire fixtures.
Migration must preserve exact opaque business identifiers and introduce no further regression.
The host binder also decodes raw JSON numbers as float64; schemas reject numeric identifiers.
The characterization tests preserve exact JSON numbers using UseNumber in snapshot comparison,
so a new numeric wire regression cannot disappear inside the comparison itself.

## Transport, Events and CLI boundaries

HTTP `/mcp` retains bearer digest authentication, loopback Host validation, Origin rejection,
protocol/media headers and native SDK Streamable HTTP handling. Events subscribe/unsubscribe/list
are an HTTP-only extension; v1/v2 callbacks, signed delivery queue, retention, read and ordered
ACK remain owned by the existing Events subsystem. STDIO forwards tools/resources to the service
with a 40s HTTP timeout and retries disabled; it neither creates a collector nor advertises Events.
Owner-only CLI archive/control/probe commands and approve-join use the private trusted local
control channel. They are inventoried in the manifest and are not added to MCP discovery.

## Reproduction and measurements

```sh
GOWORK=off python3 scripts/mcp-migration/baseline.py --measure --output /tmp/zl-baseline-new-run
GOWORK=off go test ./... -count=1
GOWORK=off go vet ./...
GOWORK=off go test ./internal/mcpserver ./docs/contracts ./internal/service ./internal/messaging ./internal/collector ./internal/historyimport ./internal/events ./internal/storage ./internal/evalfixture -count=1 -json
```

The output directory must be new. The script archives the immutable source commit, builds
zl-mcp and the existing synthetic fixture binary, records source/tree/toolchain/binary/dataset
hashes and build info, starts only the offline STDIO fixture, checks catalog parity and a real
synthetic search, then closes and waits for the child. Nonblocking chunk reads enforce a wall-clock deadline even on a partial line, with
bounded stdout/stderr; termination and Wait happen on failure. Negative tests cover
partial-line hangs and stderr floods. Synthetic state is created in tempdir by the existing fixture.
The baseline binary is not installed; no LaunchAgent/account/service is touched.
`evidence/T01/build-manifest.json` records the actual Go 1.27.1 compiler (the source directive
remains 1.26.5). Rebuild this frozen SHA using the selected T02 compiler for paired performance;
do not attribute a compiler change to architecture. The --measure command runs a warmup plus ten timed real MCP calls for each read/search/context
tool, records latency/counts/errors/bytes with normal admission, and stays within a 60s
case budget. This timing smoke is not the required five-run 1000-call paired performance
gate; allocations remain null with a reason. T03 introduces that full performance
measurement runner before any T04 refactoring. Historical eval runs are not this baseline.

Snapshots compare canonical JSON structurally and preserve array ordering. Discovery and
negative failures have no dynamic exemptions. The empty Unicode search permits only its
root snapshot_at timestamp to vary, requires valid RFC3339Nano, and fixes it in both structured
content and its JSON text projection. Result schemas and existing pagination/resource suites
validate dynamic timestamps, opaque cursors, coverage and resource links by contract instead
of accepting arbitrary field removal. No generic recursive timestamp stripping is permitted.

## Tests and acceptance

Commands, statuses, compiler/dependency pins, logs and digests are in `evidence/T01`.
The existing synthetic suites cover search/context and resource authorization, large wire pages,
archive routing, send/reply persistence, join authority and history cancellation, HTTP Events
and signed callbacks, service restart, and actual child-process STDIO forwarding.
The added fixtures complement these suites; they do not replace behavioral assertions with
mock business implementations. Initial cache-permission failures are infrastructure failures
and are distinguished from the successful isolated-cache reruns.

Model and live gates: **NOT_RUN**. T01 acceptance concerns offline compatibility evidence only;
it does not claim model quality, rollout acceptance or completion of T02–T10.
