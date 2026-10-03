# Direct messaging acceptance — work in progress

This report tracks [the task](task-direct-messaging.md). Source implementation has
passed autonomous checks. Collector recovery and installation were verified live;
Two authorized direct-send operations and v1 notification delivery were exercised;
actual client v2 incoming/first-filter acceptance remains pending.
Synthetic tests use temporary state and fake Zalo sessions;
they do not establish live delivery, permissions of a particular Zalo recipient,
or client discovery. No model evals were run.

## Current evidence

| Requirement | Evidence | Remaining verification |
| --- | --- | --- |
| Direction and first locally known incoming filters | Storage/Events and full-service TLS tests; concurrent live/replay duplicates produce one first fact, retention removes text without resetting novelty | Actual client discovery and agreed live subscription scenario |
| Legacy subscription compatibility | Migration checks retain generation, activation boundary and queued bytes; v2 is separate; injected migration failures roll back DDL and leave the corpus intact | Backup/migration/preservation completed; two existing v1 callbacks delivered and processed by the agent |
| Quoted reply uses the collector session | `internal/service/sending_test.go`: real service, HTTP MCP, retained quote, repeat and restart; one upstream send | Observed one correctly quoted outgoing message; API result remained unknown, so no automatic resend |
| Unquoted send without an existing collected dialogue | Same service test uses an exact synthetic ID with no corpus record | Actual Zalo acceptance; access restrictions remain upstream-dependent |
| Stable send identity and ambiguous results | Storage/manager tests plus full-service HTTP MCP: single claim, changed arguments conflict, lost response stays unknown across restart; shutdown during a blocked upstream call cancels it, persists unknown and frees account lock without resending | Passed: stable sent/unknown before and after restart; one corpus match per approved text |
| Quote retention and replay repair | Deletion removes metadata; matching replay repairs metadata without another message/event; conflicting body cannot attach a quote; nested `api/send_message_contract_test.go` decrypts actual HTTP form, verifies quote fields and encrypted acceptance | Live quoted relationship observed after replay repaired metadata |
| Separate send permission | Default disabled; schema rejects duplicate/empty/overlong recipient IDs; collection policy unchanged; an allowed recipient cannot quote another dialogue's retained record | Verified temporary one-peer permission; original disabled sending policy restored |
| SQLite and upstream failures are reported accurately | Manager tests distinguish STORAGE_ERROR from unavailable quote/status; adapter tests distinguish HTTP/decode uncertainty from distinguishable protocol rejections; MCP failure helper redacts untyped storage errors and validates tool arguments before domain dispatch | Observe actual live outcomes without claiming recipient delivery |
| Documentation and skills | Updated guides, executable contracts and both repository skills; validators and local link audit pass; manual client update instructions documented | Installed copies unchanged; client-specific reload remains unverified |
| ChatGPT discovery | Local installed endpoint advertises existing profiles; see [research](direct-messaging-research.md) | Determine where the external/client catalogue diverges; do not assert a cache defect without evidence |
| STDIO sending | Two actual bridge subprocesses send/read status through one service with one collector; shared UUID causes one upstream send and private text stays out of logs | Installed-client configuration check |

## Checks completed during implementation

From the repository root:

```sh
go test ./...
go vet ./...
go test -race ./...
```

From `third_party/zcago`:

```sh
go test ./...
go test -race ./...
go vet ./...
```

Both project skills passed `quick_validate.py`. Configuration and contract tests
also passed after adding the permissions schema. These results cover the current
implementation stage, not the final clean-checkout acceptance gate.

A separate candidate source snapshot outside iCloud (426 Git-listed tracked and
unignored files, with a SHA-256 manifest) passed root/nested tests and vet plus both
no-CGO cross-builds. It contains the pending working-tree changes, so it is not a
checkout of a published commit. A clean Git checkout remains a publication check.

No-CGO builds for macOS arm64 and Linux amd64 passed after the lifecycle changes.
The [live plan](direct-messaging-live-plan.md) was partly executed with explicit
user approval; collector and send outcomes are recorded below.

## Gates still open

Autonomous contract, migration, transport and recovery checks have passed. The
final publication check must use the exact committed tree, including a clean
checkout, links/schema/skill checks and history/tree secret scanning.
Verify client discovery through the actual connection. The concrete live scenario
is prepared; agree recipient/text with the user and record upstream acceptance
separately from event callback delivery and agent processing. The collector-only installation was subsequently authorized and verified as
recorded below; actual client v2 Events gates remain open. The user
separately authorized committing and publishing the reviewed source before live
acceptance; publication does not establish deployment or live compatibility.
Preserve the account session, corpus and subscription state throughout.

## Listener recovery follow-up

A deterministic cancellation regression fails against the previous `Stop`
implementation: the socket remains assigned and blocks the next start. The fix
clears connection state after all workers exit. A separate loopback websocket
regression performs two actual handshakes with the same listener after a consumer
stops it on a synthetic parsing failure; ten race-enabled repetitions passed.
Reaction reference IDs now accept integer numbers and decimal strings with no
float64 conversion, and reject invalid/fractional/overflowing values.

Root and nested module test/race/vet passed after these fixes. The macOS arm64
no-CGO candidate was built. At this source-test stage installed service replacement, live reconnect recovery
and callback delivery remained unverified; the later authorized collector trial
is recorded below. No live messages were sent or subscriptions changed.

