# Mobile backup message expiry

Status: statically verified client calculation and internal integer helper. Not yet
an import, deletion scheduler or acceptance test against a real mobile archive.

## Evidence

Read-only analysis of the installed Zalo desktop client 26.9.10.2959:

- Shared-worker format-1 conversion preserves SQLite `TimeStamp` and `TTL`. Its
  import adapter passes these as `ts` and `ttl` to `/Bne`.
- `/Bne.transformMessageFromServer` retains `ttl` unchanged and assigns the server
  timestamp; it does not convert TTL to seconds or restart it at import time.
- `GSaP` delegates `deriveTTLItems` to `Xu5j`. `Xu5j` derives message expiry as
  `beginTime + ttl`. `wlxX` chooses `serverTime`, then `sendDttm`, then the client
  clock. `bUXd.getTimeNow` returns `Date.now()` plus a server clock offset when
  available. These values, and therefore TTL, are milliseconds.
- `s2Ee` schedules expiry on a rounded timer. A separate message-filter path uses
  an additional configurable overtime margin. Timer rounding and that margin are
  not part of the expiry timestamp and are not reproduced here.
- Quote expiry is separately derived from quote `ts + ttl`; it is not permission
  to delete the containing message. Quote normalization and expired quote content
  handling remain open.

Evidence is version-specific static source inspection, not a documented upstream
protocol guarantee. No Zalo database or private conversation was read.

## Internal calculation

`MessageExpiryMS(serverTimestampMS, ttlMS)` requires a positive archive server
 timestamp and nonnegative TTL, both exact signed 64-bit integers. Zero TTL returns
`declared=false`. Positive TTL returns the checked sum and `declared=true`.
Overflow or missing/invalid timestamp returns a fixed error with no result.
Do not use floating point or substitute the current import time for an absent
archive timestamp. The caller must retain account/source provenance and decide
expiry eligibility using an explicitly supplied trustworthy clock. This helper
neither filters rows nor writes messages or Events.

Historical messages must remain silent. A future importer must avoid restoring
expired content and handle future expiry of imported content, rather than merely
checking the clock once at ingestion. Deletion/undo controls and quote expiry
must have separate semantics before historical persistence is accepted.

## Candidate-page expiry policy

`PreparedRowPage.ApplyExpiry(nowMS)` requires a caller-supplied positive epoch
millisecond clock. Validate all row and quote expiries before changing the page.
A declared expiry at or before `nowMS` makes a message ineligible: clear its owned
payload and remove the candidate. An expired quote clears the quote's message,
attachment and display-name bytes, retaining only scalar identity/type/expiry and
status metadata; it does not remove the containing message. Clear and discard the
owned raw BinNet copy as it also contains the expired quoted payload. Mark the
candidate `QuoteExpired`, and count only the first transition on repeated checks. Missing/zero quote TTL
is undeclared; positive quote TTL requires a positive timestamp and checked sum.
Negative/overflowing quote expiry fails the whole check before mutation. Report
separate expired message/quote counts, preserve examined and source-gap counters.

The logical eligibility boundary is exact; it intentionally does not replicate
Zalo's delayed/rounded physical cleanup. This mutates private candidates only,
creates no Events and is not ongoing corpus deletion. The service must recheck
eligibility at commit and schedule future expiry when durable import is added.

## Durable storage candidate (2026-10-05)

[Silent historical expiry storage](history-message-expiry.md) now provides atomic
message/deadline insertion, read-time visibility filtering and maintenance cleanup
with permanent expiry tombstones. This is not yet called by the mobile record
converter/import journal. The local OS UTC clock is used for visibility and normal
maintenance; server-clock drift/provenance and quote/control expiry remain separate
acceptance gates. No real archive was imported and no running service was changed.

## Atomic operation-page persistence

The trusted storage port `CommitExpiringHistoryOperationPage` accepts an existing
running history operation ID/revision, source continuation evidence without
`Messages`, and at most 50 `domain.ExpiringHistoryRecord` values. It derives the
page messages from those records, rejecting ambiguous dual input. Validate all
records/deadlines, including out-of-interval records, before any page writes.
Apply the existing operation ownership, collection, source, interval, budget,
phase/cursor and cancellation checks; do not invent another source or operation.

Commit silent messages, permanent identities/novelty evidence, original expiry
markers, source progress/continuation and operation revision in one transaction.
Checkpoint failure rolls back all page/TTL writes. Stale or cancelled workers
cannot persist. Retained duplicates preserve their original message and expiry;
page replay cannot extend lifetime. Already due deadlines are applied inside the
same transaction. Inserted count is cumulative ingestion, not available-message
count; imported bounds do not guarantee visibility or uninterrupted coverage.

This internal port extends the existing history-operation transaction, not its
public source enum. It does not dispatch, download, own an archive, serialize a
mobile cursor, publish a new MCP capability or establish archive compatibility.
Mobile integration must additionally provide its exact retained-source checkpoint
and content/control semantics; existing source restrictions remain authoritative.
