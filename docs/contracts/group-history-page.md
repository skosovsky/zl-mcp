# Group cloud history page candidate

Status: internal protocol contract before implementation; live acceptance pending.

The candidate follows [upstream PR #370](https://github.com/RFS-ADRENO/zca-js/pull/370), head `4eeceafad031ce4f594c4532e3363dbdf01450b0`, and the installed native client's group-cloud endpoint inventory. It is a group-only read, not a direct-history fallback or full inbox source. No read/seen mutation or message sending is part of the request.

One call uses the current authenticated API session and its advertised `group_cloud_message` base. It sends encrypted GET `/api/cm/getrecentv2` with `nretry=0`: exact group ID, numeric `globalMsgId` cursor (zero initially), `count` in 1–50, empty `msgIds`, the existing IMEI and `src=3`. Decimal cursors preserve every digit without float64 or fixed-width integer conversion; fractional, exponent, signed, structured and overlong cursors are rejected. No guessed service domain is used when the account lacks that source.

The response retains at most the requested count of raw records and nullable continuation/filtering evidence: `hasMore`, exact `lastMsgId`, `isFiltered`, `isFilteredByPhase`, `isFilteredByTimeJoin`, `isOld`, join timestamp and internal error. Missing/null message arrays, oversized pages and malformed continuation types are errors rather than silent exhaustion. Cursor absence is distinct from zero. Raw records remain internal and must be normalized and validated against the requested typed identity before any persistence; a page read itself creates no messages, Events or first-incoming facts.

This optional SDK extension does not broaden the existing public API interface or start another listener. There is no automatic paging, import, retry or complete-history claim in this layer. Upstream filtering, missing continuation or an empty page does not prove historical completeness. A later bounded import operation must separately define request identity, progress, cancellation, restart and compatible Events semantics before exposure as an MCP tool.
