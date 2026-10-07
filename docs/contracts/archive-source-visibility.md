# Native source visibility for read-only archive pages

Status: candidate profile; installation and real-source acceptance must be recorded
separately. Strict corpus import and live tombstones are unchanged.

The pinned format-1 consumer's SQLite message/count queries exclude MsgType 20
before metadata conversion. The converter maps 36 to `chat.undo` and forwards
GlbMsgId/CliMsgId/SenderId unchanged. Its later undo branch can relabel a record
as `chat.delete.everyone`, but retains those same IDs. Evidence is in the native
worker SHA-256 `713c0c4469ba7467ea2c8dc35a07c1319b72566057fdf3091ca73a59d3f58b1c`
and [taxonomy research](archive-control-taxonomy-research.md). No vendor module
is executed. Backup type integers must not be replaced with desktop model enums.

## Whole-source classification

Before emitting any page, inspect all deferred types in the selected immutable
SQLite image, independently of requested dates/order/cursor. Allow at most 5,000
combined deferred rows and the existing 30-second SQLite execution budget.

* Type 20 follows the native query exclusion: its body/action is never rendered
  or executed, even with absent or invalid metadata. A separate bounded metadata
  scan counts the exact known `msginfo.actionlist` subset; it is not the reason
  other type-20 records are excluded. Scan at most 8 MiB total, 256 KiB per blob.
* Type 36 is admitted only with canonical positive sender/global/client IDs,
  valid source timestamp/nonnegative TTL and exact integer status 3. Classify the
  entire set without reading MsgContent/BinNet. Duplicate global or sender/client
  targets, invalid scalars, budget/cancellation/query errors fail the whole file
  without a prefix. Zero global IDs in a control remain unsupported.
* Type 33, other deferred types and other recall statuses remain unclassified.

Build a private file-local recall target set. Suppress all type-36 rows and all
ordinary rows with a matching genuine global ID. If an ordinary row has no global
ID, suppress only an exact (plain sender ID, client ID) match; a different genuine
global ID never matches by client ID alone. Sets never cross typed conversations
or source files. Quotes/mentions remain unresolved metadata, not rendered quoted
content. Source recall times are not inferred from original message timestamps.

This is immutable snapshot visibility, not an account-wide deletion operation.
It writes no live/corpus tombstone, import checkpoint, novelty fact, Event,
subscription, acknowledgement or send. WAL incompleteness and current local
visibility checks remain explicit; no phone request or identity lookup occurs.

## Coverage and continuation

ArchiveCoverage adds optional nonnegative whole-file `source_native_excluded_rows`
(type 20) and `source_recall_rows` (classified type 36), plus `suppressed_source`
for recalled/suppressed rows examined on the current page. Existing
`source_information_rows` is the recognized informational subset of excluded
rows, not an additional disjoint count. Do not sum it with excluded rows.

For examined type-20 rows, existing `unsupported_content.native_information`
counts the recognized subset; `native_excluded` counts other omitted type-20
records. Invalid omitted metadata is visible as `invalid_excluded_metadata`.
Metadata gap counts may overlap omissions. Empty excluded/recalled pages still
advance the genuine examined-row cursor; continuation is required if has_more.
Source/page counts do not establish complete Zalo history or create notifications.

Public responses retain their own copy of projection-gap counters after private
page buffers are cleared. Synthetic owner and actual MCP resource tests cover
whole-source suppression outside the requested interval in direct/group files,
long-text replay and denial of a valid resource claim targeting a recalled row.
