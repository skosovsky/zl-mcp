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
conversion while that count is nonzero: no verified target mapping exists yet.
This prevents an in-window text page from being imported ahead of a later or
out-of-window control. Detection is not implementation of deletion/undo semantics.

Convert only verified type-0/webchat candidates without attachment tag
6. Attachments are decoded by [their contract](mobile-backup-attachment.md), but
decoding alone does not establish corpus rendering. Report other recognized content and rich text as unsupported; retain their
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
atomic page/checkpoint persistence remain required before public import acceptance.

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
