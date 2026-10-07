> Current update: conversation Events v1 has been removed. Discovery exposes legacy/group and conversation v2. Payload recovery and processing checkpoints are documented in [event-recovery](contracts/event-recovery.md). Earlier observations below retain their historical version names.

# Conversation completion: current open gates

Current checkpoint: 2026-10-07; the dated entries below preserve prior evidence. The full scope is
[task-conversation-completion.md](task-conversation-completion.md). This report does
not reduce that scope or declare completion. The [current requirement audit](conversation-completion-current-audit.md)
reconciles later acceptance with historical pending labels. Historical checkpoints in
[the acceptance report](conversation-completion-acceptance.md) are evidence for
their recorded versions, not a description of the current installation.

## Current installation and client discovery

Latest installation: clean signed source `8032172133a6b11fb3898160a451dc8e4dc3eaad`,
binary SHA-256 `faee83dcf72da02779e235b56eab0e7f731cd1af3e43303640860997dad00140`.
[Linux/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37618094550)
passed root/nested test, race, vet, three CGO-free builds and native acceptance.
The exact clean candidate passed local native startup acceptance. A verified
private backup preceded deployment through the unchanged single LaunchAgent;
corpus, identities, tombstones, send/quote records, first-incoming facts,
subscription definitions, schema 14, config and encrypted archives were preserved.
The endpoint bound 1.059 seconds after bootstrap without an additional restart.
Sending remains disabled. All eleven installed skill files match the source.

The installed native [source visibility](contracts/archive-source-visibility.md)
classifies whole-file type-20 exclusions and type-36/status-3 recall targets.
All 56 captured typed conversations were paged to completion through local HTTP:
75 pages, 1,638 source rows, 1,637 examined rows before the agreed cutoff and
1,124 nonempty text records. Exactly 448 records lack a supported text projection,
39 are native exclusions (36 recognized information plus three others), 20 are
source recall controls and six are expired. Counts reconcile to 1,637; there are
no rejected rows or live tombstone omissions in this source/window. The 505
unresolved sender mappings occur in returned records. The 73 unresolved quotes
count metadata gaps in examined rows and may overlap omissions; they are not
an additional disjoint row count. No author or quote content is invented.

The first deployed visibility version `00cb628` exposed a coverage ownership bug:
private page cleanup cleared the returned unsupported-content map. A regression
fails on that version and passes on `8032172`, which copies diagnostic counters
before cleanup. Do not use its initially empty gap map as completeness evidence.
Synthetic owner and actual SDK tests also reject a valid full-text resource claim
for a recalled original whose undo lies outside the requested date interval,
in both direct and group files; visible long-text resources retain their full text.

The actual connected plugin independently read all 56 conversations/75 pages.
Its 1,124 text records, 1,637 examined rows and all omission counters exactly match
local HTTP. Actual client status is connected/authenticated with no error and
58 corpus messages. STDIO negotiates 2025-11-25 with 22 tools, three resource
templates, resources/logging and no Events. HTTP exposes legacy/group and v2;
its single active v2 subscription has an empty, unblocked journal. No ack occurred.
The source bytes/digest, subscription activation boundaries and sending records
remain unchanged; no phone synchronization, history import or Zalo send occurred.

This closes complete bounded reading of the captured local source. It does not
prove complete account history, media rendering, missing sender identities,
strict mobile WAL/import admission or the outstanding incoming-only/first-only
and fresh-send live scenarios. The original goal remains active.

### Revalidated direct-send acknowledgement acceptance

Read-only audit of the separately authorized October 6 trial confirms two saved
sent receipts, identical plain/quote UUID repeats, one corpus record per message
and an exact quoted relationship to the accepted plain ID. The current connected
client independently read both same request/message/recipient identities through
`zalo_get_send_status` after subsequent service restarts. The sending code/contracts
are unchanged from the published checkpoint preceding the trial. Six operations
remain sent and the original ambiguous operation remains unknown. No send,
permission change or automatic repair occurred in this audit.

This closes the corrected plain/quote acknowledgement live gate alongside the
existing encrypted numeric/string wire tests. It does not establish the literal
JSON type used in the live response, incoming/first-only delivery or first sending
to a peer absent from the catalogue/corpus. The older matrix's pending fresh-send
label is historical and superseded by this evidence; no repeat trial is required.

### Previous installation and initial archive visibility

Previous installation: clean signed source `1c0434bd1adb4f4b7916e47bd282bcb4f5f29c02`,
binary SHA-256 `48a074407f892e1c3246d22a74801871837f6ef752d3c061e42f9cc85b335446`.
[Linux/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37610540504),
root/nested test, race, vet, configured CGO-free builds and exact-candidate native
acceptance passed. Verified backup and comparison preserve the corpus, records,
subscriptions, configuration, agent and encrypted archives. Both installed skills
match all eleven source files. Sending remains disabled.

Authenticated public HTTP source discovery and all three catalogue pages returned
56 unique typed conversations from the permanent library. A two-page read of the
requested September 26 interval returned both records, with genuine global IDs,
verified sender mapping and nonempty text. No private text or IDs were printed.
The source digest and encrypted files remained unchanged. No phone request,
corpus import, ack, send or subscription edit occurred. Retention is until owner
deletion, independently of the acquisition cache expiry.

