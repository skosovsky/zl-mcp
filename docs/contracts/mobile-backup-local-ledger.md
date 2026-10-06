# Trusted local mobile attempt ledger

Status: owner-only Unix control routes. Prepare/status/cancel are nonexecuting; the separately authorized [offer probe](mobile-backup-offer-probe.md) dispatches one diagnostic phone request.

`cli_prepare_mobile_backup` validates the executable mobile request contract and
prepares a durable attempt against the current stored account/collection policy.
No current listener is required for preparation; dispatch must separately acquire
the actual current guarded session. Same request UUID and exact normalized args
return the original attempt; changed args conflict. One active attempt per account.

`cli_mobile_backup_status` reads an exact operation UUID with account and policy
checks, including while the collector is disconnected.
`cli_cancel_prepared_mobile_backup` cancels only a prepared attempt using the
explicit last-read revision. A concurrent dispatch/CAS change rejects cancellation;
this route never pretends to cancel phone/network work. An already cancelled
attempt may return its current state only with the current revision.

All routes accept only POST /rpc and executable strict envelope schemas, maximum
16 KiB, no duplicate JSON object keys or trailing values. Return a contract-checked
attempt without public/private RSA material, archive URL or key, texts or session
values. Failures use fixed domain errors, not raw SQLite/SDK details. Routes are
not MCP tools and cannot be invoked via tools/call. They use the service's existing
owner-only control socket and store, never open another database or listener.

CLI: `mobile-backup-prepare` reads the request JSON from stdin;
`mobile-backup-status <operation_id>` and
`mobile-backup-cancel <operation_id> <revision>` return the ledger status. They
require an already running service and do not create runtime directories. Preparing
an attempt is not authorization to dispatch it. This scaffolding does not close
mobile archive compatibility or deeper-history acceptance and must not appear in
agent skills as an available history source.
