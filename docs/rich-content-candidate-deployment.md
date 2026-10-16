# Rich content: historical candidate deployment worklog

This file records successive historical stages, including failed and superseded
candidates. For the installed version and remaining gates, use the
[current acceptance matrix](rich-content-acceptance.md); later notes here do not
constitute whole-goal acceptance.

Date: 2026-10-15. Status: candidate installed on the existing Mac; live acceptance,
final independent acceptance and publication remain open. This is not evidence
that all native media/call classes are supported.

## Upgrade

The existing native Go service and its existing LaunchAgent were used. No source
checkout moved and no second Zalo listener was started. Before replacement the
agent was unloaded and the account lock acquired. A private full state copy,
configuration, plist and prior binary were saved outside iCloud. Fifteen state
files were verified against their source hashes; a separate temporary copy of the
complete SQLite main/WAL/SHM family reproduced the baseline and passed integrity
check. The original backup remains private. No old database was restored.

| Artifact | SHA-256 |
| --- | --- |
| Prior native binary | `09f82dfdfd0c9f57b349007768bda2ba83501e009751f98a369ecb5f387a666a` |
| Installed candidate | `4558a2e1fac69ab7410ddc3c49700be2bf5ecc8cf8d5520100a791a10c2eff4a` |

The initial candidate was subsequently replaced by the tested diagnostic build
`e46b09b4694c8bda3e0e3ba5a6a720c4eed59562bda3f7ea831b30fbddb3cbd2`.
It corrects native reaction `rType`/`source` JSON tags and adds bounded reaction
reference/photo-shape diagnostics. The previous candidate binary is privately
retained. The same LaunchAgent/service was restarted; all eight test journal
records/receipts and callback hashes were unchanged and the collector reconnected.
The production v2 definition remained unchanged. Neither build is a published
revision yet.

The client-only reaction-target fix was subsequently deployed as native build
`509b6f7446209232dfb582106d307762800e56f069b05bc78a05c352ca072093`.
The root/harness linter and adapter/storage/events race suites passed. Two
independent reviewers accepted the bounded lookup and upgrade-identity guards.
The prior diagnostic binary is privately retained. The same service reconnected;
both pending test records/receipts and all ten verified callback files remained
unchanged. The working v2 definition was again checked unchanged. The corrected
lookup still requires a new native incoming observation for live acceptance.

That live gate subsequently passed: a new incoming reaction supplied only a
client reference, which resolved to the exact original synthetic emoji message.
The retained source, independently signature-verified callback and actual MCP
journal matched. Ack and its idempotent repeat succeeded. This proves the
supported local direct-text resolution path; action semantics and other target
metadata coverage remain limited as documented.

The candidate is an uncommitted worktree build. Its hash identifies this concrete
binary, not a published revision. Publication/release provenance remains a gate.

## Post-upgrade checks

SQLite migrated from version 14 to 16 and passed integrity check. Before any
local test subscriptions were added, read-only comparison confirmed:

| Invariant | Result |
| --- | --- |
| Original messages, old columns | All 92 rows preserved exactly |
| Message identities | All 93 preserved |
| First-incoming facts | All 6 preserved |
| Subscription definitions/revisions | All 9 preserved exactly |
| Processing checkpoints | All 10 preserved exactly |
| Previously retained signed delivery payloads | All 42 byte hashes unchanged |
| Existing active subscription | One v2; unchanged |
| Collector via installed MCP | Connected, authenticated, no last error |

Actual local `events/list` returns legacy, message v2, message v3 and reaction v1.
Call v1 is not advertised. An actual `resources/read` against an existing message
returns the new metadata resource with explicit legacy provenance; no media
bytes were downloaded. External client discovery/automation behavior is separate.

## Live stand preparation

A temporary native Go receiver listens only on one exact loopback endpoint. An
operator allowlist entry was added for that URL and the same service restarted;
all other configuration fields remained equal and outbound sending stayed off.
Independent verification of the exact Standard Webhooks HMAC bytes and challenge
succeeded for two new exact-direct subscriptions: incoming message v3 and
incoming reaction v1. Both have no time expiry. They do not widen or replace the
working v2 subscription. Callback bodies/signing material and exact identities
remain in private local test artifacts, outside the repository.

