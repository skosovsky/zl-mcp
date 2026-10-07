# Mobile candidate to corpus record conversion

Status: internal converter candidate, not a public or accepted phone import.

The installed desktop client's format-1 `convertCrossV2ToCrossV1` preserves
GlbMsgId/CliMsgId/MsgContent/TimeStamp/TTL. Its `crossMsgToNormalMsg` initially uses
MsgContent as text, but replaces text with an attachment for rich-text action rtf.
Thus tag-6 attachments cannot be ignored even for SQLite type 0. Nontext kinds
also use decoded attachments rather than labeling MsgContent as plain text.

Before conversion, the immutable SQLite reader counts deletion/undo source
records (types 33/36) across the entire selected file, independent of timestamp,
status and page position. Preserve `SourceControls` on each bound page. Reject
conversion unless every control is a verified own-direct type-36/status-3
candidate in a committed whole-source prelude, or legacy immutable checkpoint
evidence proves the accepted first-page classification. WAL sources remain
rejected. Received/group, type-33 and other-status controls block the source before
any ordinary record prefix. The prelude scanner independently handles late and
out-of-period controls; this converter handles requested-period row coverage. Converted recall targets remain private and
carry the original source timestamp, not an invented recall time.

Convert verified type-0/webchat candidates without attachments, or with exactly
one decoded attachment whose present action is exactly `rtf`, or one decoded
attachment with absent/explicitly empty action. For absent/empty action retain
MsgContent as plain text and do not label the record rich text. The native
MSG_TEXT branch starts with MsgContent and replaces it only for exact `rtf`;
an absent action is not a missing-message signal. Unknown nested fields remain
coverage limitations, not authorization to interpret their values. The installed
consumer's MSG_TEXT branch uses nonempty attachment title, falling back to
MsgContent when title is empty/absent. Preserve that visible text projection and
mark `attachment_types=["rtf"]`; do not claim preservation of formatting or
interpret params, links or styling. Validate UTF-8 and the 1 MiB text bound before
returning any record; the aggregate text/BinNet bound includes projected text.
Unknown outer tag 6, multiple attachments and other actions remain unsupported.
Attachments are decoded by [their contract](mobile-backup-attachment.md), but
decoding alone does not establish other corpus rendering. Report other recognized content as unsupported; retain their
source-gap counts and keep full import acceptance open. Do not infer attachment
contents or substitute a narrower text-only implementation for the full goal.
Preserve exact string message/client/mapped-sender IDs, original UTC timestamp,
validated direction and original expiry. Source stays unset: only the storage
import policy assigns history. Text is untrusted data, never tool authorization.

Validate the entire bounded page/request/account/interval, scalar and aggregate
payload/expiry consistency before returning any records. Recheck expiry at an
explicit trusted current clock, with no timestamp or sender fallback. Borrowed
page bytes remain unchanged; returned domain records own their scalar metadata.
Keep this message's quote-send metadata separately from its quoted target. Do not
invent a reply target, author/name or complete quote/mention rendering from partial
BinNet. Count unresolved quote/mention metadata explicitly. Fail with no record
prefix on cancellation or malformed/binding-inconsistent candidates.

This operation performs no requests, writes or Events. Global deletion/undo
handling, nontext/rich-text conversion, actual archive/identity compatibility and
real phone source acceptance remain required before public import acceptance.

## Diagnostic inspection without persistence eligibility

`InspectPreparedArchivePage` validates the same complete request/account-bound
page and classifies content without returning records. It reports plain-text
candidates, expired/unsupported content, unresolved quotes/mentions and explicit
persistence gates `unverified_wal_snapshot` / `unverified_source_controls`.
These gates do not suppress whole-page validation: malformed candidates fail
the diagnostic with no counts instead of masquerading as a safety gate. All
temporary constructed records are cleared before returning. Inspection performs
no writes or Events and does not change `ConvertPreparedArchivePage` eligibility.
An empty interval can still have a whole-file persistence gate.

The owner-only archive probe additionally reports optional
`unsupported_content_reasons` and `unsupported_content_kinds` maps. Each map
partitions the `unsupported_content` count across validated, unexpired candidates;
expired, malformed and supported records are excluded. Reason keys are fixed:
`non_text_kind`, `unparsed_attachment`, `multiple_attachments`, and
`unsupported_attachment_action`. Kind keys are restricted to the known
`mobilePayloadKind` classifications. No raw action, attachment title, parameters,
URLs, IDs or message body can become a key. Counts are positive and bounded by
the request's maximum 5000 examined records. Older diagnostics may omit the maps.

These explanations are independent of WAL/control persistence gates and do not
authorize import or establish rendering of the rejected content. The real
October 7 probe predates these fields: its two unsupported records cannot be
retroactively assigned a kind or reason from aggregate counts alone.

The separate `own_recall_candidates` diagnostic counts only validated direct
type-36/status-3 rows whose sender maps to the current account, as specified by
[row preparation](mobile-backup-row-page.md#verified-own-direct-recall-classification).
It preserves exact targets privately and returns no body. Conversion returns
validated recall targets separately from ordinary records; persistence uses the
[atomic checkpoint contract](mobile-history-checkpoint.md#atomic-own-direct-archive-recalls).
This does not invoke live recall, create Events, support arbitrary deletions or
establish complete mobile import. WAL and unclassified whole-file controls remain
explicit persistence gates; public mobile-archive admission remains disabled.

## Plain-text fallback verification: 2026-10-07

Read-only inspection of the installed consumer shared-worker source confirms
that MSG_TEXT retains the initial MsgContent when attachment action is absent or
empty. One decoded attachment is required; unknown nonempty actions and repeated
attachments remain unsupported. This is an explicit plain-text projection only:
unknown nested fields, quote/mention semantics and formatting are not claimed
complete. WAL/control/identity/expiry admission gates are unchanged.

Consumer source SHA-256:
`7a83866c905956cfb122571b2f28cf00a8b61e0f362c40a82010a9947a7d6ff7`.
The 341-byte MSG_TEXT branch SHA-256 is
`904e5869fa6d5991d09fb52be324fe206b89b7a2599d66c3a276c7ffc9f3f8f8`.
The consumer code was inspected as text; no native module or private archive
was executed by that consumer during this verification.
