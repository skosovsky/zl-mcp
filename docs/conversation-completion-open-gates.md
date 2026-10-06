> Current update: conversation Events v1 has been removed. Discovery exposes legacy/group and conversation v2. Payload recovery and processing checkpoints are documented in [event-recovery](contracts/event-recovery.md). Earlier observations below retain their historical version names.

# Conversation completion: current open gates

Current checkpoint: 2026-10-06; the dated entries below preserve prior evidence. The full scope is
[task-conversation-completion.md](task-conversation-completion.md). This report does
not reduce that scope or declare completion. Historical checkpoints in
[the acceptance report](conversation-completion-acceptance.md) are evidence for
their recorded versions, not a description of the current installation.

Latest mobile checkpoint supersedes the earlier download/format failure notes:
authenticated offer, scoped download, format-1 checksum/XZ, typed selection and
immutable SQLite inspection passed in authorized installed-service probes. The
selected image contains 24 rows: zero in the September 30 interval and two in
the subsequent September 26 interval (Vietnam local days).
Its latest timestamp matches that peer's live corpus. Newly sent test messages
were in another peer, so they do not demonstrate a stale export. A specific
historical omission and uncheckpointed WAL loss have not been established.
WAL-mode images still fail the current conversion gate. The production mobile
port and worker lifecycle are now wired in source but remain uninstalled; public
mobile source selection and real archive acceptance remain open. Diagnostic inspection now validates and classifies candidates even
when a source gate prevents persistence, without returning message records;
malformed candidates fail instead of being silently classified as blocked. See the [corrected evidence](contracts/mobile-backup-offer-probe.md#selected-image-coverage-and-correction-of-the-live-comparison).

### Mobile journal prerequisite: 2026-10-06

The [atomic mobile checkpoint contract](contracts/mobile-history-checkpoint.md)
now has an internal storage implementation. Schema 13 binds the immutable image
and its fixed expiry to an exact integer timestamp/rowid continuation. Records,
original TTL, permanent identities, novelty state, row-gap counts and operation
revision commit together; failed source/progress writes roll back all of them.
Restart preserves the source binding and charges reserved acquisition work.
Synthetic tests cover rollback, cancellation, account/policy isolation, gap-only
pages, source substitution, large rowids, limits and absence of Events from
historical imports. Full root race passed before the final transaction-work budget
guard; the updated storage/history-import/domain/contracts race checks and full
root vet then passed. CGO-free macOS arm64/Linux amd64 builds and redacted scans
of the worktree and all 40 existing published commits passed. Signed source
[`323dffa`](https://github.com/skosovsky/zl-mcp/commit/323dffa318b13753fb028fc3bbf03458b472f72b)
was published; [both macOS and Linux CI jobs passed](https://github.com/skosovsky/zl-mcp/actions/runs/37438523127).

This remains an internal prerequisite. Public start still rejects `mobile_archive`,
the legacy worker cannot claim its queue, and at that publication neither the
archive reader nor phone acquisition was wired to this port. Schema 13 and these source changes have not been
installed. The two September 26 candidate rows are not restored messages; their
live import and the existing WAL/control/content acceptance gates remain open.

The subsequent internal page adapter connects authenticated encrypted snapshots
to the existing reader/converter and the atomic journal page format. An integrated
synthetic test commits a rejected/expired-only page, restarts both stores, resumes
large integer rowids without renewing snapshot/message TTL and verifies two retained
records with no Events or deliveries. Binding changes, incomplete continuations,
WAL/control gates and cancellation return no writable page. This supersedes the
reader-format integration gap, while acquisition/worker wiring and public/mobile
acceptance remain open. No live snapshot was requested or imported.

One full local test run exposed startup cancellation being reported as a storage
failure by the existing history worker. Its recovery path now treats cancellation
errors from a cancelled context as graceful stop while preserving genuine storage
failures. Dedicated tests verify both outcomes; do not represent the initial
failed run as passing evidence.
After that fix, full root tests and vet passed, as did the affected collector,
history-import and mobile-backup race checks and CGO-free macOS arm64/Linux amd64
builds. The integrated adapter remains uninstalled and public source selection
remains disabled.

Signed adapter checkpoint
[`f7038c0`](https://github.com/skosovsky/zl-mcp/commit/f7038c0609382b9ceca67d2287a72368f8713585)
was published; [macOS and Linux CI passed](https://github.com/skosovsky/zl-mcp/actions/runs/37439506274).
The subsequent acquisition binding adds schema 14: one transaction creates a
single phone-attempt/operation link before dispatch. Its guard validates the
history revision, reserved work and account/scope again at dispatch time. Tests
cover link-write rollback, retry identity, changed budgets, stale/recovered/
cancelled history, account/policy revocation, refusal to adopt an unlinked attempt,
unchanged owner-diagnostic dispatch and atomic migration. Full root tests/vet
and storage/mobile-backup/history-import race checks passed. This is still an
internal port; the acquisition/worker loop is not wired, schema 14 is uninstalled,
and no phone, live import or subscription operation was performed.

The acquisition binding was published as signed
[`d611c38`](https://github.com/skosovsky/zl-mcp/commit/d611c387a3ce80dbc73c5a3aa1cfb4c4cfb217af);
[its macOS/Linux CI passed](https://github.com/skosovsky/zl-mcp/actions/runs/37440505244).
The subsequent internal mobile worker consumes only mobile operations through a
typed session callback. It reserves/reconciles phone (180s), download/save (120s)
and page (30s) stages separately within the 420s allowance, then atomically commits
pages. A linked operation never calls the receiver again, including a prepared
attempt without a snapshot. Non-page work completion preserves source/page counts.
Inactive owned attempts are closed without resetting terminal evidence or granting
network authority, including after scope revocation.

Integrated synthetic tests exercise durable dispatch, encrypted save, actual
reader/converter paging, gap-only pages, restart without redispatch/TTL renewal,
snapshot loss, pre-dispatch failure cleanup, cancellation, malformed pages,
unsupported WAL sources and unauthenticated admission. Known WAL/control source
gates now have a distinct unsupported error instead of being reported as malformed
SQLite. These tests use no Zalo network or model evals. The production session port,
shared startup recovery/lifecycle wiring, live archive acceptance and public mobile
source selection remained open at that checkpoint; the worker was not installed or running in production.

### Production session integration: 2026-10-06

The current service source now creates the private encrypted snapshot store and
starts the mobile queue under its existing collector session. Recovery precedes
both legacy and mobile workers. The service cancels and joins mobile work before
clearing the session port, and closes snapshots after service workers stop.
Synthetic production-port tests exercise authenticated archive acquisition,
encrypted snapshot publication and page processing using one restore/listener,
one offer and one download, with zero historical Events/deliveries. Separate tests
verify parent/service cancellation, upstream session loss, scope release, private
scratch cleanup and redacted session formatting. No phone request or deployment
was performed for these checks. WAL/control/content eligibility and the two real
September 26 candidates remain unaccepted; public mobile input is still disabled.
Full root tests and vet, affected service/collector/history-import race checks,
CGO-free macOS arm64/Linux amd64 builds, diff whitespace validation and a redacted
worktree secret scan passed for this integration. These are synthetic/source
checks, not proof of live mobile history recovery or installed-client acceptance.
Full root tests/vet and the affected storage/history-import/mobile-backup race
checks passed for this checkpoint. The scan of the worktree found no secrets.

### Published probe checkpoint and native installation: 2026-10-06

Signed source commit [`57d4c8d`](https://github.com/skosovsky/zl-mcp/commit/57d4c8da9a19a1c5f589b4d52a74d66e49699dc9)
was pushed to main. [Its CI](https://github.com/skosovsky/zl-mcp/actions/runs/37428680045)
passed root and nested-module test/race/vet and CGO-free builds on both macOS and
Linux. Local vet, cross-builds, updated reader/service race tests and all 36
published commits' secret scan passed. One initial concurrent local root race
run timed out at the Events integration test's five-second startup deadline;
that scenario then passed three isolated runs, the full service package passed,
and both CI jobs passed. Do not erase the initial failure from the evidence.

The signed native candidate SHA-256
`b0780bd2f8f1a09c2024cf411a6a80951dcb9bed11d5e718a505481df4803536`
failed installed readiness. A sampled live process remained in dyld/libobjc
initialization before Go startup with the MCP port closed. That observation does
not establish an application-code regression or its underlying OS cause. Only
the executable was rolled back; neither session nor database was restored or
changed by the rollback. The prior verified binary SHA-256
`afc77e4dc2ded6fc741dd3fe6d50fac0badb1285eae4536a48f62a368ee18341`
subsequently returned connected/authenticated with no last error. SQLite
quick_check passed; subscription IDs, activation/generation watermarks, filters
and lifetime fields matched the pre-update backup. Credentials were not read or
compared. That first attempt and rollback remain historical evidence.

A subsequent isolated native smoke test of the exact signed candidate used fresh
private temporary state without a Zalo session: HTTP MCP discovery returned 21
tools and status correctly returned `auth_required`. After the installed-path
`-h` preflight passed, the same candidate was installed through the existing
LaunchAgent and reached connected/authenticated status. A later independent
read confirmed SHA-256 `b0780bd2f8f1a09c2024cf411a6a80951dcb9bed11d5e718a505481df4803536`,
schema 12, SQLite integrity, one active v2 subscription and an empty unblocked
recovery journal. Subscription boundaries matched the private pre-update backup.
No phone operation, send, subscription change or recovery acknowledgement was
made during deployment. The initial OS initialization delay remains unexplained;
it is not a proven application regression. Published docs checkpoint
[`514d7ed`](https://github.com/skosovsky/zl-mcp/commit/514d7edfd726422cf1db85fbe23de1800a5a5e7a)
also has [successful macOS/Linux CI](https://github.com/skosovsky/zl-mcp/actions/runs/37429531864).

The new private encrypted snapshot store is an internal prerequisite for durable
mobile import, not an exposed import feature. Its [contract](contracts/mobile-backup-snapshot.md)
requires request/account/digest binding, fixed expiry across retries, encrypted
selected-only bytes, durable spent UUIDs and bounded private cleanup. It is not
wired into the service or history worker. WAL/control conversion gates remain in
force.

### Snapshot prerequisite and current client check: 2026-10-06

Signed source [`52a541a`](https://github.com/skosovsky/zl-mcp/commit/52a541a7d9eb8f971456b2d10bd40a75efcd0c3b)
contains the private snapshot store and its AAA regression tests. Root and nested
module test/race/vet, CGO-free cross-builds and redacted scans of the worktree and
all 38 published commits passed. [CI passed on macOS and Linux](https://github.com/skosovsky/zl-mcp/actions/runs/37432573776).
The store remains unwired; this commit required no production process update.

The connected Codex client's catalogue now includes all three history-import
tools, superseding the earlier absence report. An initial `zalo_get_status` call
returned `-32603: MCP request timed out`. At the same time local MCP status
remained connected/authenticated with no error. Read-only tunnel log inspection
found control-plane TLS/header/poll timeouts against `api.openai.com`; these do
not establish the exact cause of the individual failed tool call. An unauthenticated
HTTPS reachability check then succeeded and one client retry returned
connected/authenticated with `last_error=null`. Neither process was restarted,
no credential values were printed or changed and no import or subscription was changed. Current
client connectivity is accepted; intermittent transport failures and actual
client history-import workflow acceptance remain separate gates.

### Attachment decode checkpoint: 2026-10-06

Read-only native inspection established the nested tag-6 field names and exact
32-bit integer/UTF-8 wire paths. The [attachment contract](contracts/mobile-backup-attachment.md)
and parser preserve explicit presence and ordered outer repeats, reject malformed
whole results and clear private owned data. BinNet now decodes attachment fields
instead of treating the whole outer value as an unknown occurrence; nested unknown
fields still count. The converter still reports attachment-bearing records as
unsupported content. Thus lower unknown-field counts do not establish supported
rendering or complete import. Root/nested race/vet, cross-builds and the bounded
Go fuzz check passed. These source changes have not been installed or tested on
a fresh phone archive; the current service remains the previously accepted native
probe build. No phone request, Zalo send or subscription edit was performed.

| Requirement | Current evidence | Remaining gate |
| --- | --- | --- |
| Browse without a keyword | Current installed HTTP and actual connected-client catalogue→period browse→stable continuation→matching context checks pass; production regression tests | Final signed checkout/publication verification |
| Expanded conversation catalogue | Guarded periodic preload refresh and actual connected-client catalogue reads; source/installed skills match and this desktop discovers both | Final deployment/publication consistency and other intended clients |
| Explicit silent imports | Durable history journal with UUID/retry/cancel/restart tests; installed direct preload/group reads; synthetic archive-to-record-to-silent-storage callback retains text without Events; production mobile session/worker integration verified synthetically | Three import tools are now discovered by the connected client; its import workflow and real mobile import remain unverified; current source is not installed |
| Older Strangers message | Requested peer is selectable; September 30 archive interval is empty and September 26 has two candidate rows; preload only supplies an already retained record | Recovery from a deeper verified source, or a concrete verified source limitation agreed with the user |
| Mobile request/transport | Current-session request/offer observer, durable dispatch ledger, restart recovery and private offer ownership; host-scoped cookie transport, scoped body consumption and revocation tests | Correlated live offer and owner mapping now accepted; download and selected sender mapping accepted by later probes; durable production import remains |
| Mobile archive reading | Independent format-1/XXH32/XZ/OpenSSL and SQLite fixtures; typed file selection, immutable bounded SQLite reads with exact fractional-millisecond interval mapping, digest/request/account-bound keyset paging; one session/download across pages | Format/SQLite/plain-to-session reading accepted; full BinNet/content semantics remain |
| Mobile conversion/import | Supported plain-text candidate conversion preserves exact IDs, sender, timestamp, direction and original TTL; explicit unsupported rich-text/nontext and quote/mention counts | Nontext/attachment/quote/mention semantics and atomic durable page/checkpoint persistence; text-only support is not the final feature |
| Deleted/recalled messages | Source schema 11 prevents restoration of an observed exact typed deletion by live replay or history; absent-message deletion, restart, namespace, novelty and rollback tests. Whole-file mobile type-33/36 detection blocks unverified conversion | Mobile control target/state semantics and full handling before public archive persistence; live marker receipt alone does not establish mobile compatibility |
| Expiry after import | Expiry markers, SQL read-time filtering and cleanup are present. New trusted expiring operation-page port atomically commits records/TTL/checkpoint; rollback/restart/duplicate/cancel/due-expiry tests passed | Wire into the mobile importer, quote expiry and trustworthy clock/real archive acceptance |
| Group pages | Installed phase-aware candidate; two live groups each returned a terminal recent page; durable phase traversal covered synthetically | Live old-phase traversal when available; no completeness claim |
| v2 discovery and filters | Installed schema-12 service exposes legacy/group and conversation v2 only; v1 contracts and implementation removed, migration cancels obsolete pending jobs; direction/novelty/restart tests | Client recovery discovery/adoption confirmed by Ann; agreed incoming-only/first-only live scenarios remain |
| STDIO transport | Exact current installed bridge initialized a real STDIO client at `2025-11-25`, listed 21 tools including recovery and read connected/authenticated status; no Events advertised | STDIO does not replace external HTTP Events acceptance |
| First direct send outside catalogue/corpus | Synthetic authorization/acknowledgement tests and preserved send ledger | A user-selected recipient and separately approved exact send |
| Numeric acknowledgement plain/quote routes | Synthetic encrypted-wire tests; earlier two-send acceptance retained | Separately approved fresh live sends; unknown operations must not be resent |
| Deployment/publication | Earlier schema-11 migration/publication checks preserved below. Signed recovery implementation `19e2105` installed with verified binary/SQLite/skill backup; docs commit `07791a0` pushed; local root race/vet, cross-builds and secret scan passed | Remaining feature/client/live gates; recovery source and docs have successful macOS/Linux CI at `07791a0` |

The current installed binary uses schema 12 and includes the plain-owner mapping
fix, stage-specific mapping diagnostics, verified exact-endpoint auth-cookie fix
and candidate classification (SHA-256
`b0780bd2f8f1a09c2024cf411a6a80951dcb9bed11d5e718a505481df4803536`).
Authenticated reads confirmed recovery tools, connected/authenticated state with
no last error, SQLite integrity and one active v2 subscription. The user reported
Ann discovering all three recovery tools, reconciling three exact message IDs
with completed notifications, acknowledging in order without duplicate notices,
and updating/verifying the enabled automation to read the journal first. An
independent local read confirmed an empty journal without a delivery block; this
local verification made no acknowledgements. Recovery adoption and backlog
reconciliation are complete. See [recovery acceptance](event-recovery-acceptance.md)
for evidence boundaries; exactly-once is not claimed.

The schema-11 checkpoint below remains historical. Experimental mobile ports and an owner-only [offer probe](contracts/mobile-backup-offer-probe.md) are present,
but have no MCP archive dispatch/import route. Real download, selection and SQLite
inspection are accepted; conversion and persistence remain open.
The probe is installed and tested synthetically. Authorized phone probes received
confirmation and established/fixed zero-code control handling. A correlated
archive offer was later received; direct comparison of plain/session IDs was
corrected. The latest probe was rejected by the identity API outer envelope with code 600.
The installed endpoint-specific version-691 candidate returned the same code
600 in a subsequent live probe; version mismatch did not resolve the failure.
A missing zpw_sek at the identity host was then established from cookie scope and
native-source comparison. A subsequent live probe with the installed correction
passed authenticated owner mapping and reached `offer_ready` (246,660 bytes).
That identity-mapping checkpoint downloaded/imported nothing. Later authorized
probes verified archive download and selected reading as described above; no
production archive import has occurred.
Initial migration verification used
a private database copy; that copy was removed afterward. The latest 8→11 check
compared complete message/identity/send/novelty records and subscription IDs,
filters, generations, watermarks and lifetime fields. Callback and secret values
were not read or compared; do not claim a full subscription-row comparison.
Snapshot builds are
reproducibility evidence, not a signed clean Git checkout or release artifact.

Selected-archive traversal is bounded by source-row budget, at most 100 pages and
120 seconds; rejected/expired-only pages advance independently of retained rows.
Borrowed buffers and continuations are cleared. Counts preserve source/type/control
and metadata gaps. Archive exhaustion does not establish complete Zalo history.
A failed callback discards the aggregate result; a future importer must atomically
commit a page with its checkpoint rather than infer durable progress from counts.

Historical imports remain silent. Existing service/session, corpus, send operations
and subscription boundaries must be preserved. No model evals are used. Close each
remaining gate against its full requirement, not a smaller synthetic substitute.

Latest worktree verification on 2026-10-05 includes schema 11 and the exact
fractional-millisecond archive interval fix: full root and nested-module race
tests and vet passed, as did `git diff --check`. CGO-free builds passed for
macOS arm64 (SHA-256
`58723498bec69a396e6c5a5a8bbda15c6d5631c85b7d43b1c20be681805612a7`)
and Linux amd64
(`52c28da2c2940c8776ee5caf64ea0433100b128205fd92cf0a506facd6edcb3b`).
These builds were made from the uncommitted worktree and were not installed;
they do not close migration, real-archive, client or publication gates.

The subsequent isolated candidate uses `-trimpath`: macOS arm64 SHA-256
`0d21c0dc17ad04b967b0e361a375a8c1f2515d111cb3f06e74582af399cd8646`,
Linux amd64 `92debb4623d46aaeff791adddd0fb538584369217c9fd724b677c68fd55a8d93`.
All 643 source-file hashes remained unchanged after the migration helper was
removed. This candidate closes the offline schema-11 migration gate only.
An actual connected-client status read still reports authenticated/connected,
47 stored messages and no last error; its catalogue still exposes 15 Zalo tools
and none of the three history-import tools. No service or subscription changed.

That observation preceded installation. The checked candidate was subsequently
installed through the same existing LaunchAgent on 2026-10-05 local time. A cold
full-state/config/binary/plist backup was verified before replacement; startup
returned authenticated/connected within the bounded acceptance window, and the
connected client's status call also succeeded. Database integrity and migration
8→11 were verified; all pre-existing message/identity/send/novelty records and
the selected subscription fields remained present and unchanged. No login,
phone archive request, send or subscription mutation was performed.

Local `tools/list` returns 18 tools, including import/status/cancel; local
`events/list` returns legacy, conversation v1 and v2. The current external
client's available catalogue still has 15 tools and excludes import/status/cancel.
Successful status through that client does not prove its v2 event discovery or
resolve that catalogue discrepancy.

Discovery follow-up: the installed `server/discover` advertises tools, resources
and Events and supports `2026-07-28`; local lists contain all 18 tools and three
event profiles. Official personal-plugin guidance requires Refresh followed by
testing in a new enabled conversation (see [connection guide](local-connection.md#refresh-chatgpt-plugin-discovery-after-an-update)).
The available controlled browser account has no Zalo personal plugin, so no
refresh was applied there. Verification must use the owning account/workspace;
this observation is not proof of a tunnel or server cache failure.

Publication checkpoint: SSH signature verification passed for
[`fc88336`](https://github.com/skosovsky/zl-mcp/commit/fc8833646f502357ea5a2bccd85bb53c999ede48),
and remote `main` matches it. Clean-checkout checks passed for root and nested
race/vet, CGO-free macOS arm64/Linux amd64 builds, LaunchAgent plist and 173
relative documentation/skill links. Gitleaks 8.30.1 found no secrets in the entire
published `main` history. All 468 Go/module files in the installed candidate's
source manifest match this published commit.
[Its Linux/macOS CI run](https://github.com/skosovsky/zl-mcp/actions/runs/37316075019)
completed successfully for both jobs and the exact commit SHA. This checkpoint does not close the remaining
archive/import, client Events or fresh-send requirements.
