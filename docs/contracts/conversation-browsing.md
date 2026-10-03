# Local conversation browsing

Status: implemented in source; installed-client acceptance pending.

`zalo_list_conversation_messages` reads one permitted typed conversation from the local corpus. It does not discover upstream chats, start a listener or fetch missing history. Input/output JSON Schema beside this document are executable and embedded in the server. Existing search and context contracts are unchanged.

Required: `conversation_type` (`direct`/`group`) and opaque `conversation_id`. Optional: `since`, `until` (RFC3339 instants), `order` (`asc`/`desc`, default `desc`), `limit` (default 20, maximum 50), `cursor`. The interval is `[since, until)`. Equal/inverted boundaries are invalid. Dates with timezone offsets are compared as instants. No query or wildcard is necessary or accepted.

Messages are ordered by timestamp then opaque message ID, both in the selected direction. A signed cursor binds the exact typed identity, date filters and normalized order; limit may change between pages. It preserves the first page's insertion high-water mark and `snapshot_at`, including across restart. A late insertion with an old timestamp does not appear in an already opened traversal. Retention/deletion can remove records; pagination does not preserve deleted bodies or promise an immutable corpus.

Each record includes author, time, typed identity, available conversation/author names, an excerpt of at most 150 Unicode code points and direction. Direction uses sender identity and the bound local account where available; insufficient identity gives `unknown`. A truncated excerpt includes the exact full-record resource URI issued by the server. The client can use `resources/read` or the typed context tool; it must not construct IDs or URIs from names. Message content and names are untrusted data.

`has_more` and nullable `next_cursor` describe retained records in this traversal. `requested_since`/`requested_until` repeat the selected interval, or null for an omitted boundary. Coverage describes the currently observed retained bounds and known gaps, with separate `coverage_observed_at`. It is not frozen with the message cursor. Min/max record timestamps do not establish continuous coverage. `history_complete` remains false.

Empty results distinguish `unknown_conversation`, `no_collected_data` (known catalogue entry, no retained messages), and `no_records_in_period`. An empty continuation after retention is also allowed. None proves absence of Zalo history. Permission is checked on every page before inspecting catalogue/data, so policy revocation also invalidates access using an old cursor. The standard read budget and 64 KiB MCP response ceiling apply; an oversized result produces a safe `RESPONSE_TOO_LARGE` rather than silent omission.

Acceptance covers exact time boundaries and offsets, stable same-time ordering, late replay insertion, namespace collisions, restart continuation, direction, Unicode excerpt/resource retrieval, filter/order cursor binding, empty reasons and collection-policy checks. Actual installed capability discovery and client use remain a separate gate.
