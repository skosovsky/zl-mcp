# Build, rollback and live-check handoff

This handoff authorizes no deployment. The accepted T10 SHA is supplied after the
final commit; use that exact SHA, not a moving branch. Keep the original binary
and its recorded build manifest before any separately authorized rollout.

## Clean checkout build

Create a separate checkout of the accepted SHA and use Go 1.27.1 with GOWORK=off.
The pinned remote SDK replacement and existing local third_party/zcago are part of
the checkout; no sibling repositories or go.work overrides are required.

```sh
git clone --no-hardlinks /absolute/path/to/zl-mcp /absolute/path/to/clean-checkout
cd /absolute/path/to/clean-checkout
git checkout --detach <accepted-T10-SHA>
export GOWORK=off
go mod download
make check
CGO_ENABLED=0 go build -trimpath -o /absolute/path/to/new-zl-mcp ./cmd/zl-mcp
```

Verify the actual binary SHA256 and `go version -m` against the chosen build
manifest. Evidence distinguishes the original T01 binary/compiler from the frozen
business baseline rebuilt with the T03 harness/compiler; these are not interchangeable.
Host-bound performance uses the fixed T03 measurement baseline and documented v3
budgets; `make performance-acceptance` is separate from check. Never recalibrate a
budget from a new run without an explicit contract decision.

## State compatibility and return to old binary

There is no DB migration. The native stores, account/session owner, send/join/history
ledgers and Events persistence remain authoritative. Fresh synthetic acceptance
starts the baseline and candidate sequentially on the same prepared state in both
orders (`SDK-01/restart-old-new-4347-v1.log` and `restart-new-old-4347-v1.log`).
This proves the tested fixture/state path, not all live account behavior.

For a separately authorized rollout, first record the installed binary, config and
state paths, ownership/modes and hashes without copying account contents into
migration evidence. Stop the current owner cleanly and wait for its exit before
switching binaries; never run two listeners/workers against one account or DB.
Keep an access-controlled consistent state/config backup under the operator's
existing procedure. Start the accepted candidate only after the old owner exited.

On rollback, stop and wait for the candidate, restore the recorded original binary
and original invocation/config, then start exactly one owner. Preserve the durable
state and operation UUIDs. For ambiguous sends/joins, inspect native operation
status; do not replay effects, create replacement UUIDs or reset ledgers. Do not
restore an old state snapshot over newer accepted operations to force a retry.
No LaunchAgent operation, production stop/start or account action was performed
by this migration. Operator-specific service commands require a separate rollout
instruction and inspection of the actual installed service configuration.

## Separately authorized live gates (currently NOT_RUN)

- HTTP auth/Host/Origin, version discovery and STDIO forwarding with actual client settings.
- Exactly one account/session/DB owner, graceful shutdown and queue/worker recovery.
- Read/search/context coverage, stale/gaps, resource policy and archive TTL/tombstones.
- Events v1/v2 subscriptions, callbacks/signatures, read/ack/retry/cancel/retention.
- Send/join/history authority and durable status recovery; effectful smoke requires
  explicit authorization and a selected test target, never automatic resend.
- Model evaluation with a separately authorized driver/model, token/cost record,
  independent effect evidence and the full model grading contract.

Model and live gates remain NOT_RUN until independently executed and accepted.