The initial requested live batch is text emoji, sticker, photo without/with
caption, then reaction addition/removal. No results are claimed before receiving
and correlating actual source/read/resource/journal/callback evidence. No test
message or call has been sent by the assistant. No phone synchronization requested.

Both installed skills were backed up and replaced with repository copies;
all six files in each match by content hash. Structural validators passed.
This does not refresh cloud automation instructions or prove model behavior.
No new model evals were run.

## Remaining gates

- Inspect the agreed live batch and classify each native shape without guessing.
- Gather any remaining native evidence for advertised mappings; keep unsupported
  call/video/contact/action semantics explicit rather than passing synthetic proof
  off as live acceptance.
- Reconcile v2/v3 overlap using the [operator barrier procedure](rich-content-migration.md)
  only for an explicitly agreed consumer transition. The
  current working v2 subscription remains intact; no transition is claimed.
- Verify restart/recovery of retained new events, then account for and cancel test
  subscriptions and remove temporary receiver/configuration artifacts safely.
- Final acceptance by both independent reviewers, commit/push and green CI.

Rollback must obey [the read-only compatibility gate](contracts/rich-content-events.md#rollback-precondition-and-proof).
A stopped backup does not authorize discarding newly collected messages or ack.

## Subsequent prepublication checks

Architecture compliance/coverage passed at 100%, with closed exceptions,
production graph, 14 isolated and 6 source controls. The adapter's standard-library
allowlist adds only `crypto/sha256` and `unicode/utf8` for bounded metadata and
privacy-preserving identity. Root/harness lint passed. The existing offline CI
profile passed 50/50 cases and 480/480 assertions, with model/live NOT_RUN.
Full root race passed after removing an unrelated shared fixture deadline from
the existing response-budget test; its assertions are unchanged and three focused
race reruns passed. The remaining macOS amd64 CGO-disabled build passed too.
This evidence does not close the pending actual live scenarios.

## Test peer switch before live execution

The user chose a different existing test account for convenience. Before the
switch both original local test subscriptions had zero delivery records and no
new source observations. Those two empty subscriptions were cancelled; two new
exact-direct incoming-only subscriptions were activated for the chosen test peer,
with no expiry. Independent signed verification passed. The original active v2
subscription remained byte-for-byte unchanged, and the service was not restarted.
Private old/new definitions were retained for audit. No live result is implied.

## Call investigation instrumentation

The next native diagnostic build is
`9ec138d5ddb597019dddb1b9f849f354a2f742b7552661d96649074020e10f86`.
It adds bounded numeric unknown-header snapshots and ignored action/control
counts; it does not enable call classification, a call channel or call Events.
SDK listener race, full SDK test/vet, adapter race and root/harness lint passed.
Both reviewers accepted the preparation with the explicit counter-generation
limitation documented in the [upstream evidence](rich-content-upstream-evidence.md).

The same LaunchAgent was updated, retaining the previous binary privately.
Initial preflight encountered a read timeout before any stop/replacement; fresh
read-only MCP status/journal checks succeeded, so the planned update proceeded.
Afterward the collector reconnected, both test journals remained empty, all
eleven verified callback files were unchanged, and the working v2 definition
was unchanged. A private baseline records source high-water marks, connection
generation, numeric diagnostics and log position. The new diagnostic cycle was
observed from the installed process, and the independent local receiver passed
its health check. At that intermediate stage no diagnostic call had been accepted as a live
result; the later user-confirmed two-call observation supersedes that status.

## Exact-record diagnostic candidate

The subsequent installed native candidate has SHA-256
`9d5735429a413e163790bedd95d9c5bb7c365f48ef6baf42324652f52677df85`
and schema 17. A fresh stopped-service backup and post-upgrade conservation
checks preserved corpus, subscriptions, signed payloads and processing cursors.
The owner-only exact preload selector matched the retained unknown record and
returned only the allowlisted structural keys described in the implementation
checkpoint. No call mapping has been enabled. Intermittent MCP read latency is
still open; successful readiness does not close native acceptance.

The latest installed type-diagnostic candidate is
`b41c96917436a56b17bcf578a694a7959b50b66459037557fe47d45696447618`
(schema 17). The old Background LaunchAgent policy was changed to Standard after
a comparison with the same binary restored native readiness. The new candidate
then passed official SDK status/journal reads and exact-record type inspection.
Retained data and all subscription definitions remained unchanged. Call
classification is still open; see the checkpoint for the bounded evidence.
