# Mobile history diagnostics

Status: experimental internal source. A local attempt ledger is implemented in the
current source; phone dispatch, encrypted archive acceptance and durable import
are not available as CLI/MCP capabilities. An older installed binary may not have
these commands. Do not describe preparing an operation as loading history.

Use the current service and its existing account. The commands below connect to
its owner-only Unix socket; they do not restore a session, start another listener
or open the corpus database independently. Preparation can work while the
collector is disconnected if its stored account is known. It grants no permission
to execute a phone operation later.

Prepare a private request JSON file matching
[the executable selection contract](contracts/mobile_backup_attempt.input.json):
stable request UUID, exact typed conversation ID, RFC3339 `[since, until)` and
optional bounded message/archive budgets. Omit optional budgets to use defaults;
explicit zero is invalid in the input schema. Reuse the same UUID and exact args
for retries. Do not include session values, credentials, URLs or archive keys.

With an already running service and absolute configuration/request-file paths:

```sh
zl-mcp -config /absolute/path/config.toml mobile-backup-prepare < /absolute/path/private-request.json
zl-mcp -config /absolute/path/config.toml mobile-backup-status OPERATION_UUID
zl-mcp -config /absolute/path/config.toml mobile-backup-cancel OPERATION_UUID REVISION
```

The response contains the normalized request, operation UUID, state, revision and
creation/update times. Preparation writes only the attempt ledger. Public RSA
correlation material and private transport values are excluded. Treat response
IDs as private operational metadata; do not publish a real request/response.

Cancel only a `prepared` attempt using its last-read revision. If cancellation
races with dispatch, it fails instead of claiming phone work stopped. An already
cancelled attempt can return its current state with its current revision. No
network cancellation is attempted. Read status after a lost response; never make
a fresh phone attempt automatically.

This ledger is not a new history source in the research skill. Continue using
actually discovered history-import tools for group pages or a bounded conversation
preload snapshot. The mobile pipeline still needs trusted local dispatch,
independently verified archive hosts and current-session host-scoped cookie
transport, real format/account/identity evidence,
attachment and control semantics, persistent expiry and selected silent import.
Any agreed phone acceptance must specify the exact operation and avoid exposing
archive keys or message bodies. Historical imports remain without Events.

See [local ledger contract](contracts/mobile-backup-local-ledger.md) and
[current acceptance gates](conversation-completion-open-gates.md).
