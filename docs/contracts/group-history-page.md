# Group cloud history page candidate

Status: SDK page candidate published at revision 6a17fad; domain normalization implemented in the worktree. Installed/live acceptance and explicit import remain pending.

The candidate follows [upstream PR #370](https://github.com/RFS-ADRENO/zca-js/pull/370), head `4eeceafad031ce4f594c4532e3363dbdf01450b0`, and the installed native client's group-cloud endpoint inventory. It is a group-only read, not a direct-history fallback or full inbox source. No read/seen mutation or message sending is part of the request.

One call uses the current authenticated API session and its advertised `group_cloud_message` base. It sends encrypted GET `/api/cm/getrecentv2` with `nretry=0`: exact group ID, numeric `globalMsgId` cursor (zero initially), `count` in 1–50, empty `msgIds`, the existing IMEI and `src=3`. Decimal cursors preserve every digit without float64 or fixed-width integer conversion; fractional, exponent, signed, structured and overlong cursors are rejected. No guessed service domain is used when the account lacks that source.

The response retains at most the requested count of raw records and nullable continuation/filtering evidence: `hasMore`, exact `lastMsgId`, `isFiltered`, `isFilteredByPhase`, `isFilteredByTimeJoin`, `isOld`, join timestamp and internal error. Missing/null message arrays, oversized pages and malformed continuation types are errors rather than silent exhaustion. Cursor absence is distinct from zero. Raw records remain internal and must be normalized and validated against the requested typed identity before any persistence; a page read itself creates no messages, Events or first-incoming facts.

This optional SDK extension does not broaden the existing public API interface or start another listener. There is no automatic paging, import, retry or complete-history claim in this layer. Upstream filtering, missing continuation or an empty page does not prove historical completeness. A later bounded import operation must separately define request identity, progress, cancellation, restart and compatible Events semantics before exposure as an MCP tool.

The [explicit import operation draft](history-import-operation.md) records those
requirements separately. It is not an executable contract or exposed capability;
the historical notification decision remains pending.

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
Store.Put rejects them, so a later import operation must explicitly select its
separate ingestion policy rather than accidentally treating backfill as ordinary
replay. This adapter itself has no storage or subscription dependency and starts
no listener. Tests use a fake page reader and demonstrate this boundary.
