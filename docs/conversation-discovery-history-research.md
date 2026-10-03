# Conversation discovery, Strangers and historical browsing

Research date: 2026-10-03. Status: findings and proposed design; no new tools or runtime changes implemented by this investigation.

## Confirmed local findings

The reported direct conversation is absent from the installed SQLite catalogue. Accent-insensitive/case-insensitive checks of both conversation names and stored sender names found no match for the supplied name. This is a missing-data case, not merely an MCP name-search failure. Matching by name cannot establish that an unnamed peer ID is the same person; no target peer ID was available.

At inspection the corpus contained 23 messages: 12 direct and 11 group. The catalogue contained two direct and two group conversations. The earliest retained direct message was dated 2026-10-02 UTC, so the reported September 30 direct message was outside the observed direct corpus. There were 10 live and 13 replay records. These are observation counts, not coverage guarantees.

The collector heartbeat was current, its stored error was null, and the current listener session had decoded 12 replay direct messages. Persistence reported 12 duplicates, zero exclusions and zero errors. This establishes successful processing of the returned replay batch; it does not establish that Zalo returned every conversation. No fresh direct frame was observed during this session's inspected diagnostic window.

The investigation read source, read-only SQLite metadata and aggregate diagnostics. It did not restart the service, open a second Zalo session, request new upstream history, send messages, change subscriptions or friendship state, or mark chats read. Private names, account IDs and message bodies are intentionally omitted here.

## Strangers is not an implemented exclusion

`internal/zalo/events.go` consumes direct and group live/replay messages. `internal/zalo/adapter.go` normalizes direct messages without checking friendship. The pinned listener's command 501 handler also has no friendship filter. Storage applies the shared collection policy; the inspected direct replay batch had no policy exclusions.

Consequently, the screenshot's Strangers category does not prove a filtering bug in our code. It establishes that the native client knows a conversation our local catalogue does not identify. Remaining hypotheses include a limited replay window, a separate upstream inbox/synchronization path, or unavailable peer metadata. They require protocol evidence before assigning a cause.

The native application's existing conversation does not imply that the listener session can retrieve the same historical records. Historical availability and live delivery from a non-friend must be tested separately. Adding the sender as a friend is not an acceptable substitute for Strangers support.

## Catalogue limitations

`zalo_list_conversations` reads only the local `conversations` table. Group metadata populates group entries; received messages discover direct entries. The tool does not refresh a complete upstream inbox or contacts list. It correctly reports `catalog_complete=false`.

The pinned Go API exposes `GetAllFriends`, `GetUserInfo` by known IDs and `FindUser` by phone. Friends are contacts, not all conversations, and phone lookup is not global name search. The reviewed JS API has archived/hidden/pinned conversation subsets; none establishes a full inbox including Strangers. Archived chats are not interchangeable with Strangers.

Recommended directory model: merge supported sources by typed conversation/peer ID while retaining source, last refresh, partial/error status and nullable name. Separate a contact with no observed messages from an observed conversation. Enrich known IDs through supported profile APIs. Search names and aliases locally with Unicode normalization, return ambiguous matches, and never guess an ID from a display name. A complete upstream Strangers/inbox source remains a research dependency.

## Browsing without a search word

`docs/contracts/zalo_search_conversation_messages.input.json` requires a nonempty `query`; `internal/storage/store.go` rejects an empty parsed query and always uses FTS `MATCH`. Sending an empty search or wildcard does not provide supported chronological browsing.

Add an explicit local `zalo_list_conversation_messages` tool. Proposed contract:

- Required typed conversation identity; optional RFC3339 `since`/`until`, documented interval boundaries, order, bounded limit and opaque cursor.
- Stable snapshot pagination, ordered by timestamp plus a deterministic tie-breaker. Cursor must bind filters and order.
- Message IDs, sender, direction, timestamp, excerpt, truncation and full-record resource URI; no mandatory keyword.
- Coverage with requested interval, retained first/last record, known gaps, snapshot time and `history_complete=false` unless independently established. First/last records must not be presented as a continuous covered interval.
- Distinguish unknown conversation, known conversation with no retained data, and no records in the requested interval.