Installed STDIO discovery confirms 22 tools, archive inventory, both source_id
browse arguments and three resource templates; it exposes no Events. Status is
connected/authenticated without errors. The refreshed ChatGPT plugin card confirms archive inventory and updated
source_id descriptions (Read17/Write5). The active connected model subsequently
called archive inventory, all three catalogue pages and both requested historical
message pages successfully, without phone synchronization. Initial HTTP catalogue and STDIO checks failed;
the same service subsequently passed both without another restart. A runtime
sample showed SQLite query-planning activity, but isolated query-plan preparation
on a private backup completed promptly: the latency cause is not established.

The narrow source-wide type-20 informational classification is installed and
verified for this real file. This closes the selected historical archive read,
not whole-account message visibility, strict mobile import, WAL completeness or
remaining live v2/sending gates. Unknown controls continue to block reads rather
than return an unsafe prefix. See [current evidence](contracts/archive-control-taxonomy-research.md).

### Previous first-page readability check

The previous `1c0434b` archive reader was called once for each of the 56 typed
conversations, with a one-record page and an explicit September 1–October 6
UTC window. All 56 requests completed: 47 returned a page, while nine refused
with `SOURCE_CONTROLS_UNCLASSIFIED`, without returning a prefix. The error
responses are text-content envelopes, not structuredContent; the verification
script initially grouped them as unclassified tool errors. The service journal
records exactly 56 corresponding reads, 47 OK and nine control refusals.
The 47 readable files contain 759 source rows and two classified informational
rows; these are whole-file counts, not proof that all 759 texts are visible.
Only first pages were examined, not every projected record. No phone request,
corpus import, send, acknowledgement or subscription change occurred.

### Installed discovery diagnostic

The bounded metadata-only [diagnostic](contracts/mcp-discovery-diagnostics.md)
is installed from the clean signed source above after full state preservation
and native acceptance. The endpoint bound 1.47 seconds after bootstrap without
another restart. Current STDIO discovery confirms 22 tools and connected,
authenticated status without errors; all eleven installed skill files match.
The September 26 public archive read was repeated successfully on this binary,
with unchanged source ciphertext and digest and no phone request or import.

Refresh tools reached this process: at 11:00:54 UTC, server/discover, tools/list
and events/list returned HTTP 200. tools/list returned 22 definitions in 114252
bytes, including archive inventory, with no continuation. Catalogue SHA-256 is
`d007a080496acc61293b799be3240f98d1c7c3e74a58fc487a602047575129a3`.
After reloading the ChatGPT plugin card, it displays Read17/Write5 and the new
archive tool/source_id descriptions. The earlier unchanged card observation
did not prove that backend catalogue refresh had failed; current evidence locates
successful server discovery and visible registration. The active model tool snapshot subsequently exposed the new capabilities.
Authenticated plugin calls returned the permanent source, all 56 unique typed
conversations over three pages, and both nonempty September 26 records over two
ordered pages, with distinct archive identities and genuine Zalo IDs. No record
was quote-anchor eligible. No private text or IDs were printed. This accepts
actual client archive invocation, without explaining prior transport latency.
No tunnel restart, credential change or permission expansion was needed.

### Previous diagnostic installation

Latest diagnostic installation: clean signed source `de80416b1358ba097b622bbb774cc693c63d4033`,
SHA-256 `ac58ec1ddd90c44ba256b7047d919edd7062ffd51c5f4197e6bcbcc200f3d890`.
[Linux/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37600368737)
and exact-candidate local native acceptance passed. Full state/config/agent and
encrypted archive preservation were verified. Owner inspection identified exactly
one type-20 row in the requested file; its format-1 exclusion/visibility semantics
are still open. The native format-1 reader maps undo to 36 and excludes 20 before
its webchat conversion. Desktop MSG_UNDO=20 is not a backup-row classification.
See [control evidence](contracts/archive-control-taxonomy-research.md).
The prior installation evidence below remains historical.

Previous archive-reader installation: clean signed source `3352ae1780123dbe249209ca2d30690342033454`,
binary SHA-256 `4970968063d793fc26851f06a4ad16c1e15564a8866aba52abb013bacdf36a99`.
[Linux/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37598415082)
passed root/nested test, race, vet, CGO-free builds and native acceptance. The exact
clean candidate passed local native startup/shutdown acceptance. Deployment used
the unchanged existing agent with a verified full-state backup. Configuration,
corpus, identities, tombstones, send/quote records, first-incoming facts and
subscription definitions were preserved; schema 14 integrity is valid and sending
remains disabled. Both installed skills match all eleven current source files.

The permanent library retains 54 direct files and two group files until owner
deletion. Actual authenticated public source discovery and catalogue paging
succeeded without another phone request. The requested historical message read
failed explicitly with `SOURCE_CONTROLS_UNCLASSIFIED`; no prefix was returned.
The selected file has 24 rows; older inspection counts only type-33/36 controls
and reports zero, while one sampled kind is unsupported. Identifying the broader
control class and its visibility semantics is the next gate. Synthetic SDK tests
pass but do not establish that real-account message reading works.

After deployment, the actual connected client status call reports connected/authenticated
with no last error. Discovery of the new archive tool in that client is still
unconfirmed. HTTP reports connected/authenticated with no last error, v2 and
legacy group Events and all recovery tools. One active subscription has an empty,
unblocked journal; no ack, send, acquisition or subscription edit was performed.
This does not close remaining live v2/sending gates or prove complete Zalo history.
See [archive read contract](contracts/archive-mcp-reading.md).

### Prior installation checkpoint

