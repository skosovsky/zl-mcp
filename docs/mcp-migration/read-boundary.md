# ADR: typed search and context read boundary (T04)

Status: contract fixed before implementation. Public executable schemas in
`docs/contracts` remain the authority; this ADR describes internal Go composition.

The first slice contains `zalo_search_messages`,
`zalo_search_conversation_messages`, `zalo_get_message_context`,
`zalo_get_conversation_message_context` and the full-text group/conversation
resource templates. `internal/app/search` owns typed search orchestration and
coverage/empty-reason assembly. `internal/app/conversations` owns typed context
and full-message requests. Consumer-owned ports expose only those reads; they
accept domain.ConversationRef and opaque string message/cursor IDs. There is no
SDK/SQL/storage type or universal map in a new application API.

Search returns an invocation-owned typed page with hits, next cursor, snapshot
and a narrow in-memory shortening capability. `internal/adapters/store` owns the
native storage.SearchPage and its private continuation; shrinking delegates to
existing Store.ShortenPage and never reruns FTS. Coverage and summaries are read
again after truncation exactly as before. Group/conversation DTOs preserve their
separate wire identities, order, null/empty arrays, unknown completeness and gaps.

Context read adapters reuse existing Store.Context/ConversationContext and their
20KiB clipping/neighbor/reply semantics. Group results are converted directly from
the existing typed storage values; legacy map conversion is confined to adapters.
Shared typed conversation message/coverage/context DTOs live in the explicitly
bounded `internal/app/shared` package, depend only on domain/standard values and
contain no transport implementation. Full resource reads call the same consumer
ports and current policy, with the existing1MiB cap and URI grammar owned by MCP.
The64KiB SDK result budget/output validation/resource links remain wire concerns.

Composition supplies read ports explicitly to the internal MCP constructors.
Service and evaluation fixture build one borrowed store adapter and application
handlers. Existing construction callsites/tests in this Go module are updated
atomically; these are internal APIs, not public external Go library exports.
MCP server code imports app ports/handlers and never the concrete read adapter.
Store/session/workers and shutdown retain their existing single owners. Adapter
errors translate sql.ErrNoRows to the existing safe domain NOT_FOUND response;
other underlying causes remain private to the existing error projection.

Other reads, commands, admission and Events keep their current baseline paths and
existing exact architecture exceptions until T06/T07. No new backend exception is
introduced. Source selection remains an explicit read contract: omitted source_id
uses the current corpus; a nonempty source_id selects an account-bound permanent
archive via the existing ControlPort, mandatory date window/current policy and
archive row identities. Search/context remain current-corpus operations under the
existing schemas. T06 supplies archive-specific typed handlers/ports; no archive
fallback, implicit synchronization/import or outgoing quote identity is added here.

The T01 original business sources and T03 evaluation overlay are frozen separately.
Baseline rebuilding uses the accepted T03 overlay commit; later candidate composition
must not be copied into baseline. Baseline and candidate still receive independent
copies of the same versioned synthetic fixture. No performance comparison is claimed
in T04; the accepted T03 measurements remain the baseline for T09.

Acceptance: leaf requirements in evidence/T04/requirements.json, relevant existing
storage/contracts/MCP HTTP/STDIO/resource tests, explicit application/read-port and
FTS trim counters, offline suite and architecture100/100. Both independent reviews
must accept the same source digest before the sole T04 commit.
