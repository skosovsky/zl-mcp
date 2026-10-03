# Conversation ingestion: implementation evidence

Date: 2026-10-03. This is evidence for [the all-conversations task](task-all-conversations.md), not a completion report.

## Confirmed protocol behaviour

A bounded diagnostic using the existing authenticated session requested direct-message replay (command 510) after receiving the WebSocket cipher key. Both `lastId=null` and `lastId="0"` returned nine direct messages in a valid `data.msgs` array. Three entries matched the user-provided screenshot phrases, including the recent entry at the reported time. Counts and matching flags were inspected without printing message bodies, peer IDs or credentials. The response remains in a private temporary file, outside the repository; it is not yet inserted into the production corpus.

The same diagnostic earlier requested group replay (511); valid arrays were empty. A separate group history request returned API error 404 while group metadata remained accessible. Direct replay therefore provides a recovery path for the observed incident; it does not prove a complete historical archive, a stable retention window or pagination.

The diagnostic temporarily stopped the sole service listener and restored its LaunchAgent in a `finally` block. No subscription, account membership, message sending or authentication revocation occurred.

## Catalogue discovery

The pinned Go client exposes group and friend catalogues, but no complete conversation catalogue or direct history endpoint. The reviewed JS API inventory contains group history, pinned/hidden/archived conversation subsets and friend/group lists; these do not establish a complete personal-chat list. Use group catalogue plus event/replay discovery, enrich peer metadata through supported profile APIs, and expose direct catalogue completeness as unknown/partial. Do not equate friends with conversations.

## Source versions and fixes

