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

## Current deployment checkpoint

The last authorized phone probe confirmed the zero-code handling fix, then
expired without an archive offer. Source commit `ac394b3` increases the Unix
response budget to 200s and adds safe ignored-control diagnostics; its CI passed.
Its installation did not become ready. A Go stack showed schema meta-validation
of `zalo_get_join_status.output` before opening HTTP. Isolated contract and MCP
server construction completed quickly, so the startup cause is unresolved.
The verified previous `5e92289` binary was restored without reverting SQLite,
session or configuration. Authenticated MCP reads then confirmed connected/
authenticated state, schema 12 integrity and the active v2 journal. The installed
version still has the old 40s Unix write budget; do not claim the long response
fix was accepted on the live installation. No further phone request was sent.

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

Control diagnostics additionally record whitelisted action/status/error scalars,
correlation and presence booleans, and archive size. Fixed validation reasons
cover control decoding and offer envelope/sequence/account/database/cipher checks.
Rejected field values and raw source errors are excluded. The authorized probe
with the status fix reached `waiting_for_backup` and then `INVALID_SOURCE_EVENT`;
no offer/download/import was obtained. That version did not identify the rejected
control, so the active/idle correction alone is not a proven resolution.

The owner-only Unix HTTP response deadline is 200 seconds, above the 190-second
CLI request budget and the bounded phone wait/finalization. A shorter write
budget previously hid the final timeout as `COLLECTOR_UNAVAILABLE`; it did not
mean that the collector stopped. Receiver diagnostics report only fixed reasons
for ignored correlation/account/host controls, and only while a receiver is active.
They neither reveal rejected values nor weaken matching requirements.

The authorized zero-code-fix probe received confirmation and the zero-code
control, then correctly continued waiting. No accepted archive offer or decoder
failure was observed before the source deadline. The ledger ended `interrupted`
with `TIMEOUT`; archive download/import remain unverified. This proves the
zero-code interpretation fix, not successful phone archive delivery.
