# Durable expiry of silent historical messages

Status: internal storage port; mobile archive import is not yet wired to it.

An expiring historical record is a normalized message plus an exact positive
millisecond deadline derived from its original timestamp and TTL. A zero deadline
means undeclared TTL. Validate all messages and deadlines before mutation; use the
existing silent history insertion and attach expiry in the same transaction.
Deadlines must be after the original timestamp. Never restart TTL at import time.
Already expired newly inserted rows are removed in the same transaction before
commit; insertion counts represent committed identities, not retained content.

Only newly inserted historical content receives a deadline. Preserve an existing
live or historical record and its original deadline on duplicate/conflicting
imports. Deadline metadata is keyed by typed conversation/message identity,
persists across content removal and restart, and prevents both live replay and
historical reimport from restoring expired content. It contains no message text.

Trusted maintenance takes an explicit UTC clock and removes content whose deadline
is at or before it through existing transactional removal, including FTS, quote
metadata and delivery payload cleanup. Keep permanent identities and expiry
metadata. Startup/periodic retention runs expiry even when age retention is off.
No expiry action produces a new Event or sends a Zalo message. Once observed
expired, the durable flag remains set even if the system clock moves backwards.

Read paths use `visible_messages`, filtering the original millisecond deadline
at SQL execution time with integer seconds/fraction conversion, even before
physical maintenance. This includes browse/search/context/resource/message reads,
coverage/counts, directory availability and reply lookup. Existing snapshot and
permanent identity bookkeeping continue to use the underlying table where needed.

This primitive does not prove a complete mobile record converter or quote/control
semantics. Public import acceptance also requires a checkpoint committed with each
page. No public tool exposes clock or deadline manipulation.