- Go upstream commit: `d4ff65b460577b2557e70220b68d08ce1f7431b4` (pinned local dependency plus documented patches).
- JS reference commit: `dadfef18bcac53537741855c99f084152da1fad9`.
- [Reference listener](https://github.com/RFS-ADRENO/zca-js/blob/dadfef18bcac53537741855c99f084152da1fad9/src/apis/listen.ts).
- [Reference group history](https://github.com/RFS-ADRENO/zca-js/blob/dadfef18bcac53537741855c99f084152da1fad9/src/apis/getGroupChatHistory.ts).

The local direct-message union decoder silently swallowed decoding failures and retained stale fields when reused. It now clears its state and returns failures. Mixed replay previously discarded `msgs` whenever `groupMsgs` was nonempty; it now emits both homogeneous batches, preserving existing batch type semantics. Synthetic regression tests pass under the race detector. These fixes are not yet deployed as all-conversation support.

## Remaining verification

- Persist recovered direct messages through the new typed storage path and retrieve them via MCP.
- Verify normalization of incoming/self-sent messages and both deletion types.
- Verify catalogue expansion, coverage and full-text retrieval after restart.
- Migrate subscriptions and pending payload without widening their scopes.
- Verify the new Events profile and deployment independently of replay success.

## Implementation checkpoint

The first implementation stage adds a typed `ConversationRef`, a shared collection policy, config validation for explicit `all`/`selected` modes, and direct-message normalization. Legacy group JSON stays unchanged. Executable schemas cover typed identity, collection policy, subscription scopes and the new event payload; they are not yet advertised as operational MCP capabilities.

SQLite migration 4 is implemented and tested against a legacy fixture. It preserves message sequence values, deleted-row AUTOINCREMENT high-water mark, subscription boundaries/generations, fanout watermark and queued payload bytes; rebuilds FTS; creates a conversation catalogue and typed key namespaces; and rolls back on a late DDL failure. Existing physical `group_id` columns remain compatibility ID aliases, with generated `conversation_id` columns. Group defaults preserve old SQL writers. The single-account state binding remains the account boundary.

Both modules pass `go test -race ./...`; root `go vet ./...` passes. The existing MCP/service/storage test suite passes with the new schema. No production migration has run.

Next: wire the shared policy and typed IDs into storage Put/read/delete/coverage and catalogue updates; filter legacy reads to groups; add general MCP tools and event scopes/profiles; consume both live/replay categories in collector; verify recovery through MCP; update documentation/skills; then perform backed-up deployment and end-to-end acceptance. At that checkpoint, config validation alone did not prove operational collection; the subsequent collector/storage checkpoint below records the wiring.

## Collector and storage checkpoint

The service and local CLI now open storage with the shared typed policy. Put/dedup/identity/event persistence and direct reads/deletion use typed namespaces; unknown conversations are discovered transactionally. Legacy message/context/search/coverage queries filter to groups, preventing collisions from leaking direct records. Existing group subscriptions cannot match a direct event with the same numeric ID. Fanout advances through both categories while preserving the legacy profile for existing targets.

The collector uses a typed listener port through the authentication guard, requests both replay categories after cipher-key readiness and handles typed undo. Its legacy listener port remains available for existing consumers and test adapters. Status counts include permitted direct records, and group catalogue refresh also updates the conversation catalogue. Connection readiness no longer closes collection gaps: it does not prove replay completeness. The earliest open gap remains preserved; operational failure details remain in collector status.

Synthetic tests verify direct/group collision isolation, policy enforcement, dedup after deletion, automatic discovery and a full collector/auth-guard/storage scenario with direct replay, group deletion and a new self-sent direct message. Root `go test -race ./...` and `go vet ./...` pass after these changes. Production binary/config/state remain unchanged.

Remaining: general conversation catalogue/coverage/search/context methods and MCP contracts/tools/resources; new Events scopes/profile fanout/claim validation; diagnostic counters/overflow behaviour; skills and complete documentation; clean cross-platform builds; backed-up deployment, actual replay recovery through MCP and final end-to-end acceptance. None of those is implied complete by the collector tests.

## General MCP read interface checkpoint

Four additive tools now expose discovered conversation catalogue, typed metadata/coverage, general message search and context. Each has an executable input/output schema. Legacy group tools and resource URIs retain their response forms and group scope; MCP discovery now contains twelve tools. General search uses the shared storage policy and adds type to stable timestamp/ID pagination, with a filter-bound signed cursor and the original insertion snapshot when transport shortening occurs.

Typed context and full-text resources stay inside their exact namespace. New message URIs escape opaque IDs; a synthetic MCP client verified IDs containing slash and percent, full Unicode text recovery, collision pagination, incomplete catalogue reporting and cursor rejection after changing type filters. Direct peer names can be learned from inbound author metadata; self-sent messages do not overwrite them with the account owner's name. Catalogue page allocation is bounded to limit plus one, although NFC name filtering still scans the discovered catalogue.

`zalo://collection` is an executable-contract JSON resource reporting collection mode and permitted conversation/message counts by type, with explicit false catalogue/history completeness. It contains no callback, peer IDs, credentials or message text. It uses the same read budget and account authorization as other resources.

Root race tests and vet passed after the general tools were added; focused normal MCP/contract tests pass after adding collection diagnostics. The installed service has not been changed. New general Event scopes/profile remain unimplemented; existing group subscriptions keep their format and scope. Next work is Events fanout/verification/retry integration, ingestion overflow/counters, final documentation/skills, cross-platform build and backed-up deployment/recovery acceptance.

## Events and ingestion pressure checkpoint

Both Events profiles are now advertised. The additive conversation profile supports all/direct/group/exact scopes, typed message identity and optional discovered conversation name. Persisted fanout selects the profile per subscription, keeps stable event IDs and serialized retry bodies, and rechecks the actual message's policy at claim time. Legacy IDs, scope and payload remain unchanged. Overlapping subscriptions share journal event IDs; consumers processing both profiles independently can deduplicate by (name, eventId).

Synthetic tests verify all four scopes, newly discovered conversations, late replay first inserted after activation, Unicode truncation/full-text URI, signed delivery, durable restart and isolated cancellation. A real local TLS receiver independently verifies signatures through HTTP MCP subscribe/read/unsubscribe calls for both conversation types; production address validation remains intact. A policy-reduction restart test confirms that broad and exact direct queues cannot bypass the new group-only policy and that cancelled payloads are cleared. Retry preserves order inside a subscription without blocking unrelated subscriptions.

Two eviction paths were removed: raw socket frame buffers and listener message/replay/undo buffers now apply cancellable backpressure. Cipher-key and parse-error delivery also wait for a receiver. Nonessential status/reaction/typing signals retain best-effort behaviour. Direct and group live counters are separate; successful enqueue, queue pressure and cancellation are observable without IDs, texts or credentials. Burst/cancellation tests exercise both buffer layers. Backpressure cannot prove upstream replay completeness or persistence of a queued message after process failure.

The installed binary, configuration, corpus and active callback scopes are unchanged. Root and nested module normal/race tests and vet pass; focused added diagnostics and policy-revocation tests also pass. CGO-disabled command builds succeed for macOS arm64 and Linux amd64. These builds are temporary acceptance artifacts and have not been installed.

Remaining acceptance: synthetic corpus load/performance checks; complete documentation and both skills; backed-up deployment; typed MCP retrieval of the available missing direct messages; restart and final delivery/recovery audit.

## Corpus load and skills checkpoint

The load benchmark persisted 8,000 synthetic messages in 4,000 conversations, with colliding direct/group IDs and known gaps. On the local Apple M1 Max, three measured iterations reported approximately 4.84 ms for a catalogue page, 2.48 ms for collection status, 180 ms for search plus coverage of 50 hits, and 6.14 ms for context. The complete command took 4.24 seconds including setup. These are local measurements, not an SLA. Status and coverage summaries now use aggregate queries rather than one query per discovered conversation; page memory remains bounded, while catalogue name filtering still scans discovered metadata.

Both optional skills are updated and structurally validated. The historical research skill name is retained for compatibility but covers both conversation types, with explicit legacy-only fallback. Events processing handles versioned conversation and unchanged legacy payloads, typed context lookup and trusted scope/checkpoint association. No model evals were run. Deployment, actual missing-message recovery and the final live audit remain pending.

## Deployment and recovery checkpoint

The authorized macOS installation has been upgraded to the built Go binary with explicit all mode and the existing single LaunchAgent. A fresh stopped-service backup of binary/config/plist/full state passed integrity and file-restoration checks. An isolated copy migrated without network access, preserving eleven legacy tables through their original column views and the message sequence high-water mark; repeated open remained idempotent. Private original backup and post-migration state are retained outside the source tree.

The installed service restored its existing session, became authenticated/connected, and preserved the old group corpus and exact legacy subscription scope/boundary. Direct replay recovered available missing records. General MCP search and typed context retrieval verified the previously missing messages against the reported case; no manual message import or test message send was used. Both profiles are visible in installed Events discovery.

A controlled restart preserved message identities, sequence numbers, text and subscriptions. Repeated upstream replay did not add duplicate corpus records; recovered messages remained retrievable through MCP while reconnecting and after authentication. Known gaps and catalogue/history incompleteness remain visible.

Signed delivery of both types, scopes, cancellation and durable queue restart are independently verified through the synthetic HTTP/TLS receiver tests. No broader live subscription was created for the existing receiver. Its legacy group subscription intentionally excludes direct messages; agent notification for direct messages is not claimed. Remaining work is the final requirement-by-requirement acceptance audit and any corrections it finds.

## Final acceptance

The requirement-by-requirement audit is recorded in [all-conversations-acceptance.md](all-conversations-acceptance.md). It adds a single unified service integration scenario covering source/collector/storage/HTTP MCP/signed TLS delivery, full service restart, replay deduplication and cancellation. Broad-scope fanout quotas and exact typed Unicode boundaries are verified separately. The final installed binary matches the tested source build; both direct and legacy/general group MCP reads, private permissions and installed capability limitations were checked after a controlled restart. There is no unfinished implementation gate; particular-agent direct notifications and complete upstream history are explicitly outside the verified claims.