The current installed clean source is `3592f3d56f5900a230c8cf0ab6ad310b88328f62`,
with ad hoc signed binary SHA-256
`9ea75d9103645517bcebf77899cfc269032eeba7f225423b39794d71606f68e0`.
Root tests/vet, affected-package race and macOS arm64/Linux amd64 builds passed locally.
[Linux and macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37587570124)
passed root/nested test, race and vet, CGO-free builds and native startup acceptance.
The exact clean candidate (`vcs.modified=false`) also passed isolated native
startup/shutdown acceptance locally. A verified private full-state/binary/config/
LaunchAgent backup preceded replacement through the existing single agent.

Post-deployment comparison preserved all prior messages, identities, tombstones,
send/quote records, first-incoming facts and exact subscription activation fields.
Configuration and LaunchAgent remain byte-identical, sending remains disabled,
and schema 14 passes integrity checks. Local HTTP, installed STDIO and the actual
connected MCP client returned authenticated/connected with no last error.
HTTP exposes legacy/group and conversation v2 plus recovery tools; the one active
v2 subscription has an empty, unblocked journal. STDIO exposes 21 tools and
resources/logging without Events. No diagnostic acknowledgement was sent.

Owner-only account archive capture/status/coverage/remove now use a separate
private encrypted cache. The whole-account scope was explicitly chosen by the
user. The independent encrypted three-file fixture retains two direct files and
one group with colliding typed identities under a restrictive collection policy;
restart and repeated date-window inspection neither redownload nor change corpus,
Events, subscriptions or send records. These routes are not public MCP tools.
Both installed skills still match all five source files each; this owner feature
does not add a public workflow to them. Real account acquisition subsequently retained all 56 mapped files (54 direct,
2 group), with 1,638 source rows. All files are readable and retain WAL markers;
8 contain controls. Two offline date-window sweeps and an identical capture retry
left source ciphertext/expiry unchanged and imported no messages. This closes
retention/repeated offline coverage, not safe mobile admission or historical
message recovery. Exact typed selection subsequently verified the two September 26
records in the requested direct conversation: both are `webchat`, both have
nonempty source text and valid decoded metadata; each has one attachment with
absent action/title, and there are six unsupported nested field occurrences in
total. The earlier converter rejected the absent action as
`unsupported_attachment_action`. Static consumer inspection confirmed the native
MSG_TEXT fallback: absent/empty attachment action keeps MsgContent, with only
exact rtf replacing it with title. The installed fix now classifies both same-source
records as plain text (`plain: 2`). Unknown nested fields remain explicitly counted;
this does not establish full metadata rendering or historical completeness. The selected file
contains 24 source rows and no controls but retains its WAL marker. Inspection
left the encrypted source unchanged and performed no download/import. Unknown
metadata interpretation and independent WAL admission remain open.
The all-file date-window sample inspected 891 source rows; scalar validation
rejected 128 at message ID validation. All 763 remaining rows have valid metadata:
460 plain projections and 303 unsupported projections. Eight files have more
unsampled rows, so this is not complete content coverage. Retained bytes/manifest/
expiry and all runtime state were preserved. Follow-up fixed-key diagnostics established zero global IDs for all 128 sample
rejections. All have positive status, recognized payload kind and valid client ID;
100 have nonempty source text and 28 do not. Remaining validation and visible-text
semantics are not proved by these observations. The native consumer can synthesize
local IDs for missing globals; this does not give our zero IDs genuine upstream
identity or permit using invented IDs in the corpus/quote-send routes.
A [separate snapshot-reading proposal](contracts/retained-archive-reading-proposal.md)
is awaiting a scope decision; it is not an available MCP capability and does not
waive strict import admission. See [retained archive task](task-retained-mobile-archive.md).

## Previous installation and live evidence

The installed clean source is `72d17589fea0fa773ead4c340ced74fa87ba9d49`,
with binary SHA-256
`c2872c54033c51f978dfd47b2117b56505c45247b3e1c8a85b5e195b62c53153`.
[Linux and macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37502905736)
passed root and nested-module test/race/vet, CGO-free builds and native startup
acceptance. The clean native binary also passed isolated startup acceptance
locally before deployment. A verified private full-state/binary/config/agent
backup preceded replacement through the existing single LaunchAgent.

Post-deployment comparisons preserve all prior messages, identities, tombstones,
send operations, quote metadata, first-incoming facts and exact subscription
activation boundaries. Configuration and LaunchAgent are byte-identical; sending
remains disabled and SQLite remains schema 14 with valid integrity. Direct HTTP
and the actual connected MCP client subsequently returned connected/authenticated,
with an empty, unblocked journal for the one active v2 subscription. No
acknowledgement, phone request or message send occurred during deployment.

Startup initially reported reconnecting and a failed group catalogue refresh;
some ten-second local HTTP reads timed out. The same process subsequently
connected without a second restart, and a bounded proxy-free direct check
succeeded. Status still retains `UPSTREAM_UNAVAILABLE` for the group catalogue
refresh; this connected observation does not prove successful catalogue refresh
or explain intermittent HTTP latency. Previous clean source `e7f4d7f` remains
recoverable in the verified private backup.

The existing app-backed Zalo connection was located in Safari and its
**Refresh tools** action invoked. The ChatGPT app detail page subsequently
displayed 5 write tools, 16 read tools, the legacy group event, and conversation
v2. This verifies the visible catalogue, including browse/import/recovery,
but the page does not expose the send input schema or prove that an existing
conversation has reloaded it. Local HTTP `tools/list` returns the corrected
full-string non-whitespace pattern `^[\s\S]*\S[\s\S]*$`.

An earlier saved-request retry was rejected by the action approval reviewer
before MCP invocation and provided no connector schema evidence. Subsequently,
the user explicitly authorized two fresh exact texts to the same verified peer,
one plain and one quoting the first, plus same-UUID retries. Both were accepted
through the installed service's MCP interface with nonempty message IDs; each
same-UUID retry returned its identical saved receipt. The source configuration
was backed up and restored after temporarily enabling only that recipient;
sending is disabled and the collector returned connected/authenticated.

