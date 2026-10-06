# Mobile history diagnostics

Status: experimental internal source. The owner-only CLI supports the attempt
ledger, phone-offer probe and selected-archive inspection. Authorized installed
probes passed download, decryption, identity mapping and SQLite inspection.
Durable mobile import is still unavailable; none of these diagnostics is an MCP
tool. An older installed binary may not have these commands. Do not describe
preparation or a successful probe as loading messages into the corpus.

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

After the user authorizes the exact selected conversation and period, an owner
may execute one prepared attempt with its current revision:

```sh
zl-mcp -config /absolute/path/config.toml mobile-backup-probe-archive OPERATION_UUID REVISION
```

The phone may require confirmation. Read status after an observation timeout;
do not redispatch an active attempt. A terminal attempt cannot be reused to make
another phone request. New dispatch requires a fresh UUID and user authorization.
The request selects what is inspected from the account-wide transfer; it does not
ask the phone to produce an archive of only that dialogue. Only the selected
file is read, and private scratch is removed after the diagnostic. No archive
URL, key or message body is returned. Prior scratch cannot be reused to change
the interval after completion.

`source_rows` describes the selected main image; `source_period_rows` counts all
rows in the exact interval before validity/type/TTL filters. Min/max timestamps
do not prove continuous history coverage. `text_candidates` counts validated
plain-text candidates, while `convertible_messages` excludes pages with source
safety gates. `conversion_block_reasons` explicitly identifies unverified WAL
snapshots or source controls. Unsupported content and unresolved quote/mention
counts remain visible even for a blocked page. Malformed candidates fail the
operation instead of being mislabeled a safety gate. `import_performed` is always
false. A WAL marker is not itself proof that the phone lost transactions.

This ledger is not a new history source in the research skill. Continue using
actually discovered history-import tools for group pages or a bounded conversation
preload snapshot. The mobile pipeline still needs verified producer snapshot
semantics, attachment and control semantics, persistent expiry and selected
silent import. Earlier failed transport/format probes are preserved as dated
evidence, not the current status of the inspected source.
Any agreed phone acceptance must specify the exact operation and avoid exposing
archive keys or message bodies. Historical imports remain without Events.

See [local ledger contract](contracts/mobile-backup-local-ledger.md) and
[current acceptance gates](conversation-completion-open-gates.md).
