# Group cloud history page candidate

Status: the durable import worker and MCP interface are implemented. A bounded live group page was read successfully on the installed candidate; deeper phase traversal and external-client import-tool discovery remain open. See [current acceptance gates](../conversation-completion-open-gates.md).

The candidate follows [upstream PR #370](https://github.com/RFS-ADRENO/zca-js/pull/370), head `4eeceafad031ce4f594c4532e3363dbdf01450b0`, and the installed native client's group-cloud endpoint inventory. It is a group-only read, not a direct-history fallback or full inbox source. No read/seen mutation or message sending is part of the request.

One call uses the current authenticated API session and its advertised `group_cloud_message` base. It sends encrypted GET `/api/cm/getrecentv2` with `nretry=0`: exact group ID, numeric `globalMsgId` cursor (zero initially), `count` in 1–50, empty `msgIds`, the existing IMEI and `src=3`. Decimal cursors preserve every digit without float64 or fixed-width integer conversion; fractional, exponent, signed, structured and overlong cursors are rejected. No guessed service domain is used when the account lacks that source.

The response retains at most the requested count of raw records and nullable continuation/filtering evidence: `hasMore`, exact `lastMsgId`, `isFiltered`, `isFilteredByPhase`, `isFilteredByTimeJoin`, `isOld`, join timestamp and internal error. Missing/null message arrays, oversized pages and malformed continuation types are errors rather than silent exhaustion. Cursor absence is distinct from zero. Raw records remain internal and must be normalized and validated against the requested typed identity before any persistence; a page read itself creates no messages, Events or first-incoming facts.

This optional SDK extension does not broaden the existing public API interface or start another listener. There is no automatic paging, import, retry or complete-history claim in this layer. Upstream filtering, missing continuation or an empty page does not prove historical completeness. The separate bounded import operation defines request identity, progress, cancellation, restart and silent Events semantics.

The [explicit import operation contract](history-import-operation.md) records
those requirements separately, with executable request/status schemas. Silent
historical import was selected on 2026-10-04. The import worker uses this page
port through the existing guarded session; it creates no second listener.

## Domain normalization boundary

The application adapter accepts only an exact typed `group` identity, count in
1–50 and an exact decimal cursor. A direct identity returns an explicit
unsupported-source result without network access. The SDK extension must be
present; it never falls back to the old group endpoint or a guessed direct URL.

Each raw record is decoded without float conversion. Decimal numeric IDs and
millisecond timestamps are converted to their exact string representation for
the pinned message model. Missing identity/time, invalid numeric alternatives,
a different `idTo`, oversized raw/text content or a malformed record rejects the
whole page. The adapter preserves nullable source flags and continuation and
uses the existing account ID to classify direction. Message text remains data.

Normalized messages deliberately have an empty persistence source. The current
Store.Put rejects them. The import journal uses its explicit historical page
transaction to assign `history` provenance and commit without Events, rather than
treating backfill as ordinary replay. This adapter itself has no storage or subscription dependency and starts
no listener. Tests use a fake page reader and demonstrate this boundary.

## Flag normalization and failure classification

Nullable continuation/filtering flags accept JSON booleans and exact integer
literals 0/1 on the source wire, normalizing them to booleans. Missing/null stays
unknown. Strings, other numbers, arrays and objects are rejected. This applies to
hasMore, isOld, isFiltered, isFilteredByPhase and isFilteredByTimeJoin.

The group leaf is decoded separately from the common encrypted envelope. A leaf
format failure is a typed invalid-page error with an allowlisted field reason,
not a fabricated upstream API error code 0. Diagnostics never expose the value
or raw page. Existing local source/history bounds and silent ingestion remain.

## Response budget and phase evidence

The SDK caps both the HTTP wire body and decompressed JSON at 8 MiB before
decoding the encrypted envelope. An oversized or truncated response is an error,
never a successful partial page. Record-count and the adapter's 1 MiB aggregate
normalization budget apply independently after decoding.

Native Zalo 26.9.10.2959 carries both `lastMsgId` and the returned `isOld` into
its next backward request. Its `getCM` routes `isOld=0` to `getrecentv2` and
`isOld=1` to `getoldv2`. The phase-aware source implementation follows this
routing and the durable import contract preserves phase in its status payload.
The legacy SDK method still reads recent pages, without inferring phase from a
nonzero cursor. Missing production continuation phase stops the import rather
than guessing. Synthetic routing and journal tests do not establish live old
page availability. Installation and live phase traversal remain pending.
