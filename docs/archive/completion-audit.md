# Historical MVP verification

The original MVP verified schema validation, eight MCP tools, full-text resources, allowlist enforcement, SQLite WAL/FTS, Unicode search, bounded responses, signed snapshot cursors, retention and reply context. Synthetic tests covered trusted approvals, token/target binding, idempotency, membership rate limits, crash reconciliation and unknown operation outcomes without repeating mutations.

Authorized live checks confirmed session restoration, collection from selected groups, search/context after restart, Reply decoding, ordinary and moderated membership flows, and preservation of pending operations. A controlled interruption recovered the test messages available through upstream replay. These observations establish specific successful cases, not complete history or an unlimited replay window.

The former split collector/STDIO architecture was replaced by the unified service. Historical normal/race/vet checks and cross-platform builds passed at that stage; later source changes require new checks. Historical model evaluations are summarized [separately](../evals/report.md).

Limits: history completeness is not guaranteed; upstream listener overflow may lose events; natural session expiry and every kick/reconnect path were not exhaustively observed; edits and deletions were not confirmed live. The former architecture is not current installation guidance.
