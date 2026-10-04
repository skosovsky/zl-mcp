# Conversation completion: acceptance evidence

Status: work in progress. Tracks [the combined task](task-conversation-completion.md). No model evals are used.

## Local browsing implementation

The executable input/output schemas were added before the storage and MCP dispatch implementation. `zalo_list_conversation_messages` provides exact typed local chronological browsing without a keyword, bounded pages, RFC3339 `[since, until)` and signed snapshot continuation. Existing search behavior remains unchanged. The [browsing contract](contracts/conversation-browsing.md) describes snapshot versus current coverage, resource retrieval, empty reasons and limits.

Added storage regressions exercise same-time ties, both orders, timezone offsets/exclusive upper boundary, a late historical insertion excluded from an existing traversal, continuation after reopening SQLite, colliding direct/group IDs, direction, Unicode truncation, empty reasons and permission checks. An MCP client calls the new tool without a query and reads the returned escaped resource URI, validating the embedded output contract through the actual handler. Tool-count discovery expectations now include the additive tool.

This stage uses temporary synthetic databases only. No installed binary, session, configuration, subscription or runtime data was changed. Strangers protocol discovery, directory expansion, explicit history import, live v2/first-incoming checks, installed skills and final publication remain unfinished. Do not interpret source capabilities as capabilities already available in the connected client.

Validation for this checkpoint: root `go test ./...` and `go vet ./...` passed; focused `go test -race ./internal/storage ./internal/mcpserver ./docs/contracts` passed. Both repository skills now describe conditional use of the additive browse capability, without assuming the installed client already exposes it. Complete root/nested race checks, clean checkout validation, deployment and client reload remain later gates.

