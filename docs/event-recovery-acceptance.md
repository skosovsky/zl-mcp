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

At the initial checkpoint the cloud automation still needed a client-side
instruction update and reconciliation of retained callbacks. That checkpoint
preceded the adoption evidence below.

See [event recovery contract](contracts/event-recovery.md) for the ordered workflow,
retention and the crash/concurrency window. The workaround does not prove or fix
the internal cause of the receiver's missing payload.

## Publication and installed bridge follow-up

[GitHub CI for `07791a0`](https://github.com/skosovsky/zl-mcp/actions/runs/37410266125)
completed successfully on both macOS and Linux, including root/nested test,
race/vet and CGO-disabled cross-builds. Full published main history secret scanning
covered 23 commits with no findings. Source and installed files matched by SHA-256
for both skills (five files each).

The exact installed binary's STDIO bridge initialized a client at protocol
2025-11-25, listed 21 tools including all recovery tools and returned the active
collector status. It did not advertise Events. No second Zalo listener was started.
The preserved active v2 subscription ID, generation and activation boundary match
the verified pre-update SQLite backup; no callback URL or signing key was read
for that comparison.

## Ann adoption and backlog reconciliation

The user relayed Ann's direct acceptance report: all three recovery tools are
available, one matching v2 direct-chat subscription includes incoming and
outgoing messages, and three journal events matched previously completed
notifications by exact message ID. Ann acknowledged them in order without
repeating notifications. Ann also reported updating the saved automation and
verifying it through `automations.list`: read the exact subscription journal
first, process in order, acknowledge only successful handling, and stop at an
explicit gap or ambiguity. The automation is enabled. These client-side actions
are reported evidence, not a local inspection of the cloud automation settings.

An independent authenticated read on this Mac confirmed the three recovery
tools, one active v2 subscription, an empty journal (`has_more=false`) and no
delivery block. Collector is connected/authenticated, `last_error` is absent,
and schema-12 SQLite integrity passed. This verification made no acknowledgements
or subscription changes. Client adoption and initial backlog reconciliation are
therefore recorded as complete. A crash between notification and acknowledgement
still permits a duplicate; exactly-once is not claimed.
