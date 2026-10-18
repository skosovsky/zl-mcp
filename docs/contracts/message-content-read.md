# Bounded message text reading

`zalo_read_message_content` is an additive tools-only alternative to existing
text resources. It reads retained corpus text by an exact typed message URI from
context/browse, the legacy `zalo://groups/{group_id}/messages/{message_id}` URI
issued by group context or Events, and saved archives only by their server-issued `zalo://archives/{token}`
URI. `archive_row_id` is never accepted as a Zalo message ID. Existing resources
remain unchanged. No media bytes, arbitrary URLs or upstream writes are involved.

Input: required `resource_uri` (1–6000 characters), optional opaque `cursor`
(max12000), optional integer `limit` (1–8192 Unicode scalar values, default2048).
Do not construct the archive token. `/content` metadata URIs are not text targets.

Output: resource_uri, source (`corpus`/`archive`), text, text_kind
(`author`/`none`/`legacy_projection` for corpus; `plain`/`rich` for archive),
text_provenance (`retained_message`/`saved_archive_projection`), zero-based scalar
`offset`, scalar `length`, `total_characters`, has_more, nullable next_cursor,
available=true. `none` deliberately exposes empty author text rather than
attachment metadata as words of the author. Legacy projection is identified,
not inferred. Legacy group URIs use the original legacy/plain text projection,
including retained compatibility text that typed semantic reads may suppress;
their text_kind is always legacy_projection. They remain in the group namespace
and cannot resolve a colliding direct-chat ID. Archive projection provenance does not claim original rich bytes.

Each structured JSON page is at most48KiB (including escaping and continuation).
The server also budgets its duplicated TextContent representation, including its
additional JSON escaping, and reserves1024bytes for the tool envelope/SDK metadata.
Their combined serialized size must remain within64KiB; high-escape text or a long
resource URI/cursor therefore reduces the effective character count.
The effective page may be shorter than limit. Follow next_cursor until has_more=false;
empty retained text returns offset=length=total_characters=0, next_cursor=null.

Continuation is HMAC-authenticated and bound to principal, account, policy revision,
URI, full text SHA256, and next scalar offset. Deadline is15minutes after the first
page, never extended. Service restart invalidates cursors. It is not a stored copy
or a lease on access: every page rereads via existing owners, rechecking current
policy, retention, recall and archive capability. Changed text requires restarting.
Underlying archive capability can expire earlier. Cursor contents are opaque to clients.

Error details.reason: invalid_target, invalid_limit_or_target, invalid_cursor,
expired_cursor, content_changed (`INVALID_ARGUMENT`); access_denied
(`PERMISSION_DENIED`); unavailable (`NOT_FOUND`, indistinguishable missing/expired/
recalled source); unsupported_projection (`SOURCE_UNSUPPORTED`); oversized_projection
(`RESPONSE_TOO_LARGE`). Only a known NOT_FOUND owner result becomes unavailable;
a transient owner failure is never reported as a missing message. Sanitized
STORAGE_ERROR reasons read_failed (retryable), read_cancelled (not retryable),
and read_deadline (retryable) distinguish infrastructure and context outcomes.
Known COLLECTOR_UNAVAILABLE and RATE_LIMITED retain their code/retryability
with read_unavailable; SOURCE_UNAVAILABLE uses source_unavailable with retryable=true
because the owner intentionally cannot distinguish expired access from temporary
archive unavailability. SOURCE_UNSUPPORTED / SOURCE_CONTROLS_UNCLASSIFIED retain
their code with unsupported_projection. Errors never include target IDs, tokens, source rows or raw
underlying errors. Existing generic schema validation remains INVALID_ARGUMENT.

Example first page: `{"resource_uri":"zalo://conversations/direct/chat/messages/message","limit":1024}`.
A result has `offset:0,length:1024,has_more:true,next_cursor:"<opaque>"`.
Next call repeats the exact URI and returned cursor. It may adjust limit without
changing the target. A denied page has code PERMISSION_DENIED and details.reason
access_denied; no partial text is returned.
