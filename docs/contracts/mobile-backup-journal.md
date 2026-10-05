# Mobile backup attempt journal candidate

Status: internal SQLite ledger, not a tool or available mobile history source.
Executable request/status shapes are mobile_backup_attempt.input.json and
mobile_backup_attempt.output.json; their presence does not add MCP discovery.
Schema migration 9 adds a separate attempt table. Existing history, send,
subscription and corpus tables are unchanged. A normalized request fixes UUID,
typed conversation, UTC interval, message budget and archive-byte budget.
The archive may contain wider account history, but the intended import scope
remains this exact selection. Fingerprint includes every normalized argument.

Preparing the same UUID/arguments returns the saved attempt; changed arguments
conflict. Bind all reads/transitions to the current account and collection policy.
Only one active attempt per account is allowed, with a 10000-row ledger cap.
Before dispatch, atomically persist the generated public key and transition
prepared→dispatching with revision CAS. This commit must precede HTTP dispatch.
No private key, archive key, URL, raw payload or message text is stored.

Progress states are dispatching, waiting_for_confirmation, request_result_unknown,
mobile_restoring and waiting_for_backup. offer_ready is terminal for the phone
attempt only; it does not mean download/import complete. Other terminal states
are failed, cancelled and interrupted. Reject invalid transitions and stale CAS.
Service startup invokes recovery under the state-directory process lock before
exposing HTTP/CLI endpoints or restoring the collector. Active attempts become interrupted, retaining
request identity and public key; never retry automatically. No key-bearing
offer is reconstructed after restart. New attempts need a new explicit UUID.

This ledger alone performs no network, imports or Events. The internal observer
binds BeforeDispatch and Progress callbacks to committed revision CAS. A stale
observer cannot redispatch; callback errors stop the guarded adapter. Production
request execution wiring, a trusted-local entry point and the full archive pipeline remain
required before live use. Public keys stay private installation metadata
and are excluded from serialized/formatted attempt structures.

A fresh store without account metadata and without attempts needs no recovery.
Missing account metadata with existing attempt rows fails startup rather than
rebinding or silently ignoring orphaned evidence. Recovery itself sends no request.
