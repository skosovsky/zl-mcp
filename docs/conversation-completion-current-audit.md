# Conversation completion: current requirement audit

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

Do not repeat the accepted two-message trial or historical phone captures merely
to refresh a report. The pending recipient question concerns a different gate:
first sending to a peer absent from the catalogue/corpus. No such peer is yet
selected or authorized. Temporary live v2 tests require agreed scopes and must
preserve the existing production subscription's activation boundary.

Real mobile import, deeper unavailable history and complete upstream inbox/history
proof are not established by a readable snapshot or matching row counts. Any
reduction of the original result requires agreement with the user; this audit
does not mark the goal complete.
