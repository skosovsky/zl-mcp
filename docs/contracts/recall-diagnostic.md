# Owner-only controlled recall diagnostic

This local Unix-socket diagnostic is not an MCP tool or general recall API.
It borrows the current service sender/session, never restores another client.
`probe-recall SEND_REQUEST_UUID` binds to an existing sent operation and its
exact live-collected own message. The only eligible text is:

> zl-mcp: проверка архивного отзыва, сообщение будет отозвано. Отвечать не нужно.

Require the current account as sender, direct conversation, supported exact
message/client IDs, collection permission and a sent result less than 30 minutes
old. Receiver and IDs come from that operation/corpus; the caller cannot supply
arbitrary message IDs or text. This guard does not replace human authorization.

Before dispatch, durably reserve an account/send-UUID-bound private receipt under
state/diagnostic-recalls (0700 directory, 0600 regular file), with at most 20
receipts. One network call, 30-second bound. A reserved/ambiguous receipt never
redispatches, including after restart or a lost response. Known responses return
only a numeric status; `upstream_response` is not proof of collector observation
or archive semantics. Errors never return upstream text/credentials. Receipts
are retained as diagnostic idempotency evidence; no automatic deletion.

The control uses executable input/output schemas; it is absent from public MCP
HTTP and STDIO discovery. It does not edit subscriptions, send ledger or corpus:
actual undo observation follows the ordinary collector/tombstone path.
