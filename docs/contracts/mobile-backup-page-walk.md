# Scoped selected archive traversal

Status: internal streaming composition; no public dispatch or durable import yet.

Traverse one request/account-bound selected archive without another download or
session. Validate an absolute scratch path, numeric account, positive expiry
clock, page size 1–50, page limit 1–100 and callback before reading. Bound traversal
to 120 seconds. Each page uses `ReadPreparedArchivePage`, including its source-row
budget and digest-bound keyset cursor. Expired/rejected-only pages still advance.

The callback borrows a page only until return. It must not retain buffers or a
cursor; the walker clears both on every success/failure. Copy the continuation
before callback delivery and never derive progression from mutable callback data.
The caller owns any side effects; this primitive alone makes no corpus/Events
writes. Callback failure or cancellation returns a fixed error and no summary.
Future durable import must commit each page and its checkpoint together.

Return aggregate examined/rejected/expired rows, retained candidates and explicit
metadata/control/type gaps. Source exhaustion means only this selected archive
and interval, not complete Zalo history. Preserve source has-more on page/message
budget stops. Closed stop reasons: `available_archive_exhausted`, `message_limit`,
`page_limit`. Never treat dropped or unsupported rows as complete recovery.