Checkpoint commit `0079c5edbd9b62aa07f7664244435254518f02c3` was pushed to public main. [CI run](https://github.com/skosovsky/zl-mcp/actions/runs/37149944708) completed successfully for Linux and native macOS. Tree and checkpoint-commit secret scans found no leaks; both repository skills passed structural validation. These checks do not establish installed browse availability or resolve the remaining scope.

The subsequent [native-client static-code investigation](conversation-discovery-history-research.md#installed-native-client-static-code-follow-up) found local Strangers classification and a known-thread preview request candidate. It does not yet establish full inbox enumeration, a working direct-history API or recovery of the reported missing conversation. No installed runtime changed during this investigation.

## Offline continuation and directory source checkpoint (before installation)

The native-client investigation identified response continuation fields omitted
by the pinned Go listener. The new source patch preserves queue origin and
`more`/`lastActionId`, emits metadata once after mixed batches, and sends a
bounded continuation only after the batch has persisted. Root replay-pager and
nested listener/model race tests passed. These synthetic results do not establish
that the reported missing historical conversation will be recoverable.

The additive directory schemas now allow aliases, relationship metadata,
metadata provenance and an explicit `has_stored_messages` flag. A transactional
contact page merge respects collection policy and does not create message,
Event or first-incoming records. Local catalogue reads use accent-insensitive
candidate matching, including Vietnamese đ/d, without identifying people by
name. Storage tests verify metadata-only entries, actual-message upgrade,
policy exclusions, atomic rollback and reopening the migrated database.
Root storage/MCP/contracts tests passed. Installed upstream refresh, source diagnostics,
deployment and live acceptance are still pending; this checkpoint
must not be described as a working complete inbox or complete history import.

The source now includes a guarded contacts-page adapter and cancellable background refresh in the existing service session. Synthetic tests cover short/repeated/oversized pages, 20-page termination, partial-error retention and cancellation. The MCP diagnostics resource validates its safe payload against an embedded JSON schema; a real in-memory MCP client verifies alias discovery and metadata-only entries through the updated tools. This remains uninstalled source evidence.

## Installed checkpoint: 2026-10-03, revision 2699d2e

[CI](https://github.com/skosovsky/zl-mcp/actions/runs/37151538751) passed root and nested-module test/race/vet on Linux and native macOS, plus the configured CGO-free cross-builds. The installed arm64 Go binary SHA-256 is `b6f72e447ed104ca052f5f45bc2db5193d44d5f1932fe5f7fdaad2982d80b0b4`. The existing single LaunchAgent was retained. After bootout, installation waited for account-lock release, verified a private full-state/config/binary/plist backup outside iCloud, then replaced the binary atomically and bootstrapped the same agent. No login, message send or subscription mutation was performed.

SQLite migrated from version 6 to 7. The collector is authenticated/connected. All 23 messages and permanent identities, first-incoming facts, both subscription rows and both send-operation rows match the stopped-service backup exactly. The existing ambiguous send remains `unknown`. The first contacts refresh observed/permitted 14 unique IDs and ended on a short page; the catalogue now contains 16 typed entries. This proves source refresh, not completeness of all dialogues.

The installed authenticated HTTP endpoint advertises 15 tools, including keyword-free browse, and all three existing event profiles including v2. Actual browse returned two messages and a continuation page with the same snapshot; the diagnostics resource reported the completed contacts refresh. Token values and message bodies were neither logged nor published.

Live queue metadata reported `queue_exhausted` after one page for both direct (12 records) and group (2 records) queues. All replayed records were duplicates, with no persistence/decoder error. This verifies the terminal metadata path only; a live `more=true` continuation has not been observed. The reported Strangers dialogue still does not match exact/accent-normalized catalogue queries. Available replay and friends metadata did not recover that conversation or its September history.

The currently exposed connected-client tool catalogue still lacks the new browse tool (and the send/status tools). Server-side discovery alone does not prove refreshed plugin discovery. Repository skills are structurally checked; no local copies of either project skill were found in the Codex skills directory. Remaining gates include client capability refresh, explicit history import, Strangers recovery and the uncompleted v2/new-recipient live scenarios. The goal remains in progress.

## History adapter checkpoint (source only)

Revision 6a17fad introduced the bounded group-cloud page candidate; its
[CI](https://github.com/skosovsky/zl-mcp/actions/runs/37152344493) passed.
The subsequent domain adapter normalizes exact numeric identifiers/timestamps,
checks the requested typed group identity, preserves nullable continuation and
filtering evidence, and rejects a whole malformed/oversized page without yielding
partly importable records. Direct history and missing advertised group-cloud
sources produce an explicit unsupported result.

Synthetic adapter tests verify own/incoming direction, exact IDs above 2^53,
untrusted text preservation, invalid/mismatched records and unchanged empty
storage. Normalized history records carry no ordinary persistence source;
Store.Put rejects them. A session-guard regression verifies cancellation of an
in-flight history request and rejection of another request after shared auth
loss. Targeted root/nested race tests and root vet passed. This is not installed
or live history acceptance and does not recover the reported Strangers dialogue.
The user decision on notifications during explicit backfill remains pending;
ordinary live/offline replay semantics have not changed.

The next directory source checkpoint adds bounded known-ID profile reads behind
the shared session guard. Pure/stub regressions reject foreign or conflicting
response identities, duplicate/ambiguous version keys and invalid batch sizes;
verify that missing/unchanged profiles are not fabricated; and protect the
caller's exact IDs from the pinned SDK's mutation of its variadic argument slice.
Only directory fields cross the domain boundary. Targeted root race tests passed.
At that adapter-only checkpoint, background scheduling and installed acceptance
were pending; it does not establish discovery of unknown Strangers IDs.

Known-ID profile enrichment is now wired into the same background directory
worker, with a separate diagnostic profile result. It scans at most 20 batches
of 100, preserves policy exclusions, advances past missing/excluded pages and
does not fabricate records for unchanged/missing upstream profiles. Storage
regressions exercise excluded-page continuation and cached-ID suppression;
collector regressions exercise completed/missing/foreign responses and the
2,000-ID limit. Root storage/collector/Zalo/MCP/contracts race checks passed.
This worktree checkpoint remains uninstalled and unpublished while the prepared
Git commit is waiting for its configured SSH signer.

Known profiles older than one hour become refresh candidates without deleting
identity or message history; a regression checks fresh-cache suppression and
stale-cache eligibility. Both repository skills now distinguish profile metadata
from messages/Events and interpret the separate profile-source diagnostics.
Structural validation passed for both; no model evals were run.

An additional cancellation regression stops profile refresh after the upstream
response but before persistence. It verifies that no profile is written and the
final diagnostic reports `cancelled_or_deadline`, rather than a storage fault or
a lingering `running` state. Root tests/race/vet and nested zcago race tests/vet
passed on this worktree. CGO-free builds produced a macOS arm64 Mach-O binary
and a Linux amd64 static ELF binary. The redacted tree secret scan found
no leaks. These checks do not establish installed or connected-client acceptance.

A worker-level race regression exercises `contactsLoop` with the guarded source,
then causes shared authentication loss while a profile request is in flight.
It verifies final `partial/auth_required` diagnostics, rejects a subsequent
profile read before it reaches upstream, persists no profile and stops the
worker on shutdown. This proves the synthetic lifecycle/authentication boundary,
not a real-account profile lookup or unknown-Strangers discovery.

The [explicit history-import design draft](contracts/history-import-operation.md)
now separates the page adapter from a durable MCP operation: request conflicts,
account/policy boundaries, atomic page checkpoints, restart/cancellation,
continuation evidence and honest coverage are specified as implementation gates.
Notification/novelty semantics remain a pending decision. No executable import
schema, MCP import tool or historical persistence capability is claimed.

## Natural incoming acceptance: 2026-10-04

The user's client screenshot confirms assistant notifications from a peer outside
the previous list. A read-only database check matched that dialogue without
publishing its identity or message bodies: six records arrived through `live`
between 06:24 and 06:28 UTC, five incoming and one outgoing. All six corresponding
delivery rows are `delivered/accepted`; the first required two attempts and the
others one. The screenshot independently confirms assistant handling, rather
than relying only on callback acceptance. One known first-incoming fact is
retained for the dialogue.

The effective subscription remains v1 `zalo.conversation.message.created` with
`scope=direct`, `direction=all` and no first-incoming-only filter. This establishes
natural direct ingestion and delivery beyond the old recipient list, not live
v2 incoming-only/first-incoming filtering or discovery of all old Strangers
history. No subscriptions, send permissions or service state were changed for
this inspection. The prepared history-normalization commit completed as 8208b39
after the user confirmed its configured SSH signature.