The actual connected client's context read verified the quoted anchor and source
against the exact first message ID. Its retry of the authorized plain request
after permissions were restored passed argument validation and returned the
server's `PERMISSION_DENIED`, rather than the old regex error. This closes the
observed stale send-schema issue without dispatching another message. Fresh
plain/quote live acceptance verifies the corrected acknowledgement path; it does
not establish that Zalo used a numeric JSON ID on this particular response.
Numeric/string wire forms remain covered independently by encrypted HTTP tests.

The separately authorized single mobile archive diagnostic for the historical
September 26 selection reached `waiting_for_confirmation`, then finalized as
`interrupted`. Correlated service records report `TIMEOUT` with dispatch claimed;
they contain no accepted confirmation or offer for this attempt. No archive was
downloaded or imported, and producer count presence could not be tested. This
does not establish whether the phone displayed or accepted the prompt; the
user's phone observation remains pending. The terminal attempt was not retried.

A separately authorized follow-up on October 7 received phone confirmation and
an owned offer, downloaded and inspected the selected archive without import.
The durable attempt ended `offer_ready`, revision 4. The selected file contains
24 rows, including 2 in the requested September 26 interval; both are counted as
unsupported content, with zero text candidates. There are no observed source
controls, invalid/missing metadata or unsupported message types; 6 metadata
fields remain unsupported. Source inspection reports `unverified_wal_snapshot`.
Unsupported content and unverified WAL are independent limitations; clearing
one would not establish that these rows can be safely converted.

The accepted offer logged producer claims `msg_total=1638`, `msg_thread=56`,
with `state=available` and `verification=unverified`. This establishes presence
of the optional fields on a real offer, not their units, scope or consistency
with a selected conversation, and not a producer checkpoint guarantee. No
messages, identities, tombstones or Events were imported by this diagnostic.
An early observation still showed `prepared` and a CLI status read timed out;
subsequent correlated records establish dispatch, confirmation and receipt.
Preparation and a started CLI must not be reported as completed phone dispatch.

The subsequent source candidate adds bounded, fixed-key kind/reason counters
to the owner archive diagnostic. Root tests, affected-package race/vet and
CGO-free macOS arm64/Linux amd64 builds pass. Synthetic checks cover all rejection
categories, private-value redaction, whole-page rejection, unchanged WAL/control
gates, output schema restrictions and the one-shot probe with no persistence.
The later real one-shot repeat exercised those counters: both September 26
records were `webchat` rejected as `unsupported_attachment_action`; the WAL gate
also remained blocked. Raw action values and archive bytes were cleared by that
probe, so these categories do not establish the missing attachment semantics.
The retained account cache is intended to avoid further disposable-only captures.

The remaining full-scope gates are real incoming-only/first-only behaviour,
the separately authorized out-of-catalogue send scenario, and safe real mobile-source
admission and recovery of the user's older Strangers history. Synthetic source
tests and optional producer counts do not establish export completeness.

## Historical installation checkpoints

Source `9f1f0b5db9a562706fbb9ab518ee54a7709d9d9c` passed
[Linux and macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37463304404).
A CGO-free, trimpath macOS arm64 binary built from its clean Git archive was
ad-hoc signed and installed through the existing single LaunchAgent. Installed
SHA-256 is `77fbd286e2cce7a958c372fdb33801a07fc52c6657a8d087f52f5c69cca2f616`;
SQLite is now schema 14. A private native-binary/state/config backup and a
consistent integrity-checked SQLite copy were verified before replacement.
Existing corpus rows, send-operation rows, subscription boundaries and prior
private tokens/keys were preserved. No phone request, send, subscription edit
or journal acknowledgement occurred during deployment. The installed
`probe-recall` route was checked with a verified nonexistent send UUID: it reached
the existing owner socket and returned `RECALL_DIAGNOSTIC_UNAVAILABLE` without
dispatching recall. This accepts command routing, not real undo semantics.

Authenticated local HTTP and the actual connected Codex client both reported
connected/authenticated with no error after deployment. The full-string send-pattern
correction is now installed and visible in local HTTP `tools/list`. A retry of
the original UUID with sending disabled still failed pre-server connector
validation against the old `\S` pattern; the connector has not adopted the
updated schema. The saved sent operation and message identity remained unchanged.
Refreshed client acceptance remains open. The installed STDIO
bridge timed out with a ten-second initialization allowance, then discovered
21 tools with a longer allowance without advertising Events; that later status
reported `stopped` and is not evidence of stable collector readiness. Subsequent
local status requests also timed out, although the persisted collector heartbeat
and ingestion diagnostics continued. Transport/status responsiveness remains
under investigation; a short observation timeout does not prove process failure. Later direct HTTP checks
returned `tools/list` in 30–50 ms and connected/authenticated status in about
180 ms without restarting the service. Proxy configuration was empty and explicit
proxy-free/default clients both succeeded. Those later responses do not explain
the earlier timeouts or establish absence of intermittent latency. A subsequent
ordinary ten-second STDIO check and the actual connected Codex status call both
returned connected/authenticated; the same checks passed after installing the
CLI route correction. Both copied skills match all five source files in
each directory. The subscription journal remained empty and unblocked.

