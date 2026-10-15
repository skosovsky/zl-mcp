# Conversation completion: current requirement audit

## Current update: October 15 live filter acceptance

The [signed local callback acceptance](local-callback-acceptance.md) supersedes
the installation and pending filter/restart labels in the older reports below.
Installed clean code is `754f91056f853c8bc88a73821bef4a58a643e225`, binary SHA-256
`09f82dfdfd0c9f57b349007768bda2ba83501e009751f98a369ecb5f387a666a`.
Its full local checks, exact native startup acceptance and
[Linux/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/38697162754) passed.

| Gate | Current evidence and outcome |
| --- | --- |
| Incoming/outgoing filtering | Known-peer live incoming reached both controls; main-account outgoing reached `all` and created no delivery in `incoming` after source fanout. Signed receiver and actual MCP journals agreed. Accepted. |
| First-only positive and repeat exclusion | Genuine fresh peer was absent from the private pre-message catalogue/corpus/permanent-identity/directory/novelty snapshot. First live message reached all three subscriptions with `first_incoming=true`; repeat reached only `all`/`incoming` with false. Accepted. |
| Restart recovery | Same service restarted between first and second messages. Original generations/boundaries, first fact, exact event/message IDs and one delivery per source were retained. Actual client recovered the unacknowledged first records before ordered test-only ack. Accepted. |
| Test cleanup | Six exact test subscriptions cancelled; two receivers stopped; exact local callback permissions removed. Only original production direct/all subscription remains active, with unchanged definition and processing checkpoint. Connected/authenticated, no error; SQLite integrity passed. Accepted. |
| Strangers classification | New direct peer ingestion and catalogue discovery accepted. Friendship metadata was unknown; no complete upstream Strangers inbox or exact classification is inferred. |

No model evals, extra Zalo sends or phone sync were used for these trials.
Private identities, payloads, signing keys and callback URLs remain outside Git.
Both skills retain their existing valid v2 direction/first-known, journal recovery
and archive boundaries; local receiver configuration adds no agent workflow.

Remaining gates: first sending to a peer absent from catalogue/corpus (the fresh
incoming peer is now known and cannot prove this); strict real mobile import;
unavailable deeper source history and missing metadata as documented below.
Do not repeat accepted plain/quote sends or request another phone capture of the
retained bytes to refresh evidence. The complete task remains open; these filter
results do not silently narrow its scope.

### Remaining import gate: local inspection follow-up

The installed MCP inventory still returned one permanent source with 56 files
(54 direct, two group). A read-only owner CLI coverage sweep using three planned
25-file batches terminated unsuccessfully before returning aggregate evidence;
its failure cause was not established, so no complete inspection is claimed.
A subsequent one-file owner inspection of the same explicit library source
succeeded: the selected file was readable SQLite, marked WAL, with one source
row and no source controls. There were no rows inside the September 1 through
October 6 Vietnam-end-date diagnostic interval. This is selected-file evidence,
not a classification of all 56 files or proof of complete account history.

`internal/mobilebackup/history_page.go` rejects `SourceWAL` before returning
writable history records; `message_conversion.go` also enforces that gate.
The existing `TestSQLiteImmutableMainImageCannotProveWALCompleteness` demonstrates
why integrity success alone does not establish producer checkpoint completeness.
No source gate was relaxed, mobile import enabled, or phone request dispatched.
Current facts do not supply the missing producer/WAL or remaining control proof.
The [SQLite contract](contracts/mobile-backup-sqlite.md) and
[producer diagnostics](contracts/mobile-backup-producer-counts.md) retain those
explicit limitations. Read-only access to retained bytes remains separate.

