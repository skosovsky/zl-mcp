# Archive snapshot pages

Status: bounded public MCP reading is deployed and accepted through the connected
client. The [source-visibility profile](archive-source-visibility.md) is installed and all
56 captured files pass bounded local and actual-client pagination.
This is a separate read-only source, not a relaxation of corpus import admission.

The explicit record view is defined by
`archive_snapshot_record.output.json`. Private rows and pages serialize without
bodies; only an explicit transport projection exposes the declared fields.
Authenticated retained reads carry source provenance into the projection layer;
an unbound acquisition value or changed source UUID cannot label a page.

Read one authenticated, exactly mapped conversation file with an explicit
RFC3339 interval, order (`asc`/`desc`) and at most 50 examined rows. Stable keyset
paging uses source file digest, exact filename, millisecond interval, order,
timestamp and SQLite rowid. A cursor for a different source/window/order fails.
Rejected-only pages still advance. The public cursor additionally
authenticate account, source UUID, conversation and decoder version; private
SQLite cursor fields are never a public or unsigned cursor.

Rows retain original source rowid separately from the upstream global message ID.
Integer zero or exactly textual `"0"` in GlbMsgId means unavailable global ID for
snapshot reading. All remaining scalar, metadata, timestamp, TTL, type and status
checks remain applicable. Noncanonical, negative, overflow, missing or malformed
global IDs remain rejected. No fabricated upstream ID is used, even transiently.
Strict import readers continue rejecting zero IDs, unchanged.

Snapshot IDs use a distinct source/file/row namespace. Available positive global
IDs are reported separately; no archive-only record is eligible as a send/quote
anchor. Neither a snapshot page nor a cursor may be passed to corpus persistence.

Before returning any rows, scan the entire selected file for all deferred control
types recognized by the pinned decoder, including controls outside the requested
period. Until their semantics are classified, a nonzero count returns
`SOURCE_CONTROLS_UNCLASSIFIED`, without an accepted prefix or cursor. Unknown
content remains unsupported and cannot be rendered as arbitrary source text.

WAL-mode main images may be inspected as explicitly incomplete snapshots. They
do not prove checkpoint completion, current message visibility or absence of
later recalls. Coverage reports source/period row counts, bounds, invalid
timestamps, examined/rejected rows and `has_more`, with history completeness
false. Strict corpus-import WAL admission remains unchanged.

Empty projections remain explicit unsupported counts, rather than fabricated
visible-text records. Unknown metadata fields and unresolved quote/mention counts
remain visible in page coverage; no quote or mention context is invented.

Visible text projection uses the verified existing MSG_TEXT rules; TTL and known
live tombstones must suppress applicable records at each public read. Sender IDs
require stored verified direct identity mappings; unresolved senders are explicit,
and never trigger hidden network lookup. Public read/cursor/resource ownership,
collection policy, response limits and resource retrieval must be implemented
before these pages are advertised through MCP. Reads write no corpus, Events,
first-incoming facts, send state or acknowledgements and dispatch no acquisition.

Implemented in source: ascending/descending SQLite pages, stable physical row
identity, strict/snapshot cursor separation, zero-ID handling, whole-file deferred
control rejection, existing visible-text rules, TTL suppression and stored sender
mapping. Tests cover mixed integer/text zero values, tied timestamps, rejected-only
pages, changed source/window/order, every recognized deferred control outside the
window, independent direct/group namespaces, missing/invalid metadata, nontext
payloads, unavailable sender mapping and the executable explicit-record schema.

Public source inventory, authenticated cursors/resources, collection policy,
live tombstone checks, bounded rendering, MCP dispatch, both skills and connected
client acceptance are recorded in [the current gate report](../conversation-completion-open-gates.md).
This does not establish complete source content or strict mobile-import eligibility.
The new native-exclusion/recall profile requires its own real-source acceptance.