The installed native subprocess acceptance uses isolated synthetic state, a
separate loopback endpoint and temporary HOME. It verifies terminal snapshot
removal before authentication, unchanged terminal operation, no corpus/Event
writes, retained snapshot key/spent identity, endpoint readiness and graceful
shutdown. The test passed under race against the candidate and exact installed
executable; [development instructions](development.md#native-startup-acceptance)
make it reproducible. An initial fixture ended with exit status 1 during startup;
using a short private directory, isolated HOME and waiting for endpoint readiness
corrected the fixture without a service change. That first subprocess result
did not establish a production shutdown defect. Both ad-hoc signatures verify, and removing
signatures from temporary copies confirmed identical executable payloads despite
the path-specific installed signature changing the final file hash.


The actual connected client started an explicit `conversation_preload` import
for September 26 Vietnam time, read its durable status, and retried the original
UUID/arguments. It returned the same operation: `partial/source_window_limited`,
one page/one examined recent record, zero inserted records and
`notification_policy=none`, `history_complete=false`. This accepts the real
client's import/start/status/retry workflow, not deeper direct history recovery.

The already successful archive-reader path cannot have silently discarded a
declared SQLite `-wal` or `-shm` entry: the complete file table is validated before
selection and accepts only canonical `.db` names; a sidecar name rejects the
whole archive before identity mapping. This rules out that specific selection
bug for the accepted transfer, but proves neither producer checkpoint state nor
the interpretation of data outside the declared container. Mobile source input
remains disabled, and the two historical candidates remain unimported. Terminal
snapshot cleanup is implemented and accepted synthetically in source, including
restart after a terminal journal commit. The native startup subprocess check also
passed with the exact installed binary against isolated synthetic state; real
mobile import acceptance remains open.

### Terminal snapshot cleanup and control investigation

Current source adds terminal-image cleanup at service startup and in the mobile
loop. Authenticated snapshot binding is checked against an account-owned terminal
history operation and its permanent acquisition link; revoked collection cannot
prevent cleanup, while active/paused, foreign, changed and unlinked bindings stay
ineligible. Startup cleanup does not borrow a phone/session. Spent UUIDs survive
removal, preventing a retry from renewing the source. Synthetic tests include
restart after terminal commit before removal, journal failure preservation and
shutdown during cleanup. These changes and the subsequent idle-cost optimization
are now installed at the current source checkpoint.

Static format-1 inspection confirms the native names `ChatDelete=33` and `Undo=36`
and their `chat.delete`/`chat.undo` mappings. Its archive importer separates
MSG_UNDO, but the located `_sendUndo` method is empty; it does not establish a
portable target-ID/state rule. The currently retained local tombstone table has
zero rows, so there is no existing live target marker available to correlate with
an archive. The controlled send/recall and before/after archive scenario was authorized.
One disposable message was accepted through local HTTP MCP and retained with live
quote metadata. Its pre-recall phone request waited approximately three minutes
for confirmation and ended `interrupted` without an archive; no recall occurred.
A fresh request is awaiting user readiness. The temporary one-peer send permission
was restored to the exact original disabled configuration. See the
[controlled diagnostic evidence](mobile-backup-diagnostics.md#controlled-diagnostic-evidence-2026-10-06).
Following renewed authorization, a fresh disposable message was accepted once
through local HTTP MCP and its returned ID matches the saved send ledger. The
original disabled send configuration was restored before the archive request.
The new pre-recall attempt also terminated `interrupted` after approximately
three minutes waiting for confirmation (12:55:34–12:58:36 UTC), without an archive.
At that point no real recall, comparison or import had occurred; the next phone
request required explicit user readiness and a fresh attempt UUID.
In the subsequently authorized successful run, both confirmed archives contained
one exact-ID match: type `0`, status `3` before recall, and type `36`, status `3`
after recall. The source file retained 129 rows in both snapshots; whole-file
controls changed from zero to one. The collector persisted the exact direct
tombstone, and the real MCP client no longer returned an anchor. Re-reading the
saved recall returned status `0` without changing its receipt. This closes the
controlled own-direct-message type-36 correlation, not general control parsing:
type `33`, incoming/group controls and WAL completeness remain unverified. Both
probes kept conversion/import blocked. Details are in the
[successful comparison](mobile-backup-diagnostics.md#successful-controlled-recall-comparison-2026-10-06).
The subsequent source classifier privately retains exact own-direct
type-36/status-3 IDs after sender mapping and reports only a diagnostic count.
It is regression-tested, but not yet installed or accepted on a new phone export.
That classifier initially left controls deferred. The later atomic candidate
below handles fully classified own-direct controls; WAL and unclassified
whole-file control gates remain.
After fixing the shutdown/cleanup race and correcting the spent-UUID test to the
existing unavailable-source error contract, root tests/vet and affected
mobile-backup/history-import/service/storage race checks passed. A further real
snapshot/reader/journal synthetic cycle imports one `rtf` title silently, reaches
completed available-source exhaustion with history_complete=false, and removes
the terminal staging file; its targeted race checks passed. CGO-free macOS arm64
and Linux amd64 builds, diff checks and a redacted worktree scan also passed.
These checks use synthetic archives, not new phone exports or model evals.

A subsequent source review found repeated full-image decryption in idle terminal
cleanup. The next change uses bounded authenticated metadata hints only to skip
nonterminal reads; current bytes and journal binding are always rechecked before
removal. Its regression counts actual AEAD opens and corrupts a cached image to
check that a hint cannot authorize deletion; further checks cover a changed
journal decision and expiry after a hint was cached. Affected mobile-backup,
history-import and service race checks, affected vet, CGO-free macOS arm64/Linux
amd64 builds and diff checks passed. This optimization is published and installed
at the current source checkpoint. The preceding cleanup
commit `606ed02` and documentation commit `55299f3` both passed macOS/Linux CI.


Latest mobile checkpoint supersedes the earlier download/format failure notes:
authenticated offer, scoped download, format-1 checksum/XZ, typed selection and
immutable SQLite inspection passed in authorized installed-service probes. The
selected image contains 24 rows: zero in the September 30 interval and two in
the subsequent September 26 interval (Vietnam local days).
Its latest timestamp matches that peer's live corpus. Newly sent test messages
were in another peer, so they do not demonstrate a stale export. A specific
historical omission and uncheckpointed WAL loss have not been established.
WAL-mode images still fail the current conversion gate. The production mobile
port and worker lifecycle are wired and installed at the current checkpoint; public
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

### Rich-text projection and WAL evidence: 2026-10-06

Static inspection of the installed format-1 consumer confirms its MSG_TEXT rule:
one selected attachment with `action=rtf` uses nonempty title, otherwise MsgContent.
The internal converter now follows that visible-text projection and retains an
`rtf` attachment marker, without claiming formatting preservation. Synthetic
native-framed and conversion tests cover differing title, empty/absent fallback,
ambiguous multiple attachments, other actions, unknown tag 6, invalid UTF-8,
oversized text and no mutation of the borrowed source. Other content remains a
reported gap. No real archive was reacquired or imported for this change.

The [SQLite WAL documentation](https://www.sqlite.org/wal.html) confirms that
committed updates can remain outside the main file and that the WAL marker can
persist after checkpoint. Our independent regressions reproduce both cases.
Thus marker 2/2 alone proves neither loss nor up-to-date deletion state. The native
consumer reads selected `.db` files, but its observed read path does not prove the
phone producer's checkpoint/export semantics. That source eligibility gate is
still unresolved; do not label the two September 26 candidates recovered.
Root tests/vet, affected mobile-backup/service/storage race checks and CGO-free
macOS arm64/Linux amd64 builds passed. Added rich-text WAL/control regressions
also passed under race: supported title projection never bypasses source gates.
Redacted worktree scanning found no secrets. The preceding service integration
commit passed [both CI jobs](https://github.com/skosovsky/zl-mcp/actions/runs/37446544187).
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
| Browse without a keyword | Installed source `9f1f0b5` and actual connected-client catalogue→period browse→stable continuation→matching context checks pass; the repeated check preserved snapshot and interval across two distinct pages and matched the exact synthetic send ID/text; production regression tests | Accepted for the current desktop client; missing upstream history remains a separate gate |
| Expanded conversation catalogue | Guarded periodic preload refresh and actual connected-client catalogue reads; source/installed skills match and this desktop discovers both; repeated direct catalogue read found the exact test peer | Accepted for the current desktop client; catalogue completeness is unknown and other intended clients require their own check |
| Explicit silent imports | Durable history journal with UUID/retry/cancel/restart tests; installed preload/group reads; real client start/status/same-UUID retry accepted; production mobile session/worker integration verified synthetically and installed | Real mobile import remains unaccepted; exact installed native cleanup passed isolated startup checks and source completion/restart tests; public mobile input disabled |
| Older Strangers message | Requested peer is selectable; September 30 archive interval is empty and September 26 has two candidate rows; preload only supplies an already retained record | Recovery from a deeper verified source, or a concrete verified source limitation agreed with the user |
| Mobile request/transport | Current-session request/offer observer, durable dispatch ledger, restart recovery and private offer ownership; host-scoped cookie transport, scoped body consumption and revocation tests | Correlated live offer and owner mapping now accepted; download and selected sender mapping accepted by later probes; durable production import remains |
| Mobile archive reading | Independent format-1/XXH32/XZ/OpenSSL and SQLite fixtures; typed file selection, immutable bounded SQLite reads with exact fractional-millisecond interval mapping, digest/request/account-bound keyset paging; one session/download across pages | Format/SQLite/plain-to-session reading accepted; full BinNet/content semantics remain |
| Mobile conversion/import | Plain text and verified single-rtf visible-title projection preserve exact IDs, sender, timestamp, direction and original TTL; atomic page/checkpoint persistence and gap accounting verified synthetically | Real source eligibility and nontext/quote/mention semantics; native cleanup and synthetic text projection do not close full mobile acceptance |
| Deleted/recalled messages | Source schema 11 prevents restoration of an observed exact typed deletion by live replay or history; absent-message deletion, restart, namespace, novelty and rollback tests. Real own-direct recall produced an exact tombstone and client anchor refusal; paired archives correlated the same global ID changing type `0`→`36` while status stayed `3`. Whole-file mobile type-33/36 detection still blocks unverified conversion | Internal own-direct type-36 projection and atomic silent tombstone/checkpoint tests now cover restart and replay suppression; real source acceptance, type-33 and incoming/group semantics plus WAL completeness remain unverified before public archive persistence |
| Expiry after import | Expiry markers, SQL read-time filtering and cleanup; mobile worker uses atomic records/TTL/checkpoint port; rollback/restart/duplicate/cancel/due-expiry tests passed | Quote expiry and trustworthy clock/real archive acceptance |
| Group pages | Installed phase-aware candidate; two live groups each returned a terminal recent page; durable phase traversal covered synthetically | Live old-phase traversal when available; no completeness claim |
| v2 discovery and filters | Installed schema-14 service exposes legacy/group and conversation v2 only; v1 removed; direction/novelty/restart tests; client recovery adoption confirmed by Ann | Agreed incoming-only/first-only live scenarios remain |
| STDIO transport | Exact current installed bridge initialized a real STDIO client at `2025-11-25`, listed 21 tools including recovery and read connected/authenticated status; no Events advertised | STDIO does not replace external HTTP Events acceptance |
| First direct send outside catalogue/corpus | Synthetic authorization/acknowledgement tests and preserved send ledger | A user-selected recipient and separately approved exact send |
| Numeric acknowledgement plain/quote routes | Synthetic encrypted-wire tests; earlier two-send acceptance retained | Separately approved fresh live sends; unknown operations must not be resent |
| Deployment/publication | Signed source `9f1f0b5` passed macOS/Linux CI (`37463304404`) and is installed with verified native/state/config backup; code payload matches the clean-source candidate; exact installed native startup, HTTP/client and STDIO checks passed; schema 14 integrity, preserved corpus/send ledger/subscription boundaries/private keys and both skills verified. Signed documentation checkpoint `143ff90` is pushed and CI `37464427762` passed | Remaining source eligibility and agreed live scenarios; publish further acceptance evidence as those gates close |

### Historical schema-12 installation (superseded)

The then-installed binary used schema 12 and included the plain-owner mapping
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

### Atomic own-direct archive recall candidate

Internal conversion and journal persistence now support the verified own-direct
case atomically, with no historical Events. First-page classification must cover
all controls in the immutable non-WAL source; later or out-of-window controls
remain unsupported before any insertion. Synthetic integrated restart and rollback
checks cover the candidate. Clean commit `d6889c7` is now installed; source
acceptance against a new phone archive remains open. The real paired WAL images
were not imported; public mobile-archive admission remains disabled.

### Native deployment acceptance: atomic archive recall candidate

[Commit d6889c7](https://github.com/skosovsky/zl-mcp/commit/d6889c7f171be98c1315893f68cc4efb9e7172e2)
passed [macOS/Linux CI](https://github.com/skosovsky/zl-mcp/actions/runs/37478227495).
The macOS arm64 binary was rebuilt with CGO disabled and trimpath from the clean
checkout (`vcs.modified=false`) and ad hoc signed. Installed SHA-256:
`9eeb99cd20de8edb22addbbfb4174cd99b9784e34d84a7bdd011054a418160d0`.

A private, verified state/binary/config/LaunchAgent backup preceded replacement.
The existing single direct-Go LaunchAgent and configuration were unchanged.
Schema remains 14 and integrity is valid. Comparing the stopped-service backup
with the running database preserved every prior message, message identity,
tombstone, send operation, quote-send record and first-incoming record, plus all
subscription definitions. Newly received messages may increase the corpus.
Sending remains disabled.

The actual connected MCP client's status returned connected/authenticated without
an error. Local HTTP still publishes v2 plus the legacy group event and the three
recovery tools. The one active subscription had an unblocked journal entry with
payload; this diagnostic sent no acknowledgement. The installed STDIO bridge
negotiated 2025-11-25 with 21 tools, resources/logging, no Events and authenticated
connected status. The actual client refused the exact previously recalled anchor.
No message, subscription or phone request was dispatched for this acceptance.

This proves deployment/preservation/connectivity, not eligibility or completeness
of a real mobile source. WAL/remaining control/content semantics and the broader
live scenarios remain open.

### Whole-source control reader candidate

The [whole-source control scan](contracts/mobile-backup-control-scan.md) now reads
only scalar metadata for type-33/36 rows across an immutable selected file. It
uses 50-row keyset pages, a fixed 5,000-control ceiling and a shared 30-second
budget. Tests classify 52 verified own-direct type-36/status-3 targets spanning
multiple pages and dates outside the message request; source bytes cannot change
during mapping. Unsupported, rejected, duplicate, oversized, cancelled, foreign
or WAL sources return no partial proof. Control text/BinNet are not loaded.

This primitive performs no persistence or live recall and is not yet wired into
the history worker. Required next step is a separately checkpointed control
prelude before ordinary pages, with control progress distinct from requested
message-period counts. The installed first-page policy and public mobile input
remain unchanged; this reader does not close WAL or other control semantics.

### Atomic inactive acquisition release

The documentation-only checkpoint `2f4e90d` exposed a production shutdown race in
[Ubuntu race CI](https://github.com/skosovsky/zl-mcp/actions/runs/37480291226):
`prepared-without-snapshot` observed partial history before a separate worker
cleanup, then cancelled that cleanup and retained an active prepared transfer.
This is not dismissed as a flaky assertion or fixed by adding a test sleep.

History storage now closes an owned active linked acquisition in the same
transaction that publishes a terminal or authentication-paused status. If release
fails, both status and acquisition roll back. Running/queued history remains
untouched; terminal acquisition receipts remain immutable. The separate cleanup
port still repairs pre-upgrade inactive evidence and is idempotent.

Deterministic tests check partial, paused, cancelled, unsupported and failed
transitions without a worker cleanup call, plus rollback at the release step.
The exact failing worker scenario passed 20 race repetitions. Public mobile input
and the WAL source gate remain unchanged. The fix was subsequently installed
from clean commit `39229a9`, with successful macOS/Linux CI and connected HTTP/STDIO
acceptance below.

### Whole-source control prelude integration

The whole-source scanner is now integrated into the snapshot reader and durable
history worker. A positive control count produces a private prelude containing
only verified targets. Storage atomically publishes all tombstones and complete
source proof before ordinary pages. New optional coverage fields separate
`source_recalls` from period-local `own_recalls`; ordinary record/page counters
remain zero during the prelude. Schema stays 14, and legacy checkpoint evidence
retains its old behavior.

A synthetic production-worker cycle commits 52 outside-period recalls, stops
before its first ordinary page, restarts both stores and resumes with the same
digest and expiry. It receives/downloads once, imports one supported rich-text
record, retains all tombstones and emits no Events. Reader/storage tests cover
late controls, missing/forged prelude evidence, invalid targets and transaction
rollback. The source was subsequently installed as `bd93406` below;
acceptance on a fresh phone archive remains open.
WAL, type-33/received/group controls and public mobile admission remain gated.

### Native deployment acceptance: terminal acquisition release

[39229a9](https://github.com/skosovsky/zl-mcp/commit/39229a956c85a38137bb35372dfa247593032c9b)
passed [both CI jobs](https://github.com/skosovsky/zl-mcp/actions/runs/37482211166).
The exact clean checkout produced a no-CGO, trimpath macOS arm64 binary with
`vcs.modified=false`, ad hoc signed and installed SHA-256
`5bb25755612aac2d68a8af357647972e9546a96cfbd4299009ed7176dcd90a21`.
A verified private state/binary/config/LaunchAgent backup preceded replacement.
The configuration and existing single LaunchAgent remained unchanged, schema is
14, integrity valid, and all prior messages/identities/tombstones/send and quote
records/first-incoming evidence and subscription definitions were preserved.
Sending remains disabled.

The actual connected client initially observed reconnecting without an error,
then connected/authenticated without another restart. Local HTTP publishes v2
plus the legacy group event and all recovery tools; one active subscription had
an empty unblocked journal, with no diagnostic acknowledgement. The first cold
STDIO initialization reached its observation timeout; after service readiness,
the repeated bridge negotiated 2025-11-25 and returned 21 tools/resources/logging,
no Events, authenticated connected status. This does not establish the cause of
the intermittent startup latency. No send, phone request or subscription change
was performed for this deployment. The whole-source prelude integration remains
an uninstalled candidate at this checkpoint.


### Native deployment acceptance: whole-source control prelude

[bd93406](https://github.com/skosovsky/zl-mcp/commit/bd93406f6565336420c55ea75dac43c1ed9f8069)
passed [macOS and Linux CI](https://github.com/skosovsky/zl-mcp/actions/runs/37488172364).
A clean no-CGO macOS arm64 build has `vcs.modified=false`; its ad hoc signed,
installed SHA-256 is
`922cad63cd282cda13fc0f141d8b82fb8483f0137238d646d81f1837e03e7ba5`.
The exact staged executable passed isolated native startup/shutdown acceptance
under race. Publication scanning found no secrets in the new commit.

A verified private backup preceded deployment through the same LaunchAgent.
Config, prior corpus/identities/tombstones/send and quote records/first-incoming
facts and subscription definitions were preserved; schema remains 14 and integrity
is valid. Sending remains disabled. Initial HTTP checks found no listener while
the agent was running; the endpoint subsequently appeared without another
restart. This records startup latency, not its cause or guaranteed readiness.

After readiness, local HTTP and the actual connected client reported authenticated,
connected and no error. HTTP discovery publishes v2 and the legacy group event,
with all three recovery tools; the one active subscription had an empty unblocked
journal. The installed STDIO bridge negotiated 2025-11-25, exposed 21 tools plus
resources/logging and no Events, and returned connected/authenticated status.
No sends, phone requests, subscription changes or diagnostic acknowledgements
occurred. The prelude is installed but real mobile admission, WAL completeness
and unsupported control classes remain open; no private archive was imported.

### Producer metadata investigation

The open upstream PR 269 model at `29d01c4` declares two `backup_db` counts and
`is_full_transfer`, but contains no demonstrated producer checkpoint rule.
The source adds strictly bounded, exact-integer count diagnostics after
offer/account validation; [the contract](contracts/mobile-backup-producer-counts.md)
defines all observations as unverified. Synthetic tests check schema conformance,
precision above 2^53, redaction and unchanged offer acceptance when optional
claims are malformed. No new phone request has established these fields on a
real export. The diagnostic source was subsequently installed as `e7f4d7f`
below. A count match cannot prove message/control identity or WAL state.


### Producer count diagnostic deployment

Clean source `e7f4d7f1007c1790943248429e08f79a940610f6` passed
[macOS/Linux CI](https://github.com/skosovsky/zl-mcp/actions/runs/37491624511).
Its no-CGO macOS arm64 build has `vcs.modified=false`; installed ad hoc signed
SHA-256 is `b3d4cde91daa93ae2ce2c275006230253543ad13578e8a7cab3c1cdd7aed595d`.
The exact staged binary passed isolated native startup acceptance under race.
The same LaunchAgent/config were retained with a verified private backup;
corpus, identities, tombstones, send/quote records, first-incoming facts and
subscription definitions survived, with schema 14 and valid integrity.
The TCP endpoint became reachable 9.465 seconds after bootstrap, without another
restart; this observation does not explain all prior startup delays.
HTTP and the actual connected client returned connected/authenticated with no
error. HTTP publishes v2 plus the legacy group event and recovery tools; the
one active subscription's journal was empty/unblocked and no ack was sent.
The initial concurrent STDIO check failed its readiness assertion; after the
confirmed HTTP/client connection, a second check returned 21 tools, resources/logging
and connected/authenticated status without Events. No further restart occurred.
The first assertion does not identify which readiness field differed. Both
installed skills match all five source files each.
Sending remains disabled. No phone request or subscription edit occurred, so
actual producer count presence/semantics, WAL admission and real archive import
remain unverified. Diagnostic installation does not close those criteria.