## Offline installed-state migration check

A SQLite online backup of the installed database was opened twice through the
candidate's `storage.OpenWithPolicy`, without session data or any network service.
The private temporary directory used mode 0700 and database mode 0600; both the
copy and probe source were deleted after verification. The installed DB was
opened read-only and was not migrated.

The copy advanced from versions 1–4 to 1–6. All existing rows and columns in 22
application tables were compared exactly and remained unchanged: 21 messages,
two subscription records, and empty event/delivery journals at the snapshot.
`integrity_check` returned `ok`; `foreign_key_check` returned no violations.
Repeated opening passed. This verifies the current installed snapshot's schema
migration, not live callbacks, account-session recovery or send acceptance.

## Authorized live collector recovery and installation

After explicit user authorization, the existing LaunchAgent was stopped and a
private dated backup of the original binary, configuration, agent and state was
verified by exact comparison and SQLite integrity. A candidate using the existing
session and a separate state copy connected successfully, authenticated, and
updated its event/persistence timestamps. Sending remained disabled.

The original trial path exceeded macOS's Unix socket path limit; the candidate
was moved to a shorter private directory before connecting. No truncation or
change to the socket validation was made. Two candidate runs authenticated and
processed available replay without a listener error. The installed-state snapshot
retained all 21 messages and two subscription records (one active direct
subscription). No send operations or queued event deliveries were created.

The verified candidate state and binary were promoted to the documented runtime
paths. The original configuration and existing direct-Go LaunchAgent were retained;
there is one collector. Post-promotion status was `connected`, authenticated,
with no last error and fresh event/persistence timestamps. The installed catalogue
returned legacy, conversation v1 and conversation v2 profiles. Tunnel health and
readiness returned HTTP 200. The pre-promotion state and old binary remain in the
private rollback archive; account/session/callback values were not published.

This confirms account restoration, replay processing, migration preservation and
service startup. It does not prove a newly incoming notification, v2 client
discovery, incoming/first filters through the actual client, direct-send acceptance,
quoted replies, or send retry behavior against Zalo. Those gates remain open;
source and synthetic checks must not be reported as live delivery evidence.

The installed LaunchAgent's automatic recovery was checked by terminating the
verified service process with SIGKILL. It started a new PID without a manual
start, authenticated and returned `connected` with no last error. The corpus
remained at 21 records, one subscription was active and the send ledger empty.
Replay repaired supported quote metadata for ten retained messages. No notification
was observed because the event/delivery journals remained empty during this check.
Sending is still disabled in the installed configuration.

## Authorized plain and quoted direct-send trial

The user authorized two exact synthetic texts to one existing direct peer.
Recipient uniqueness and a retained incoming quote were verified before any send.
Two UUIDs and their exact arguments were persisted privately before enabling a
one-peer send allowlist. No group send, new peer or additional text was authorized.

The plain operation returned `sent` with an upstream message ID. The quoted
operation returned `unknown` / `upstream_ambiguous`. Exact retries of each UUID
returned the same status and ID, before and after service restart. No new UUID
was used to replace the unknown operation. The original disabled-send configuration
was restored byte-for-byte and the service restarted successfully.

The collector independently persisted one outgoing message for each approved text;
the quoted record referenced the exact selected incoming anchor. Corpus size
advanced from 21 to 23. Two existing conversation v1 subscription callbacks were
marked delivered with one attempt each. Read-only inspection of the agent thread
confirmed two completed ChatGPT notification sends, one for each test message.
Thus callback acceptance and agent notification are separately established here.
These were outgoing events; they do not prove v2 incoming-only or first-incoming
filter behavior against the real client.

The quoted API acknowledgement remains ambiguous even though a matching outgoing
message with the correct quote was observed. Raw upstream response/error details
were not retained, so this does not identify a response-decoder defect or justify
changing the send ledger to `sent`. Message observation and operation acknowledgement
must remain distinct. No automatic retry with a fresh UUID is permitted. A private
post-test SQLite snapshot preserves both operation identities for rollback review.
Collector ended connected/authenticated with no error and sending disabled.

Still open: actual client v2 discovery and incoming/first-filter event scenarios,
initiation to a peer absent from the collected corpus, installed skill reload,
and diagnosis of an ambiguous acknowledgement if it recurs in an independently
authorized send. No model evals or unapproved additional sends were performed.

## Send acknowledgement decoder follow-up

A local encrypted HTTP regression reproduces numeric `msgId` replies rejected by
the pinned Go decoder on both plain and quote routes, yielding API code 0 and an
ambiguous operation. An exact integer/string decoder now passes those cases and
rejects invalid numeric values; root and nested race/test/vet passed. This fixes a
verified contract mismatch. The original private API response was not retained,
so its particular `unknown` is not retrospectively changed to `sent`. No further
live messages were sent during this investigation.

The numeric acknowledgement patch was published and installed in the existing
single Go service with a verified private rollback binary. No further live send
was performed. Post-update checks showed connected/authenticated collection,
23 retained messages, unchanged sent/unknown operation records, two delivered
callbacks and SQLite integrity `ok`. Sending remained disabled. No schema or
MCP tool contract changed in this patch; send message IDs are still strings at
the MCP boundary. New live numeric acknowledgement acceptance remains unverified.
