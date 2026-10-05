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
