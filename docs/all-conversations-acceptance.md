# All-conversation acceptance

Date: 2026-10-03. Scope: [task-all-conversations.md](task-all-conversations.md).
The implementation and authorized macOS deployment pass the acceptance below.
Private runtime reports and consistent backups are retained outside the checkout;
this document contains no account IDs, message text, callback URLs or credentials.

## Requirement evidence

| Requirement | Evidence inspected |
| --- | --- |
| Typed identity and incoming/self direction | `internal/domain/conversation.go`; `TestDirectNormalizationUsesPeerInBothDirections`, `TestGroupNormalizationPreservesConversationAndAuthorInBothDirections`. Direct peer identity stays constant; authors normalize separately. |
| All/selected policy; unknown/new conversations | `TestCollectionPolicySeparatesNamespacesAndDiscoversNewPeers`, `TestCollectionConfigPreservesLegacyAndRejectsConflicts`, `TestTypedCollectorPersistsBothKindsAndDeletesOnlyTarget`. Discovery occurs transactionally without friend-list gating. Example configuration explicitly uses all; legacy service startup reports its deprecated group-only form. |
| Live/replay, mixed batches and decoder failures | Both typed listener paths request replay after cipher-key readiness. `TestMixedReplayPreservesBothKinds`, direct/group decoder tests, collector integration and diagnostics tests cover preservation, safe failure and per-category counters. Available direct replay was also verified on the authorized account. |
| Namespace isolation across reads/deletion/events/cursors | `TestAllPolicySeparatesMessagesEventsAndLegacyReads`, `TestSelectedDirectPolicyDoesNotAdmitSameGroup`, `TestConversationMCPSearchContextCatalogueAndFullText`, `TestConversationDeliveryRevocationAfterRestart`. Equal IDs do not merge direct/group records. |
| General search/context, author/time, Unicode/full text | Four additive executable input/output contracts; MCP tests read escaped opaque IDs and typed context. `TestConversationEventUnicodeBoundaryAndStableTypedURI` checks 2048/2049 code points in both types. Installed MCP search/context verified recovered direct records and existing group records; legacy group context agreed. |
| Exact/all/direct/group subscriptions and boundaries | `TestConversationScopesSignedDeliveryAndCancellation`, subscription boundary/refresh/expiry/concurrency tests, and policy-reduction restart. First insertion controls eligibility; late sent_at does not exclude a new record. Existing group IDs/payload/scope remain unchanged. |
| Ordered retries, isolation, quotas and cleanup | Retry tests preserve subscription order while other subscriptions progress. `TestBroadConversationFanoutCapacityPreservesCollectedCorpus` verifies 20 records → 30 matching targets: three pending jobs and 27 explicit capacity failures, with all 20 records retained. Existing byte quota, removal/retention, deadline/attempt and stale-generation tests cover the shared queue. |
| Transactional migration and compatible rollback | Migration fixture checks seq/high-water, start_seq/generation, FTS, watermark and pending payload bytes, including repeated migration and injected late DDL failure. Offline migration of a private stopped-service copy preserved eleven legacy tables through original columns and sequence high-water; integrity and repeated-open checks passed. Live deployment preserved prior corpus and subscription. |
| Honest coverage and bounded ingestion | Connection readiness does not close gaps; catalogue/history completeness stay false. Both buffer-layer burst/cancellation tests cover backpressure. Diagnostics separate frames, direct/group parsing failures, mixed-replay failure, policy exclusions, duplicate records and committed inserts. Hidden/encrypted/special system categories remain unverified in capabilities. |
| Unified end-to-end service | `TestUnifiedConversationServiceCollectionMCPEventsRestartAndCancel`: synthetic source → collector → SQLite → HTTP MCP → independently verifying real TLS receiver, for both types. The entire service restarts with the same state, old replay deduplicates, new late records deliver, and cancellation stops callbacks without stopping collection. Production callback validation is unchanged; only the test transport redirects the public authority to its local receiver. |
| macOS process, session, permissions and restart | The installed binary matches the final CGO-disabled source build by SHA-256. Existing single LaunchAgent runs the Go service directly with explicit all mode. Session restored without QR/logout. Config/session/DB/token remain 0600, state directory 0700. Controlled restart preserved IDs/seq/text/subscription and added no replay duplicates. Original and post-migration states are kept for compatible rollback. |
| Documentation and skills | README, technical/service/Events contracts, macOS lifecycle, connection examples, development guide, `conversations.md` and both skills are updated. Local Markdown links and skill structure checks pass. Research retains its historical name while covering both types; Events skill routes by profile. Historic group evals are not evidence for the revised skills; no model evals were run. |
| Public-tree privacy | Runtime identifiers are absent from tracked and publishable candidate files. Existing local `config.toml` remains ignored. Session, state, backup and runtime reports were not added to source. |

## Commands and load measurements

Both repository root and `third_party/zcago` pass:

```sh
go test ./...
go test -race ./...
go vet ./...
```

CGO-disabled builds pass for macOS arm64 and Linux amd64:

```sh
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/zl-mcp-darwin-arm64 ./cmd/zl-mcp
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/zl-mcp-linux-amd64 ./cmd/zl-mcp
```

The load command in [development.md](development.md) persisted 8,000 messages in
4,000 conversations, with colliding direct/group IDs and known gaps. Three measured
iterations on Apple M1 Max reported about 4.84 ms for catalogue pagination, 2.46 ms
for collection status, 184 ms for search with coverage, and 5.99 ms for context.
The typed ordering/event/gap indexes occupied 995,328 bytes according to SQLite
dbstat. These are synthetic local measurements, not an SLA or an upstream rate limit.

## Recovery and delivery are separate results

Available missing direct messages were recovered through Zalo replay and verified
by installed MCP search/context, including the previously reported missing case.
Existing group records remained searchable. Restart preserved both categories and
the same subscription boundary. No native Zalo database import, screenshot import,
outgoing test message or new group join was used for acceptance.

The service's signed delivery, scope selection, cancellation and restart are
confirmed against an independent synthetic TLS receiver. The existing production
receiver retains its legacy group subscription. It was not expanded to direct
chats, and delivery/processing of direct messages by that particular agent is not
claimed. Creating a new subscription uses the documented MCP Events flow.

Replay is bounded, catalogue discovery is incomplete, and the machine must be
awake and connected. Empty replay does not prove a complete archive. Special,
hidden or encrypted categories cannot be advertised as verified support. Callback
acceptance is separate from agent processing and downstream notification delivery.

## Rollback boundary

The original pre-migration backup and later compatible backups are private. Stop
the service before restoring a matched binary/config/state snapshot; retain newer
state first. Rolling back to the original group-only snapshot excludes records and
subscription changes after its boundary, so the retained post-migration copy is
needed for their later recovery. The source checkout remains in its original place.
