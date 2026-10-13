# SDK-01: JSON-RPC envelope identity

The user authorized the immutable remote replacement
`github.com/modelcontextprotocol/go-sdk => github.com/skosovsky/go-sdk v1.8.1-0.20261013081052-4a7b89fe086f`
and official toolsy v0.20.6. No SDK vendor, local production patch, sibling edit or
fork publication is part of this adoption. The original v1.8.0 module requirement
remains; the dependency matrix records the resolved fork origin/checksums.

String IDs retain type and value. Numeric IDs retain their exact mathematical
signed-int64 value, including integral decimal/exponent notation. Numeric `1` and
string `"1"` are different identities. Fractional/outside-int64 IDs are rejected
before application dispatch. The existing SDK decimal parser admits numeric
lexemes up to 128 bytes and saturates exponent parsing at 148 to bound arithmetic;
zero with an extreme exponent remains zero. Root HTTP bodies remain bounded at
256 KiB; the SDK STDIO frame budget remains 16 MiB. No arbitrary-precision runtime
parser or second MCP transport is introduced. Caller-provided already-rounded
float64 IDs cannot be reconstructed; fixtures use raw JSON/json.Number.

Invalid numeric requests produce protocol error `-32600` with id:null. SDK HTTP
uses 400 application/json; STDIO emits the error and continues its existing read
loop. Events uses its existing HTTP 200 RPC-error framing. Events validates numeric
IDs before any admission error can echo an unacceptable numeric value; other
missing/malformed IDs retain the prior metadata-error order. Valid Events IDs use
the same SDK MakeID parser with json.Number. Discovery injects the Events capability
through RawMessage maps, preserving the actual SDK envelope ID and every untouched
field rather than copying the caller ID or decoding it through float64.

HTTP protocol 2026-07-28 cancellation uses the existing SDK
PropagateRequestCancellation option: disconnect cancels only that request's
carrier. Stateless HTTP does not route a separate cancellation POST to a different
request's session; that unsupported notification path is not claimed as tested.
Actual STDIO notification cancellation uses exact numeric RequestID (json.Number)
and the SDK's synchronous invocation lookup. Neighboring requests and a new
invocation reusing a retired ID survive old completion. A test-only SDK overlay
blocks the old response writer while the new invocation is registered; production
SDK bytes remain unchanged. SDK nil loggers use DiscardHandler; global DEBUG and
raw cancellation text are included in privacy fixtures.

The one known erroneous current characterization expectation
rpc-id-numeric-large.json now expects `9007199254740993` rather than `9007199254740992`.
T01/T02 commits and original snapshots are retained, including before-fix evidence;
this is the explicit numeric-ID correction in task§4.4, not reopening earlier tasks.
Business message IDs, schemas, Events contents, native owners and persistence are
unchanged. Lost transport replies never authorize resend: real native ledger tests
retain the domain request UUID across new envelope IDs for both sent and unknown
results, with one synthetic upstream effect and retry_safe=false.

Removing this fork requires an immutable unmodified official release that passes
all ID01-ID07, root/nested tests/race/vet, CGO-free builds, full offline/parity,
privacy and fixed performance budgets. Keep provenance and historical failures;
a version number alone does not close a gate. No deployment/LaunchAgent/live
account/paid model/publication is authorized. Model/live remain NOT_RUN.


Full canonical paired acceptance records the §4.4 exception only for
`numeric-precision` → `facts.sdk-envelope-9007199254740993`: a frozen rounded
baseline value may become the corrected value, with a per-pair correction record.
The current candidate must be exactly `9007199254740993`; a still-rounded candidate
fails. Every other fact, domain ID, string ID, trace projection and effect remains
under full canonical comparison. Historical T09 pure-parity capture tests remain
unchanged and do not represent current SDK acceptance.

User-authorized update 2026-10-13 also accepts the upstream correction for
legacy STDIO requests before initialization: JSON-RPC error code `0` becomes
`-32600` (Invalid Request), with ID/message preserved and no application dispatch.
All other parity requirements remain unchanged. Wire evidence and update checks:
[dependency-update-2026-10-13.md](dependency-update-2026-10-13.md).

The paired/canonical comparisons described above are historical SDK-01 acceptance.
Current permanent verification uses the standalone Go/evaly suite and precise ID
contract tests; frozen binaries are not required. See
[harness maintenance](../harness-maintenance.md).
