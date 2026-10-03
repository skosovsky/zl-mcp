# Available replay continuation

Status: contract fixed before implementation; live acceptance pending.

The installed native-client static code continues an offline message queue when the response `data.more` is true, using `data.lastActionId` and `first=false`. This cursor is an **action ID**, not a message ID. This is available queue replay, not arbitrary per-chat historical retrieval or a complete archive.

The dependency must retain optional continuation metadata for commands 510/511: the originating queue type, nullable `more`, exact decimal `lastActionId` and page message count. Accept `more` as boolean or integer 0/1, and the cursor as a decimal string or exact integer without float conversion. Missing/invalid metadata does not cause loss of otherwise valid messages: it prevents automatic continuation and yields a safe diagnostic reason. Homogeneous batches from a mixed response carry continuation once, after all messages in that response.

The existing `RequestOldMessages` first-request method remains compatible. An optional `RequestReplayPage` extension permits `first=false` for a continuation, preserves request-ID allocation and uses the existing socket. The service starts the current first request for each enabled queue, persists every returned message before requesting its next page, and continues only when `more=true` and a valid unseen cursor is present. An empty page with a progressing cursor is allowed.

Limits per queue per connection: 100 response pages or 10,000 observed messages before stopping further requests. All records already received in a valid final page are persisted even if that page reaches the message limit. Repeated/missing cursors, invalid metadata, limits or unavailable request support stop continuation with bounded diagnostics. No raw IDs, messages or credentials appear in diagnostics. No retry loop is introduced for unsupported continuation. Reconnect starts the first request again; permanent message identities deduplicate records and preserve the subscription insertion boundary.

Queue exhaustion does not close a historical coverage gap or set `history_complete=true`. Existing replay source and Events semantics are unchanged: a record first inserted after activation may notify even when `sent_at` is old. This change does not introduce an explicit history import tool. Historical import policy remains a separate contract decision before implementation.
