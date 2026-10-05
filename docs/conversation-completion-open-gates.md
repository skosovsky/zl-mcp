# Conversation completion: current open gates

As of 2026-10-05. The full scope is
[task-conversation-completion.md](task-conversation-completion.md). This report does
not reduce that scope or declare completion. Historical checkpoints in
[the acceptance report](conversation-completion-acceptance.md) are evidence for
their recorded versions, not a description of the current installation.

| Requirement | Current evidence | Remaining gate |
| --- | --- | --- |
| Browse without a keyword | Current installed HTTP and actual connected-client catalogue→period browse→stable continuation→matching context checks pass; production regression tests | Final signed checkout/publication verification |
| Expanded conversation catalogue | Guarded periodic preload refresh and actual connected-client catalogue reads; source/installed skills match and this desktop discovers both | Final deployment/publication consistency and other intended clients |
| Explicit silent imports | Durable history journal with UUID/retry/cancel/restart tests; installed direct preload/group reads; synthetic archive-to-record-to-silent-storage callback retains text without Events | Three import tools are absent from the external client's catalogue; production mobile importer is not wired |
| Older Strangers message | Requested peer is selectable; requested older interval is empty; preload only supplies an already retained record | Recovery from a deeper verified source, or a concrete verified source limitation agreed with the user |
| Mobile request/transport | Current-session request/offer observer, durable dispatch ledger, restart recovery and private offer ownership; host-scoped cookie transport, scoped body consumption and revocation tests | Independently verified production download hosts, approved phone request and actual encrypted archive/identity compatibility |
| Mobile archive reading | Independent format-1/XXH32/XZ/OpenSSL and SQLite fixtures; typed file selection, immutable bounded SQLite reads with exact fractional-millisecond interval mapping, digest/request/account-bound keyset paging; one session/download across pages | Real format/SQLite/BinNet/plain-to-session mapping acceptance |
| Mobile conversion/import | Supported plain-text candidate conversion preserves exact IDs, sender, timestamp, direction and original TTL; explicit unsupported rich-text/nontext and quote/mention counts | Nontext/attachment/quote/mention semantics and atomic durable page/checkpoint persistence; text-only support is not the final feature |
| Deleted/recalled messages | Source schema 11 prevents restoration of an observed exact typed deletion by live replay or history; absent-message deletion, restart, namespace, novelty and rollback tests. Whole-file mobile type-33/36 detection blocks unverified conversion | Mobile control target/state semantics and full handling before public archive persistence; live marker receipt alone does not establish mobile compatibility |
| Expiry after import | Source schema 10 adds atomic history/deadline writes, SQL read-time filtering, maintenance cleanup and irreversible expiry markers; replay/reimport and rollback tests | Wire into the mobile importer, quote expiry and trustworthy clock/real archive acceptance |
| Group pages | Installed phase-aware candidate; two live groups each returned a terminal recent page; durable phase traversal covered synthetically | Live old-phase traversal when available; no completeness claim |
| v2 discovery and filters | Local Events list has legacy/v1/v2; direction/novelty/restart tests | Actual client v2 discovery and agreed incoming-only/first-only live scenarios |
| STDIO transport | Current installed bridge initialized a real STDIO client at `2025-11-25`, listed 18 tools and read connected status; no Events advertised | Final publication consistency; STDIO does not replace HTTP Events acceptance |
| First direct send outside catalogue/corpus | Synthetic authorization/acknowledgement tests and preserved send ledger | A user-selected recipient and separately approved exact send |
| Numeric acknowledgement plain/quote routes | Synthetic encrypted-wire tests; earlier two-send acceptance retained | Separately approved fresh live sends; unknown operations must not be resent |
| Deployment/publication | Root/nested race and vet, exact isolated-source builds and secret scan passed. Checked schema-11 candidate installed through the existing agent with a verified cold full-state backup; local and connected-client status authenticated/connected; preserved records verified | Remaining feature/client/live gates, SSH signature accepted by Secretive, signed commits/push and new CI |

The installed binary now uses schema 11. Experimental mobile ports are present
but have no public archive dispatch/import route and no real-archive acceptance.
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
