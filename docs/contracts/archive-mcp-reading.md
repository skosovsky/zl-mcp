# MCP archive reading

Status: deployed from clean signed source `8032172`; all 56 retained files pass
complete bounded local and actual-client pagination. Complete Zalo history and strict corpus import
remain unproven. See [current acceptance](../conversation-completion-open-gates.md).

Authenticated inventory, catalogue/message pagination, ownership, policy, TTL,
resource replay and recall suppression pass synthetic offline tests through the
actual MCP SDK. The read-only native visibility profile removes the former nine
whole-file control refusals in this retained source. Reads dispatch no phone sync.

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
explicit visibility limitations. Unclassified source controls block the whole selected file.
The [native source-visibility profile](archive-source-visibility.md) is read-only
and installed: type 20 follows the native query exclusion; exact informational actions
are a separately counted subset. Validated type-36/status-3 targets suppress
recalled rows and matching original copies inside the selected immutable file,
including controls outside the requested interval. Other controls, malformed
identity/state, duplicates and exhausted whole-source budgets fail without a prefix.
This neither applies corpus tombstones nor changes strict mobile-import admission.

`source_native_excluded_rows`, `source_information_rows` and `source_recall_rows`
are whole-file counts; informational rows are a subset of excluded rows.
`unsupported_content` and `suppressed_source` describe the examined page. Metadata
gap counts can overlap exclusions. Empty excluded/recalled pages still advance
continuation. TTL, live tombstones, collection policy and resource checks remain.
All 56 files were paged to termination: 75 pages, 1,637 examined rows and 1,124
text records. Coverage counters survive private-page cleanup and exactly reconcile
projected records, omissions, recalls and expiry. This accepts the captured source,
not account history completeness, unknown control classes or strict import.

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
The diagnostic extension de80416 established one type-20 row in the initially
selected real file. Subsequent whole-source inspection found 39 type-20 and 20
validated type-36/status-3 rows across the 56-file source. The pinned native
format-1 query excludes 20; its converter forwards the recall target identities
for 36. These verified rules are now installed for immutable read-only pages.
Desktop MSG_UNDO=20 is not the backup-row classifier. Other classes/statuses and
invalid target identities still fail without a prefix. See
[control taxonomy evidence](archive-control-taxonomy-research.md).
