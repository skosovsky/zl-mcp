# MCP archive reading

Status: deployed from clean signed source `3352ae1`; real-account acceptance is incomplete.

Current implementation: authenticated source discovery, archive conversation
catalogue pagination, message browsing and resource retrieval pass synthetic
offline tests, including SDK tool/resource handlers. Known tombstone/expiry
visibility and Unicode truncation have separate tests. The source passed Linux/macOS CI, native startup acceptance and deployment.
Actual source discovery and the catalogue read succeeded without phone sync. The
requested date-window message read returned `SOURCE_CONTROLS_UNCLASSIFIED`; no
message prefix was disclosed. The selected file has 24 rows: the older strict
inspection counts no type-33/36 controls but its sample has one unsupported kind.
The new whole-file control set is broader, and the exact class/semantics still
need investigation. This gate does not prove that the period lacks messages.

`zalo_list_archive_sources` returns at most two authenticated permanent source
descriptors. Counts describe the captured account package, not permission to read
every conversation. Effective retention is separate from original cache expiry.
No filenames, noise IDs, keys, sessions, message bodies or callback details appear.

`zalo_list_conversations` and `zalo_list_conversation_messages` accept optional
flat `source_id`. Omission keeps existing corpus behavior. Archive message reads
require explicit since/until. There is no fallback to another source, corpus or
phone acquisition. Every read validates account, immutable source digest, exact
typed mapping and current collection policy. Offline archive access does not
require a live Zalo connection.

Public cursors and resource tokens are opaque AEAD capabilities, with distinct
cursor/resource AAD and decoder version. They bind account, source UUID/digest,
request filters/order and private paging position. They survive service restart
under the library key, but never waive source removal, permissions or visibility.
Limit may change between pages, including response-budget shortening; filters and
order cannot. Catalogue query membership is frozen as a compact ordinal mask;
metadata names are current local catalogue observations, never historical claims.

Archive records use their own row identity and nullable genuine global ID, with
known or explicitly unavailable authors. Text excerpts contain at most 2048
Unicode characters. Truncation includes a `zalo://archives/<opaque token>` resource
for the complete verified record (maximum 1 MiB source text). Resource retrieval
replays the original bounded source page and rechecks TTL, ownership, policy and
known local tombstones; no plaintext cached in a token or disk index is necessary.
Archive-only records are never send/quote anchors. Known local suppression is
checked by genuine global ID; missing global IDs and unverified WAL changes remain
explicit visibility limitations. Unclassified source controls block the whole selected file. A narrow snapshot-only
exception recognizes type 20 only with exactly one valid attachment whose action
is the pinned native `msginfo.actionlist` literal; every such row in the whole
file must pass metadata classification before any prefix is returned. Classification
reads neither message content nor identities, at most 5,000 rows/8 MiB metadata
under the existing SQLite deadline, with a 256 KiB blob limit per row. Missing,
malformed, oversized, multiple-attachment or different actions fail closed. Other
deferred types, including delete/undo 33/36, remain blocking. This does not change
strict mobile import admission. Classified informational content is never rendered
or executed. `source_information_rows` counts all classified rows in the file;
`unsupported_content.native_information` counts omitted rows examined on this page.
Their metadata gaps are counted for the examined page. Empty informational pages
still advance the cursor. TTL, local tombstones, policy and resource checks remain.

Coverage distinguishes source/period counts and bounds from examined-page rejection,
expiry, unsupported metadata/content, unresolved quote/mention/sender and known
live suppression. Empty results do not prove absence from account history.
Reads create no corpus records, import checkpoints, Events, sends, first-incoming
facts, subscriptions or acknowledgements. SDK transport adapters validate every
output and shorten pages without skipping unreturned records.

## Control diagnosis

Owner inspection now includes optional `unclassified_control_types`: bounded
whole-file counts for the pinned decoder's deferred types 20, 21, 25, 26, 29, 32,
33, 34, 35, 36, 45, 51 and 52. Counts include rows outside the date window and
rows excluded by scalar sampling. No control body, original filename or identity
is rendered. The older `source_controls` field still counts only 33/36 and is
retained unchanged for compatibility. The additional histogram explains a gate;
it does not classify a control, relax visibility or permit import. Synthetic
coverage proves the distinction for an out-of-window control with no global ID.
Diagnostic extension de80416 is installed; the selected real file has exactly
one type-20 row. Native format-1 queries exclude this type; their conversion table
maps it to webchat, while mapping type 36 to undo. Desktop MSG_UNDO=20 therefore
does not classify this backup row. Its exclusion/visibility semantics remain
unverified, and the public guard is unchanged. See
[control taxonomy evidence](archive-control-taxonomy-research.md).
