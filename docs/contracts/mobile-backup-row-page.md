# Mobile SQLite page preparation candidate

Status: internal mapped source rows, not domain messages or corpus import.

Static format-1 client SQL selects MsgStatus > 0 and excludes types
20,21,25,26,29,32,34,35,45,51,52. convertCrossV2ToCrossV1 maps the remaining known
types through the native client's type-name table, preserves SenderId/GlbMsgId/
CliMsgId/MsgContent/TimeStamp/TTL, and passes parsed BinNet metadata separately.
Timestamp and TTL are milliseconds; TTL is preserved raw. The statically verified
checked expiry calculation is described in [mobile-backup-expiry.md](mobile-backup-expiry.md);
this row preparation does not yet filter expired content or schedule deletion.
Deletion type 33 and undo type 36 require separate state semantics and are not
ordinary imported messages. Content of non-webchat types is not called plain text.

## Verified own-direct recall classification

The controlled paired export on 2026-10-06 demonstrated the same global message
ID changing from type `0` to `36`, with status `3` unchanged, after an own-message
recall in a direct conversation. Classify this precise candidate separately:
direct selection, type `36`, status `3`, valid whole-page scalars/interval, and
sender resolved by the current session mapper to the current account. The mapped
sender must belong to that direct conversation; incomplete or conflicting mapping
fails the whole page. Retain only the exact string global ID in private
`OwnRecallIDs`; do not interpret MsgContent/BinNet as text or a target instruction.
Group, received-message, type-33 and other-status controls remain deferred.

The classification remains included in `DeferredControls` until persistence is
implemented and accepted. The owner archive diagnostic reports only
`own_recall_candidates`, never IDs or bodies. Clear owned IDs with the page.
No archive-control ID is a request to the live recall API. A future silent
tombstone/checkpoint transaction must precede any import of ordinary rows, and
must handle controls outside the requested period. This classifier does not
relax whole-file source-control or WAL gates, and does not prove that either
source is complete. It is the first stage of control support, not full import.

PrepareRowPage accepts at most 50 already selected rows, normalized exact request,
canonical numeric current-account session ID and guarded identity mapper. Validate
all row scalars/UTF-8/size/status and [since,until) before any mapping request. Aggregate text/BinNet is capped at 8 MiB.
Classify excluded/deletion/undo types as deferred controls, unknown types as
unsupported, absent BinNet as missing and malformed BinNet as invalid. Retain
recognized payload kind and bounded partial quote/mention metadata; count unknown
metadata occurrences explicitly. Do not infer attachments or resolve quote targets.

Build one exact direct-ID mapping request for unique retained row sender IDs in
encounter order. Require complete one-to-one canonical same-type pairs with no
extra/duplicate/missing mappings. Never substitute source sender IDs. Mark outgoing
only when mapped sender equals the explicitly supplied current account session ID;
otherwise incoming. Direct rows must map to either the selected peer or the
current account; a third sender fails the whole page. Group rows keep member
identities without inventing membership evidence. Preserve IDs as strings without float conversion. Returned
rows own copied BinNet bytes/parsed metadata; clear private buffers and reset
references on failure or Clear. Result JSON/formatting is redacted. Cancellation
fails the whole result, never a successful prefix. Counters distinguish examined,
prepared, deferred controls, unsupported types and missing/invalid/partial metadata.

This preparation does not authenticate archive/account provenance, prove completeness
or produce domain.Message/Events. Service operation must bind selected file and
account; raw nontext payload and control semantics, sender/quote mapping against a
real archive and durable selected silent import remain open. No writes occur.

Prepared rows retain `ExpiresMS` and `ExpiryDeclared` from the checked message
expiry calculation. Validate expiry overflow across the whole page before any
identity request. These fields do not imply eligibility or a running deletion
scheduler; future persistence must apply both ingestion and subsequent expiry.
