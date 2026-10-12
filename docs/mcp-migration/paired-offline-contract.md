# Paired offline acceptance

`zl-mcp-eval --baseline-binary /absolute/frozen-binary --profile offline`
interleaves frozen baseline and candidate on the same sealed dataset, seed and
repeat count through evaly.RunPaired. Each side gets fresh independent synthetic
state and separate raw output/effect directories. evaly.Compare uses all
assertions: full matched coverage, quality1 and maximum regression0. Baseline
and candidate must both complete every case/assertion with lifecycle cleanup.
No model is invoked, and model/live remain NOT_RUN. Baseline provenance is the
frozen original business sources with the accepted T03 evaluation-only overlay.
Normal unpaired CLI behavior remains supported. Paired mode is offline-only.
A failing/incomplete baseline cannot yield acceptance or superiority.

Native restart compatibility uses two frozen/candidate service executables on
one temporary synthetic SQLite state, then repeats in the opposite order. The
existing terminal-startup fixture must keep its journal, private snapshot key
and spent UUID while removing terminal staging data. Each process is stopped
and waited before its peer opens the same state; no authenticated session,
installed state or external account is used. `build_eval.py --native` builds both
service executables with the same compiler and disclosed frozen baseline overlay.
`ZL_MCP_ACCEPTANCE_BINARY` and optional `ZL_MCP_RESTART_PEER_BINARY` select their
order for the existing native startup acceptance test.

The paired gate also requires full canonical capture equality for every matched
case/repeat, independently of assertion grades. Inputs, trace order/names/errors,
complete response fields, facts and effect counts are compared. JSON text
content is decoded independently with UseNumber; it is never replaced by
structuredContent. Resource text remains exact. Null, absent fields, native IDs,
fixed fixture timestamps, array order and message content are preserved.
Only explicitly enumerated fixture-generated timestamps and opaque cursors,
operation IDs, receipts and approval tokens may vary. They must satisfy their
format and preserve equality/reuse relationships within each side, including
text/structured projections and subsequent inputs. No blanket UUID/RFC3339 or
field-name redaction is permitted. Harness elapsed time and byte counts are
measurement metadata, outside behavior equality. Changed message content,
resource content, effects or inconsistent dynamic values must fail the gate.

Performance verification fails closed on missing provenance, wrong binary role,
non-finite/negative metrics, invalid counts/settings or incomplete run records.
The measured executable digest must match a successful evaluation build of its
expected baseline/candidate role in the separately hashed build manifest.
The five-run protocol remains unchanged. User-authorized gate revision
2026-10-12-candidate-latency-plus10-v3 fixes absolute p95 limits to read
7.8683 ms, search 7.0726337 ms, context 6.9967799 ms. The user authorized
retrospective calibration from candidate +10% and reuse of existing measurements.
This budget PASS is not proof of original-baseline latency parity; v2 latency FAIL
remains historical. Allocations/op<=2.0 and allocated bytes/op<=1.20 still apply
independently against unchanged frozen T03; zero allocation baseline means zero.
Historical allocation FAIL under the original 1.20 object gate also remains.
Parent-generated capture windows bound runtime timestamps to the actual fixture
execution (one second tolerance for millisecond persistence rounding). The TTL
fixture's message/coverage times use its explicit minus-one-hour offset; archive
cache expiry uses its explicit plus-one-hour offset. Canonical comparison also
preserves temporal order. Each matched pair records both canonical SHA256s;
raw parity-capture files retain the parent window and complete responses.
The isolated evaluation package uses only the additional standard-library
packages encoding/base64 (opaque token syntax) and regexp (UUID/event/token
syntax) for this comparison. No production package or external dependency gains
an evaluation edge; arch-go permits these two exact standard imports only in
internal/evaluation and retains compliance/coverage thresholds of100.
