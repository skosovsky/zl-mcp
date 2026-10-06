# Bounded mobile SQLite inspection candidate

Status: internal bounded reader used by the service mobile-history pipeline and
diagnostic probes. Public mobile source selection remains disabled pending real
source acceptance. Input is one caller-selected opaque file from a validated
archive; authenticated account and typed conversation mapping are required before
conversion and the atomic history journal can mutate the corpus.

Require a standalone SQLite 3 file within 256 MiB: correct 100-byte header,
power-of-two page size, exact page alignment, read/write pairs 1/1 or 2/2,
fixed payload fractions, zero reserved header expansion bytes and consistent
valid in-header page count. An immutable private copy may be inspected with the
WAL marker 2/2; mixed and unknown version pairs remain rejected. This reads only
the supplied main-file snapshot. Missing WAL transactions and producer checkpoint
completeness are not inferred from integrity_check. WAL snapshots are explicitly
marked and remain ineligible for conversion/persistence until export/checkpoint
semantics are accepted; diagnostic inspection does not claim complete history.

An independent SQLite-generated regression keeps the producer connection open,
checkpoints one message, then commits a second message and a type-33 control only
to WAL. The copied main image passes our integrity/schema checks and reports one
message and zero controls; the producer view contains three rows and one control.
Thus successful main-image inspection cannot by itself justify message/control
completeness. This demonstrates the general failure mode, not a finding that the
actual phone export omitted those transactions. The complementary checkpointed
WAL regression demonstrates that 2/2 also occurs on a usable complete main image.

Copy only this selected file to a random private 0700 scratch directory under a
trusted absolute caller-supplied location, using a fixed filename with 0600 permissions.
Open immutable mode=ro with one connection; query_only, trusted_schema=OFF,
temp_store=MEMORY, bounded cache and connection limits. Cleanup occurs on normal
return; durable encrypted snapshots are owned by the operation layer, with
account/binding-checked terminal cleanup at service startup and worker boundaries
(see [snapshot lifecycle](mobile-backup-snapshot.md)). No archive
filename becomes a filesystem path. Use a 30-second context bound and integrity
check before querying a real, nonvirtual ChatContent table without generated
columns. Require the observed nine fields, and integer timestamp affinity.

Read one bounded batch for [since,until), ordered by timestamp/rowid, at most
5000 rows plus one has_more probe. Return opaque candidate rows, examined/rejected
counts and has_more. Preserve positive canonical uint64 sender/global/client IDs
without floats. Reject invalid timestamp/status/type/TTL, text over 1 MiB or BinNet
over 256 KiB; report rejected rows rather than fabricate IDs or text. Retained
text plus BinNet per batch is capped at 8 MiB; overflow fails the entire batch
without a cursor, and callers must request a smaller row batch. BinNet is
not decoded here, so no successful inspection establishes full message semantics.
No silent import, Event, subscription or corpus mutation is performed.

References: [SQLite file format](https://www.sqlite.org/fileformat.html),
[immutable/read-only URI](https://www.sqlite.org/uri.html),
[trusted_schema](https://www.sqlite.org/pragma.html#pragma_trusted_schema).

ReadSQLitePage adds private keyset continuation: SHA-256 of exact source bytes,
canonical filename, normalized millisecond period and last examined (TimeStamp,
rowid). Reject a cursor for different bytes/name/period or an out-of-period anchor.
The next query uses a strict lexicographic timestamp/rowid predicate, so timestamp
ties and rejected content rows still advance exactly. A real schema column named
rowid/_rowid_/oid is rejected to prevent shadowing the internal key. Invalid
paging-key types fail the entire batch rather than inventing a cursor. Next is
present only with has_more; terminal pages return no continuation. Changing page
size is allowed without changing the bound filter. Cursor is private internal
state, not a public authenticated MCP cursor or an account-binding substitute.

### Fractional-millisecond date bounds

An RFC3339Nano `[since,until)` interval is mapped onto integer source timestamps
as `[ceil(since_ms),ceil(until_ms))`. Round both bounds upward with integer/time
arithmetic; do not truncate or convert to floating point. For example, [0.5 ms,
1.5 ms) contains only the row at 1 ms. A valid nonempty temporal interval can
contain no integer ticks. Reject unrepresentable or overflowing millisecond bounds.
Bind the keyset cursor to these effective source bounds; the outer prepared cursor
also retains the exact normalized request fingerprint. Row preparation and final
conversion validate the same interval, before any mapper or storage work.

Selected-image coverage inspection returns total main-file row count, count inside
the requested interval, invalid timestamp count and the valid minimum/maximum
integer timestamp as UTC. The installed native format-1 reader compares TimeStamp
directly with Date.now() and assigns it to message ts: milliseconds, without a
seconds conversion. No unit guessing or window expansion is allowed. Counts
include source rows before message validity/status/TTL filters and do not establish
checkpoint completeness or history outside the supplied main image. Only the
selected file is inspected; message text and identity values remain private.