The known-peer test reset was also reconciled against its original private
backup: 17 original messages and 16 quote tuples were already present; only the
original known first-incoming sequence was restored after a verified backup.
Other corpus/send/subscription/delivery/processing records were unchanged.
See [reconciliation evidence](local-callback-acceptance.md#original-known-peer-state-reconciliation).

## Historical October 7 audit

The [earlier October 15 checkpoint](conversation-completion-2026-10-15.md) records
intermediate publication and preflight results. The October 7 table below is
retained as dated evidence, not current pending status or installation details.

Date: 2026-10-07. The goal remains in progress. This audit follows the full
[task](task-conversation-completion.md), including its earlier direct-send gates.
Historical reports retain their original observations; they are not current
installation descriptions. No model evals, new sends, subscription edits or
phone synchronization were performed for this audit.

## Authoritative checkpoint

Installed clean signed code: `8032172133a6b11fb3898160a451dc8e4dc3eaad`;
SHA-256 `faee83dcf72da02779e235b56eab0e7f731cd1af3e43303640860997dad00140`.
Published documentation checkpoint: `96cd53552d30078c984b2cc5f181b6704a88595f`.
[Code CI](https://github.com/skosovsky/zl-mcp/actions/runs/37618094550) and
[documentation checkpoint CI](https://github.com/skosovsky/zl-mcp/actions/runs/37619475976)
passed on Linux/macOS. Exact-candidate native startup passed locally.
Current-state details and retained source evidence are in
[open gates](conversation-completion-open-gates.md).

## Requirements and evidence

| Requirement | Current evidence | Outcome / missing evidence |
| --- | --- | --- |
| v2 discovery and transport | Actual client adopted v2 and journal recovery; installed HTTP exposes v2 and legacy/group only. v2 schemas and Events implementation are unchanged from the accepted client checkpoint. Installed STDIO returns 22 tools, resources/logging, three resource templates and no Events. | Discovery accepted; HTTP Events and STDIO capabilities remain distinct. |
| Incoming-only delivery | Storage/Events/full-service TLS tests cover direction. Read-only inspection finds one active real v2 subscription with `direction=all`; no saved subscription uses incoming-only. | Real filtered incoming/outgoing delivery remains pending; the all-direction subscription cannot prove it. |
| First incoming, repeat and restart | Permanent novelty facts and direction/novelty/restart tests are preserved. Every saved subscription has `first_incoming_only=false`. | Real first-only delivery remains pending; synthetic results do not replace that scenario. |
| First send outside catalogue/corpus | Synthetic exact-recipient authorization and acknowledgement tests pass. | Requires a user-selected recipient absent from the catalogue/corpus and approval of the exact send plan. Do not manufacture a contact or choose a stranger automatically. |
| Corrected plain/quote acknowledgements | Separately authorized fresh trial on October 6 produced two `sent` receipts with IDs. Both repeated UUIDs returned identical receipts. Each message has one corpus match; the quote points to the exact accepted plain ID. Encrypted HTTP tests cover integer/string response forms. | Corrected route acceptance is accepted. The literal JSON type of that live upstream response was not captured; do not claim it was numeric. No further send is required for this gate. |
| Send persistence after restart | Current SQLite and actual connected `zalo_get_send_status` both return the same two accepted request/message/recipient identities after subsequent deployments. Sending code/contracts are unchanged from the published checkpoint preceding the trial. Six operations remain sent; the original ambiguous operation remains unknown. | Saved-result restoration accepted. No new send, retry or retrospective repair of unknown occurred. |
| Callback recovery and processing | User confirmed agent notifications and journal adoption. Persistent journal/ordered ack/restart/duplicate tests pass; current single active v2 journal is empty and unblocked. | Recorded recovery is accepted; notification-before-ack crashes can still produce duplicates, as documented. This does not close first-only/incoming-only delivery. |
| Internet-first Strangers research | Official product guidance and pinned primary upstream code were researched before the native/protocol investigation; citations and dates are in [research](conversation-discovery-history-research.md). Listener/replay does not filter by friendship. | Research/order accepted. Outside-catalogue and non-friend are distinct observations; no complete Strangers inbox API is claimed. |
| Expanded catalogue and live direct ingestion | Supported contacts/preload/replay sources merge typed metadata with provenance and partial coverage; actual client finds the formerly missing peer. User confirmed messages from a previously unknown sender. | Implemented and observed paths accepted; complete inbox/name discovery and friendship inferred from names are not promised. |
| Historical user case | The selected September 26 interval returns both same-source records with genuine IDs and verified sender mapping through the actual client. Earlier September 30 reads found no records in that source. | September 26 recovery accepted. The absent September 30 records and completeness of upstream history remain explicit source limitations, not proof of absence from Zalo. |
| Browse without keywords | Corpus and retained-source tools provide explicit date windows, stable examined-row continuation, bounded Unicode excerpts, full-text resources, ownership/policy/TTL checks and exact identities. Actual plugin independently completes all 56 retained conversations over 75 pages. | Accepted for available sources. Archive rows remain separate from corpus identities and cannot be quote/send anchors. |
| Bounded available history operations | Durable preload/group operations have executable input/output contracts, UUID conflicts/retries, cancellation/progress/checkpoints, ownership and restart tests; current client start/status/retry and bounded group reads were accepted. Unsupported direct-history methods do not silently fall back to a group API. | Available paths accepted; nonempty older group phases and deeper direct history are not established by terminal recent pages. |
| Permanent whole-account captured source | All 56 files are retained outside iCloud until owner deletion, independently of capture-cache expiry. Local HTTP and actual plugin agree on 1,637 examined rows, 1,124 text projections and reconciled omissions; immutable ciphertext is unchanged. | Complete reading of the captured source accepted. This is not complete Zalo history, media rendering or resolved author/quote metadata. No new phone request is needed for these bytes. |
| Silent historical import and real mobile admission | Atomic identity/novelty/TTL/checkpoint and Events-suppression tests pass; bounded public preload/group imports preserve semantics. Strict mobile WAL/control/producer-proof admission stays gated; no real mobile source was imported. | Real mobile import remains unaccepted. Read-only archive acceptance does not waive this requirement or enable the public mobile input. |
| Best practices, docs and skills | Embedded schemas, bounded tool/resource results, stable safe errors, annotations, deterministic discovery and untrusted-content boundaries have tests. Both installed skills match all eleven source files; structural validation passes. Current acceptance distinguishes historical evidence and unavailable capabilities. | Implemented scope accepted; reports are being reconciled against this audit. No model eval claim. |
| One Go owner, preservation and publication | Clean no-CGO build uses the existing single LaunchAgent. Verified backups preserve session/corpus/identities/quotes/novelty/send ledger/subscription boundaries, config, schema 14 and encrypted source. Collector/client are authenticated/connected. Signed commits, clean checkout, secret scans, local links and Linux/macOS root/nested test/race/vet/build/startup CI passed. | Current delivery accepted; future gate-closing changes require the same checks. |

## Remaining scope

The [remaining live plan](conversation-completion-live-plan.md) defines exact
temporary scopes, source controls, restart evidence and cleanup before approval.
Do not repeat the accepted two-message trial or historical phone captures merely
to refresh a report. The pending recipient question concerns a different gate:
first sending to a peer absent from the catalogue/corpus. No such peer is yet
selected or authorized. Temporary live v2 tests require agreed scopes and must
preserve the existing production subscription's activation boundary.

Real mobile import, deeper unavailable history and complete upstream inbox/history
proof are not established by a readable snapshot or matching row counts. Any
reduction of the original result requires agreement with the user; this audit
does not mark the goal complete.
