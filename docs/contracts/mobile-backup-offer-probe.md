# Owner-only mobile offer probe

`mobile-backup-probe-offer <operation_id> <revision>` calls the existing service's
owner-only Unix socket through `cli_probe_mobile_backup_offer`. This is a diagnostic
phone execution route, not an MCP tool, a download or a message import. Invoke only
after the human authorizes the exact prepared request and is ready to confirm the
Zalo phone prompt. Preparation alone is not dispatch authorization.

Input is the strict executable envelope/arguments schema. Validate ownership,
collection policy, prepared state and the exact last-read revision before calling
the internal current-session offer runner. No session restore or second listener.
The durable observer claims dispatch once; retrying a dispatched/interrupted/terminal
attempt cannot issue another phone request. Timeout is bounded by the existing
180-second source limit. Cancellation/lifecycle/auth loss suppress late results;
read attempt status after an unknown/failed request, never automatically redispatch.

The selected typed conversation/interval constrains later processing; the upstream
phone may create an account-wide archive. The probe neither downloads nor examines
that archive. Return only contract-checked attempt state, an unverified DNS host
parsed from an HTTPS offer (no path/query/userinfo/port), archive size as an exact
decimal string and whether it fits the prepared byte budget. URLs, keys, cookies,
RSA material and message contents are never CLI output or logs. Host observation
is evidence for subsequent investigation, not permission to add it to a downloader
allowlist or forward credentials. The private offer is discarded after reporting.

This route is intentionally one-shot: losing its result requires reading the
ledger and a separate decision about another attempt. It cannot supply a resumable
archive or close production import, format/control or old-message recovery gates.
Existing prepare/status/cancel routes remain nonexecuting; cancellation through
the prepare-cancel command remains limited to prepared attempts.

## Verification

Implemented and installed from signed commit `051b581` on macOS. Root race tests,
vet, CGO-disabled macOS arm64/Linux amd64 builds and the worktree secret scan
passed. Synthetic tests prove revision/one-shot dispatch checks, private metadata
redaction, contract validation and no corpus/Events/send writes. The installed
owner CLI returned `NOT_FOUND` for a nonexistent attempt through the existing
service; its STDIO bridge still listed 21 tools and read connected/authenticated
status. SQLite integrity and selected subscription identity/filter/generation/
activation fields matched the private pre-update snapshot. No real phone request,
archive download or import has been performed for this diagnostic acceptance.

## Failure diagnostics

The service records committed dispatch/progress states with operation ID and fixed
failure categories (timeout, cancellation, authentication, unavailable source,
unknown result, rejection, invalid source event or generic execution failure).
It never logs source error text, offers, keys, URLs, account/conversation IDs or
message content. These logs supplement the durable terminal status; they do not
permit retrying a terminal operation or establish archive compatibility.

The first authorized live probe reached the phone; the user confirmed it. The
attempt ended `failed` without an offer. Its installed version did not preserve
intermediate diagnostic categories, so the exact failure stage cannot be inferred
from its terminal revision. No download or import occurred. A subsequent live
attempt needs a separate decision, not an automatic retry.
