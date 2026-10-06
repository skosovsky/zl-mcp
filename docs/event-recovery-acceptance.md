# Event recovery acceptance — 2026-10-06

Implementation: commit `19e2105`.

- Root `go test -race ./...` and `go vet ./...` passed. Targeted race checks after
  the final ordered-retention change passed for storage; Events and MCP transport
  race checks passed after removing v1.
- CGO-disabled macOS arm64 and Linux amd64 builds passed.
- Independent signed TLS receiver plus MCP HTTP tools verified subscription,
  callback acceptance, exact envelope recovery, acknowledgements and cancellation.
- Storage tests verified read-without-ack, ordered completion, tampered/foreign
  receipts, retry idempotency, restart continuity, generation changes, deletion
  and retention gaps. Retention cannot skip an earlier retained record.
- Secret scan and whitespace checks passed. No new model evals were run.

Installed on the existing Mac service with private binary/SQLite/skill backups.
Authenticated MCP reads confirmed schema 12, SQLite quick_check=ok,
collector connected/authenticated without last_error, three recovery tools, and
only legacy group and conversation v2 in events/list. The existing v2 subscription
remained active and its journal returned a retained payload. No production
processing acknowledgements, new synthetic callbacks or Zalo messages were sent
for this acceptance. The repository and local Events skill were updated.

The cloud automation's saved instructions require a separate client-side update.
Existing retained callbacks have no processing checkpoints; reconcile them with
completed notifications before using recovery on that subscription. Notification
handling through the updated automation has not yet been verified live.

See [event recovery contract](contracts/event-recovery.md) for the ordered workflow,
retention and the crash/concurrency window. The workaround does not prove or fix
the internal cause of the receiver's missing payload.
