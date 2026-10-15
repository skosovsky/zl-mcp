# Conversation completion: final requirement audit

Date: October 15, 2026. This audit covers the full
[conversation task](task-conversation-completion.md), its preserved
[direct-message requirements](task-direct-messaging.md), and the
[retained archive task](task-retained-mobile-archive.md). It separates verified
behavior from unavailable upstream capabilities. No model evals were run.

## Scope and archive role

The user clarified that the archive was retained for convenient repeatable
testing. The retained-archive task already separates offline reading from strict
corpus import: its requirements 6 and 9 explicitly say that retaining/reading a
source does not admit an import. Its practical result is a permanent local copy
that can be examined repeatedly without asking the phone to synchronize again.

That result is implemented and verified. Archive reading is an explicit separate
source, not ordinary corpus search or an outgoing quote anchor. Strict mobile
WAL/control import remains disabled and unaccepted. Treating it as a mandatory
additional completion gate for this read/test result was an audit mistake; no
unavailable capability is being relabelled as implemented. Available silent
history import is independently implemented for group-cloud and explicit preload
sources, with the bounded coverage described below.

## Requirement-by-requirement evidence

| Task requirement | Authoritative evidence | Verified outcome / boundary |
| --- | --- | --- |
| Internet-first Strangers investigation, then code/protocol | [Dated primary-source/native research](conversation-discovery-history-research.md), directory/collector implementation, [fresh-peer live evidence](local-callback-acceptance.md#fresh-account-first-incoming-accepted) | Friendship is not a listener exclusion. Accessible direct live/replay/preload paths discover peers and messages; a genuinely absent local peer was discovered and persisted. No complete upstream Strangers inbox or friendship inference is promised. |
| v2 discovery, compatible scopes and filtered delivery | Installed HTTP discovery plus [signed independent live receiver](local-callback-acceptance.md); schema/Events tests in the successful CI below | Legacy group and conversation v2 are published. Incoming reached both controls; outgoing was excluded from incoming-only after source fanout. Exact/all/direct/group scope compatibility remains covered by executable contracts and service tests. Conversation v1 is removed. |
| First incoming, repeat, replay and restart | [Fresh first/repeat/restart trial](local-callback-acceptance.md#fresh-account-restart-recovery-accepted-second-incoming-pending), permanent novelty/concurrency/migration tests | First event reached all three scopes with true; same peer's second event after restart reached all/incoming with false, with no first-only delivery. IDs, generation/boundaries and known fact survived restart. Unknown/null is never positive novelty. |
| Plain/quote send, acknowledgement, retry, recovery and local absence | [Existing plain/quote acceptance](direct-messaging-acceptance.md), [approved local-absence send](local-callback-acceptance.md#approved-local-absence-send-accepted) | Corrected plain/quote routes accepted with exact IDs and same-UUID receipts. Separately approved artificial local absence was verified through two actual MCP catalogue pages plus SQL before one accepted plain send; retry/restart returned the same receipt. This is not first upstream acquaintance. Ambiguous old outcomes were not rewritten. |
| Browse without a keyword | [Executable browsing contract](contracts/conversation-browsing.md), [boundary/snapshot test](../internal/storage/browse_test.go), actual corpus and archive client reads | Typed identity, RFC3339 half-open bounds, stable pagination/ties, Unicode excerpts/resources, namespace/policy separation and distinct empty reasons are implemented. A current actual-client date browse returned the restored two test messages plus their accepted outgoing record. |
| Expanded catalogue and useful metadata | [Directory contract](contracts/conversation-directory.md), [metadata/ownership tests](../internal/storage/contacts_test.go), preload catalogue tests and actual fresh-peer discovery | Contacts, discovered conversations and preload observations merge by typed ID with provenance/aliases. Unicode candidate matching does not identify a person. Completeness and friendship remain unknown unless a source establishes them. |
| Bounded available history, durable status, cancellation and exact coverage | [Import contract](contracts/history-import-operation.md), [atomic rollback test](../internal/storage/history_operations_test.go), [worker assertions](../internal/historyimport/worker_test.go), retained live operation journal | Public sources are group_cloud and explicit conversation_preload. UUID conflicts, revisions, reservations, cancellation, restart and cursor/phase checks are covered. Preload always reports partial/source_window_limited and cannot page deeper. Live journal contains three completed group pages with duplicates, three bounded preload results, and explicit source/invalid-page failures; no positive new historical-record live insertion is claimed from those results. Synthetic source tests prove new-record persistence and no Events. Nonempty older group traversal and deeper direct history are not claimed. |
| Contract-first history Events policy | Embedded input/output schemas, history journal/page transactions and [silent preload regression](../internal/historyimport/worker_test.go) | notification_policy=none is immutable. Atomic rollback leaves no message/identity/event/checkpoint prefix; committed historical identities suppress later live duplicates. Existing novelty remains known; previously unknown historical peers do not become first-only matches. |
| Actual historical date case and persistent archive | [Retained archive acceptance](task-retained-mobile-archive.md), [MCP archive contract](contracts/archive-mcp-reading.md) | The corrected September 26 case returned its two same-source records through the actual client. All 56 mapped conversations were independently paged to termination: 75 pages, 1,637 examined records and 1,124 text projections, with reconciled exclusions/recalls/expiry and explicit metadata gaps. This is complete reading of the captured source, not complete Zalo history. |
| Permanent source requirements 1–10 | [Archive task and acceptance](task-retained-mobile-archive.md), authenticated archive contracts/tests, final inventory and ciphertext comparison | Account/source/digest binding, encrypted private storage outside iCloud, atomic publication, independent until-owner-deletion retention, bounded source-bound cursors/resources and explicit gaps are implemented. Final inventory returns one permanent source, 54 direct and two group files. Stored source files match the stopped verified backup. Reads require no new acquisition, corpus import, Events or send authority; archive rows cannot be quote anchors. |
| One Go service, preservation and native deployment | [Deployment evidence](local-callback-acceptance.md#installed-backend-and-independent-live-preflight), final binary/config/agent and SQLite checks | One LaunchAgent directly starts Go service. Exact installed clean code is 754f910, SHA-256 below, with a valid ad hoc signature. Code/contracts/dependencies/skills are unchanged by subsequent documentation commits. Prior corpus/identities/quotes/novelty/send/subscription records were verified preserved; temporary approved removals were restored exactly. Config is 0600 and state directory 0700, outside iCloud. No Python daemon or second listener was installed. |
| Actual HTTP/STDIO client capabilities | Final read-only probe of installed serve bridge and HTTP endpoint; actual connected client status/browse/send-status/archive inventory | Both transports expose identical 22 tool descriptions/input-output schemas/annotations, four listed resources and three resource templates. STDIO negotiates 2025-11-25 and has no Events; HTTP exposes v2 and legacy group Events. Both return authenticated/connected with no error. Probe bridge exited; it did not own a collector. |
| MCP/skills best practices and installed copies | [Best-practice requirements](task-conversation-completion.md#4-применение-лучших-практик-mcp-и-agent-skills), executable schemas/service tests, [skill guide](skills.md), final installed-file comparison | Flat bounded interfaces, ownership, stable errors, resources for long text, source coverage and untrusted-content boundaries are enforced. Research remains read-only; Events uses the exact subscription journal and ordered ack. All 11 installed skill files match source; frontmatter and 11 relative links validate. No historical/model eval is presented as proof of the updated workflow. |
| Cleanup, safe default permissions and private evidence | [Cleanup/reconciliation evidence](local-callback-acceptance.md#completed-test-cleanup), final config, subscription and receipt checks | Six exact local subscriptions cancelled, two receivers stopped, callback allowlist empty. Production subscription definition/boundary and processing records preserved. Original known-peer reset was reconciled. Two fresh-peer test records/quotes were restored after the approved send; sending is disabled. Private IDs, UUIDs, bodies, keys and backups remain outside Git. |
| Publication, commands and CI | Clean signed public commits, tree/range secret scans, changed-document link checks, [code CI](https://github.com/skosovsky/zl-mcp/actions/runs/38717383462) | Root/harness/patched-zcago test/race/vet, lint, architecture and negative controls, offline synthetic checks, CGO-free darwin arm64/amd64 and linux amd64 builds, and native startup/shutdown acceptance passed on macOS/Linux. The final documentation publication must retain a verified signature, clean checkout and successful CI for its exact commit. |

Installed binary SHA-256:
`09f82dfdfd0c9f57b349007768bda2ba83501e009751f98a369ecb5f387a666a`.
Clean installed source: `754f91056f853c8bc88a73821bef4a58a643e225`.
Full local checks and exact-candidate native startup acceptance are also recorded
in [local acceptance](local-callback-acceptance.md); no unchanged production test
was repeated solely to generate a new historical report.

## Limitations retained in the delivered result

- No complete inbox, full Zalo account history or continuous date coverage is
  promised. Empty results, source exhaustion and connected state do not prove it.
- Deeper direct history, nonempty older group phases, unsupported archive media,
  missing sender/quote mapping, unknown controls and producer WAL completeness
  remain explicit source limits. Archive resources are read-only evidence.
- Strict mobile corpus import is not publicly enabled. Reading or testing retained
  bytes does not waive its separate admission criteria, create send anchors or
  run phone synchronization.
- Durable HTTP callback acceptance is not model processing. Journal recovery and
  ordered ack support missing payloads; notification-before-ack crashes can still
  duplicate a notice. The installed production automation is preserved.
- First known means first locally evidenced incoming. The approved artificial
  local-absence send does not establish first contact in upstream Zalo.

These are boundaries of the implemented available-source result, not features
silently marked as successful. No new phone request, outgoing approval or model
eval is needed to repeat the delivered read-only archive tests.
