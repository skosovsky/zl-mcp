# Bounded mobile SQLite inspection candidate

Status: offline internal reader; not a message importer or exposed history source.
Input is one caller-selected opaque file from a validated archive, with account
and typed conversation mapping still required before any corpus mutation.

Require a standalone SQLite 3 file within 256 MiB: correct 100-byte header,
power-of-two page size, exact page alignment, legacy read/write versions 1,
fixed payload fractions, zero reserved header expansion bytes and consistent
valid in-header page count. WAL-format inputs are rejected until an independently
verified export/checkpoint contract establishes their completeness.

Copy only this selected file to a random private 0700 scratch directory under a
trusted absolute caller-supplied location, using a fixed filename with 0600 permissions.
Open immutable mode=ro with one connection; query_only, trusted_schema=OFF,
temp_store=MEMORY, bounded cache and connection limits. Cleanup occurs on normal
return; crash cleanup must be handled by the eventual operation layer. No archive
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
