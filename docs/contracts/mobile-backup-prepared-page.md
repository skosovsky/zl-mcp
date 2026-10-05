# Selected mobile archive page processing

Status: internal candidate composition, no CLI/MCP route or durable corpus import.

`ReadPreparedArchivePage` reads one already selected archive with an exact normalized
mobile request, canonical current-account session ID, caller-supplied expiry clock,
guarded mapper and private scratch directory. A selected archive carries private
conversation/request binding from `FetchSelectedArchive`; reject a different
request, conversation or missing binding before SQLite or source mapping.

Page size is 1–50 and no more than the remaining request `max_messages`. The budget
counts all examined source rows, including rejected, expired and deferred rows,
not just retained candidates. Compose bounded immutable SQLite reading, whole-page
sender mapping and explicit expiry eligibility. Preserve separate source rejected,
metadata/control/type gaps and expiry counts. Cancellation or any stage error
clears owned candidate buffers and returns no page/cursor prefix.

The private continuation binds request UUID/fingerprint, account, exact file digest,
filename, date filter and keyset position, and tracks examined rows across pages.
It is not an externally authenticated token. A changed account/request/filter/file
fails before mapper access. Source has-more remains true when a request budget
stops traversal; report budget exhaustion and do not issue a continuation that
would exceed the cap. Terminal pages have no cursor. Rejected/expired-only pages
still advance using the SQLite keyset rather than the retained row count.

The archive is borrowed and unchanged. Returned candidates carry private request/fingerprint/account binding for the
[next conversion stage](mobile-backup-message-conversion.md). Returned candidates and private continuation
are separately owned and redacted in JSON/formatting. Clear resets candidates and
continuation. No download, phone request, second session, messages write or Events
occurs in this operation. Account authentication, real archive compatibility,
attachment/control semantics and persistent expiry remain required acceptance
gates; counts do not establish complete conversation history.

The reader also returns whole-selected-file `SourceControls` for types 33/36,
without interval/status/page filtering. A private page preserves that count for
conversion preflight. Diagnostic paging can still inspect candidates and counts;
conversion cannot treat paging-local absence of controls as compatibility.