Use the existing resource/context tools for full text and neighbors. Literal search remains useful for narrowing an already identified conversation. This feature can be implemented and verified using existing local data without solving upstream history first.

## Upstream historical recovery

Our listener issues `RequestOldMessages` once for each category after receiving the cipher key. Requests use `first=true`, `lastId=null` and an empty `preIds` list. They have no conversation ID. Returned messages are persisted, but the service does not traverse older pages or expose an on-demand history operation. The reviewed JS listener uses the same request shape. This is available replay, not a proved complete per-conversation history API.

The JS main branch still implements group history through `/api/group/history`. [Issue #367](https://github.com/RFS-ADRENO/zca-js/issues/367) reports HTTP 404; our earlier session investigation also observed group-history failure, as recorded in [conversation-research.md](conversation-research.md).

[Open PR #370](https://github.com/RFS-ADRENO/zca-js/pull/370), head `4eeceafad031ce4f594c4532e3363dbdf01450b0`, proposes `group_cloud_message/api/cm/getrecentv2`, a message cursor, `hasMore` and deduplication. Its author reports a live 120-message check. The PR was unmerged at inspection. It is a concrete candidate for an isolated Go implementation and bounded read-only verification using the existing session; it is not proof that our account can fetch all group history or any direct history. Preserve IDs exactly rather than copying the JS numeric conversion.

No established per-peer direct-history endpoint or complete Strangers catalogue was found in the reviewed Go/JS API inventories. Do not derive a personal endpoint by renaming the group endpoint. Mobile synchronization is a separate, unfinished upstream approach ([PR #269](https://github.com/RFS-ADRENO/zca-js/pull/269)), not a production-ready fallback.

Recommended history contract: explicit bounded operation with stable request ID, typed conversation, requested interval, page/message limits and progress/status. Run through the existing authenticated service and sole listener; persist/deduplicate results transactionally. Record source and server continuation/filtering evidence. Stop on exhausted cursor, repeated cursor, limits or upstream errors. Expose `unsupported`, `partial`, `upstream_unavailable` and obtained record bounds without claiming completeness from an empty page.

Historical import needs an explicit Events decision before implementation. Current semantics deliver a message first persisted after subscription activation even if its Zalo timestamp is older. A new explicit backfill must not silently flood existing subscriptions or turn an old peer into a supposedly new incoming contact. Specify whether historical imports are excluded from ordinary notifications, how first-peer certainty changes, and how existing replay guarantees remain compatible.

## Suggested implementation order and acceptance

1. Define executable browsing and directory contracts; add local chronological browsing and name normalization. Verify dates, pagination, ambiguous names, truncation/resources and gaps on synthetic fixtures.
2. Extend catalogue refresh with supported contacts/profile and conversation-subset sources. Label each source's limitations; verify that contacts without local messages remain discoverable without falsely claiming message coverage.
3. Establish the Strangers protocol path using existing-session diagnostics and a real non-friend conversation. Correlate receive/decode/persist counts and metadata; verify membership is not required. The reported historical case remains unresolved until its peer is identified and accessible history is recovered or an explicit upstream limit is demonstrated.
4. Verify the new group-history candidate with bounded existing-session reads; implement supported paging and coverage. Research direct history separately rather than promising parity before evidence.
5. Before adding a history-import tool, settle notification/first-peer semantics and implement retry, deduplication and restart checks. Update README, conversation/coverage guides, contracts and both repository skills. Distinguish implementation tests from confirmed live coverage; no model evals are needed for this work.

## Primary reference sources

- [JS direct/live/replay listener](https://github.com/RFS-ADRENO/zca-js/blob/dadfef18bcac53537741855c99f084152da1fad9/src/apis/listen.ts).
- [Current group history implementation](https://github.com/RFS-ADRENO/zca-js/blob/dadfef18bcac53537741855c99f084152da1fad9/src/apis/getGroupChatHistory.ts).
- [Archived subset API](https://github.com/RFS-ADRENO/zca-js/blob/dadfef18bcac53537741855c99f084152da1fad9/src/apis/getArchivedChatList.ts).
- [Pinned Go API inventory](https://github.com/amrakk/zcago/blob/d4ff65b460577b2557e70220b68d08ce1f7431b4/apis.go).

