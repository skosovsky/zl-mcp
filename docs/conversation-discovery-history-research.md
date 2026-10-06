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

Rechecked on 2026-10-05: the PR page still reports Open and describes group
history only. Our optional group adapter already uses `/api/cm/getrecentv2`
and a separate `/api/cm/getoldv2` phase. This upstream proposal does not establish
a deeper direct/Strangers history endpoint or remove the real mobile-archive
compatibility gate.

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

## Installed native-client static-code follow-up

After the internet/source review, the installed Zalo PC static bundle was examined read-only: version 26.9.10, build 26.9.10.2959; ASAR SHA-256 `0ba19f3f0f96dcce45493e7e4f259d7599836ec3b01543c2ae33d7ea4bf76777`. This is executable-client implementation evidence, not a documented public API. No runtime/session files or native message database were accessed; no login, conversation opening, settings change or second listener was performed. Extracted temporary static files are outside the repository; vendor source is not included here.

The bundle contains a local Strangers predicate for a direct conversation based on friendship/OA status, whether the account has responded, and conversation display state, alongside a setting controlling direct display of stranger messages. This supports treating Strangers as a conversation category, not assuming a separately missing message stream. It does not prove which stream our particular missing conversation uses or historical availability.

The client has `/api/preloadconvers/get-last-msgs`, with a mapping of already known thread IDs/local message IDs and the existing client identifier. This is a bounded preview candidate; it is not a complete inbox enumerator or per-peer historical paging API. Its response, permissions and request semantics must be established before implementing an adapter. Do not send a request that changes read/seen state as a discovery shortcut.

Group cloud paths include `/api/cm/getrecentv2` and `/api/cm/getoldv2`, using the group-cloud domain. No direct-cloud counterpart was established. Static presence of an endpoint does not prove it currently succeeds; the native bundle also contains the previously failing old group-history endpoint. Native polling/socket logic keeps action IDs and backup queue state beyond the minimal first replay used by our adapter. This suggests a synchronization investigation, not an assertion that changing `lastId` alone recovers all missing personal history.

Next protocol step: establish the target peer identity and compare supported catalogue/replay/preview/sync sources through the existing authenticated service, with bounded reads and retained continuation evidence. Keep live non-friend ingestion acceptance separate from historical recovery.

## Bounded group-cloud candidate implementation

PR #370 was rechecked on 2026-10-03 and remains open. The pinned proposal sends
`globalMsgId` as the numeric cursor and uses `nretry=0`; its author-side live
check remains external evidence, not acceptance for this account. The optional
Go page extension now has encrypted local wire tests preserving IDs without
float64, raw records and nullable filtering/continuation flags. It does not
import or emit messages. The installed service is still the separately verified
2699d2e checkpoint; the new history candidate is not live verified or installed.

The Go pinned/hidden methods and JS archived-list method were also reviewed.
They enumerate specific subsets; hidden-list responses additionally contain a
PIN, which must never be passed into catalogue diagnostics or logs. None of
these implementations demonstrates a full Strangers inbox enumerator. The
archived response retains `items: unknown[]`, so its identity shape remains a
protocol research dependency rather than a supported catalogue source.

## Native UI and cache-format verification

A subsequent read-only UI inspection confirmed that the reported conversation
is still visible in the native Strangers list and is unread. The row's menu was
opened and dismissed; the conversation itself was not opened, no read/seen
state was changed, and no message was sent. This corroborates a catalogue
mismatch independently of the installed service's exact/normalized name search.
The accessibility row contains a display name and preview, not a verified
protocol peer ID; these values are not imported into the service by guesswork.

Only file-format metadata was examined under the native client's ZaloData
support directory. A `stranger_box.db` file exists but has no ordinary SQLite
header, and an immutable read-only schema query returns `SQLITE_NOTADB`.
This establishes that the file is not directly readable by standard SQLite,
not whether it is encrypted, compressed or another proprietary format.
No credential/session values or decryption keys were inspected, and no native
message corpus was copied, decrypted or imported. Native cache parsing is not
an implemented historical-recovery source.

## Additional preload candidate: 2026-10-04

A fresh primary-source review found another Go implementation,
[diepxuan/zcloud](https://github.com/diepxuan/zcloud/blob/10f752b431102b71e3185a2077041efff907ed3e/src/zcloud/internal/core/chat.go),
commit `10f752b431102b71e3185a2077041efff907ed3e`. Its `GetConversations` calls
`/api/preloadconvers/get-last-msgs` with encrypted `threadIdLocalMsgId="{}"`
and the current IMEI using the account's conversation service. It treats
`data.clearUnreads` as typed conversation metadata and retains recent `msgs` and
`groupMsgs`. Comments claim 156 metadata entries and about 15–18 recent messages;
those are the author's observations, not our live evidence or guaranteed bounds.

This revises the earlier narrow hypothesis that the preload endpoint necessarily
requires an existing thread mapping. The installed native client confirms the
endpoint and serializes the supplied map; it handles returned clear-unread data
locally. Neither source proves exhaustive Strangers enumeration, a stable
retention window, per-peer older history or absence of server-side read effects.
The candidate needs bounded, account-bound protocol validation before production
use. Its fallback service domain and float-based ID decoding must not be copied:
our implementation requires an advertised account service and exact decimal IDs.

Potential value is metadata discovery beyond friends plus available recent
records, not a complete history source. No new native database access, remote
request to this endpoint, unread mutation or production import was performed for
this source review. Direct history remains unsupported pending stronger evidence.

## Native cloud routing and group decode follow-up — 2026-10-04

The upstream group-history PR #370 remains open/unmerged at head
4eeceafad031ce4f594c4532e3363dbdf01450b0 (GitHub API rechecked). Its proposed
route/payload still matches the reviewed candidate. This does not establish
per-peer direct history or independent live correctness for this account.

A fresh bounded group read failed with SDK API code zero. Reviewing the common
response resolver proved zero is also manufactured for local parse failures.
The group endpoint now resolves the encrypted envelope as raw JSON and decodes
the group leaf separately, exposing only typed allowlisted field reasons. The
next real read identified hasMore as the rejected field. The source contract now
normalizes only boolean or exact numeric 0/1 flags, retaining missing/null as
unknown; nonbinary numeric/string/container shapes remain errors. No raw live
response or message value was logged or retained in public evidence.

Static analysis of the already inspected native Zalo 26.9.10.2959 bundle found
getCM selects getrecentv2 or getoldv2 under the group-cloud domain. Crucially,
its apiEnable gate requires the group-ID prefix or a separately enabled Send to
Me/cloud OA path. Ordinary direct peers do not pass that gate. This is concrete
native routing evidence against renaming/reusing group cloud APIs for a personal
peer. It is not a universal proof that no other direct-history protocol exists.
The same native module exposes pull_mobile_msg/get_crossdb with mobile sync
sequence/key/session parameters, consistent with the previously identified
unfinished upstream synchronization path. No mobile handshake, key exchange,
second authorization or private native database import was started.

## Mobile synchronization source audit — 2026-10-04

[Upstream PR #269](https://github.com/RFS-ADRENO/zca-js/pull/269) remains
open/unmerged at 29d01c4732391cf1b68a6630cd715199ef46348a. Its transport requests
pull_mobile_msg using a generated RSA public key, then handles USER_CONFIRM,
SYNCMSG_INFO and transfer failure. It downloads an authenticated encrypted
archive and reports metadata; it does not establish a usable complete message
import. The author identifies unresolved db.crypt processing. Its example starts
a separate login/listener and logs encrypted-key metadata; neither pattern is
adopted in this service.

The inspected native startup bundle has more concrete archive handling: it
creates a 2048-bit RSA session keypair, decrypts the supplied archive key with
PKCS#1 padding, distinguishes backup format 0 versus 1, and dispatches
DECRYPT_BACKUP followed by RESTORE_CONVERSATIONS/RESTORE_MESSAGES tasks. Static
resource inventory also identifies db-cross-v4's platform-specific Electron
addon and a backup-format WebAssembly library. The binding loader only exports
that native addon; it is not an independently documented portable Go decoder.
These findings identify the next source boundary, not a verified implementation.
No native addon/worker was executed, phone transfer was initiated, key material
was obtained or private backup archive was downloaded.

Before enabling this source, its archive decoder/message schema and phone
confirmation need a reproducible check using the existing guarded session and
bounded silent ingestion. A downloaded file, successful RSA step or message
count in its metadata is insufficient evidence of usable historical coverage.
The missing direct-message case remains open. Existing preload snapshots remain
useful but do not close this source audit.

## Native backward continuation audit — 2026-10-04

The inspected static startup bundle's cloud control passes response `lastMsgId`
and `isOld` together into the next `getCloudMessage` request. The `getCM` call
uses that phase to choose `getrecentv2` versus `getoldv2`; it does not infer the
phase from a nonzero cursor. Source evidence is the same Zalo 26.9.10.2959 static
bundle (SHA-256 ef48521fcca4894c9894ceeeb94a2b7ebc7419af001a696285c9af3ec23dd88a),
at the cloud control's client-retry and `getCM` routing boundaries. This was a
static read, without native code execution or access to the private corpus.

Our decimal continuation currently omits that phase and always reads the recent
route. Successful one-page acceptance therefore does not prove traversal of old
pages. The next implementation must persist phase with the operation cursor,
retain compatibility for saved operations, and prove the old-route transition
without treating absent phase metadata as false or guessing from message IDs.

The available Chrome account was also inspected through the personal plugins
UI: its personal-plugin list is empty and its installed sidebar contains Figma
and Sites, not Zalo. No plugin, permission, subscription or connection was
changed. This surface cannot establish discovery for the existing Zalo client;
an authenticated account containing that connection is still needed.

## Native archive decoder boundary — 2026-10-04

Static inspection of the installed darwin/arm64 db-cross-v4 addon identifies
two exported JavaScript entrypoint names: decompressAndDecryptDb and
decompressAndDecryptDb_V2. This is more specific than the binding loader: the
archive decompression/decryption boundary resides in a platform-specific
Electron addon. The inspected sync and mainless worker bundles did not expose a
portable implementation or a verified parameter/schema contract for those
entrypoints. String inventory is evidence of entrypoint names only, not proof
of algorithm, format compatibility or a callable Go integration. The addon was
not loaded or executed; no real archive/key was read.

Next source evidence must establish the two formats' parameters, encrypted
container/decompression limits and decoded message schema before a bounded
portable decoder can be implemented. Merely launching the addon or using its
name as a capability claim would not satisfy the single Go-service contract.

## Archive call sites and decoded database boundary — 2026-10-04

The native shared worker (SHA-256
7a83866c905956cfb122571b2f28cf00a8b61e0f362c40a82010a9947a7d6ff7)
provides the actual call sites missing from the earlier worker sample. Format 0
passes input path, output path and decrypted key to decompressAndDecryptDb;
format 1 additionally uppercases the key and supplies a progress callback to
decompressAndDecryptDb_V2. This confirms the wrapper contract, not the cipher.

Restoration distinguishes a single SQLite database containing threads/chats
(format 0) from a directory of per-conversation .db files (format 1). The latter
uses group-prefixed filenames and message fields including SenderId, GlbMsgId,
CliMsgId, MsgContent, TimeStamp, TTL and MsgType. Attachments/quotes require the
separate native parseBinNet boundary. Both restore flows also translate backup
plain/noise conversation identities before insertion. Copying IDs or rows
without that mapping would violate the exact typed/account ownership contract.
No real database was opened and no decoder was executed.

A [public decoder analysis](https://github.com/s36-technology/zalo-linux/blob/master/COMPREHENSIVE_ANALYSIS.md)
is a candidate reference only. At inspected commit
4a6bb86d270d8df8d371528ec7136a8d9c8d940b the complete Git tree has no cited
generate-addon.py or offline_decrypt_check.py and no declared license. The
current tree therefore does not substantiate that document's source-code claim;
no code/binary was adopted. Further commit-history inspection hit GitHub's
anonymous API rate limit. This does not prove that a portable decoder is
impossible, nor justify claiming its algorithm implemented.

## Pinned decoder reference and native integrity check — 2026-10-04

The candidate source was located in a separate
[realdtn2/zalo-linux-2026 tree](https://github.com/realdtn2/zalo-linux-2026/tree/0f3049a6ab6fca6862d50fc73150d09776cf7035),
commit 0f3049a6ab6fca6862d50fc73150d09776cf7035. Its generate-addon.py contains
an independently published format-1 decrypt/extract implementation. No code was
copied or executed, and that tree declares no root license. Its format parsing
is explicitly inferred: it tries alternative ciphertext spans and omits the
native header-integrity check. It is research evidence, not a verified decoder.

Static disassembly of the installed arm64 addon independently confirms an
AES-256 CBC call, a zero-initialized IV buffer, 65536-byte reads, and ZDB4.0
magic verification in the format-1 decrypt path. Its subsequent decompressor
reads big-endian header length/checksum after the six-byte magic, hashes the
header segment with XXHash32, compares that result, and then seeks to byte 14
to read the file count. The public candidate skips that validation. The complete
file-table/checksum scope, key/chunk behavior, compressed-stream framing and
plain/noise identity mapping still require a reproducible fixture check before
being used as production decoding rules. No native function was executed.

The next implementation should be an independent bounded Go decoder with
explicit format/version, rather than the candidate's trial-and-error fallback.
It must reject malformed header/checksum, unsafe or duplicate archive names,
excessive counts/declared sizes, decompression overflow, truncated output and
unknown identity mapping before corpus mutation. A synthetic round-trip alone
cannot prove compatibility with a real mobile archive. Existing tools continue
to report direct preload as a limited snapshot; mobile history is not exposed.

The native checksum initial words were read from the installed addon and match
XXH32's published seed-zero accumulator values. read_header independently
confirms a 128-byte name ceiling. These findings now support the internal
[plaintext-header candidate](contracts/mobile-backup-header.md); its independent
reference-vector checks are documented in acceptance. Real archive framing and
plain/noise mapping remain the next source boundaries.

The native CBC method copies its caller-provided IV into local storage before
block processing. Combined with the zero IV passed by each 65536-byte call, this
confirms chunk reset independently of the fork. Format-1 key output is requested
as hexadecimal text by the native RSA wrapper; the caller uppercases it and
AES-256 expands its text bytes. This evidence supports the offline internal
[block-transform candidate](contracts/mobile-backup-blocks.md). Real framing,
compression, database schema and account mapping still require acceptance.

## Native XZ stream and Go decoder memory policy — 2026-10-04

The installed arm64 addon resolves stub 0x256fc to `lzma_stream_decoder`.
Both `ZCUtil::InitDecoder` (0xcb84) and `DecompressProcessV2` (0xd034)
call it with an unlimited memory argument and flags 0x08. The official
[liblzma container contract](https://tukaani.org/xz/liblzma-api/container_8h.html)
identifies this as the XZ stream decoder with `LZMA_CONCATENATED` enabled.
This independently establishes XZ framing, including concatenated streams;
it does not establish a real archive's filters, payload boundaries or trailer.

The pure-Go candidate `github.com/ulikunitz/xz` v0.5.17 was downloaded with
Go checksum verification, without modifying go.mod or go.sum. Its pinned
source commit is 6ead826b4d3c7c9856f2daa905cf06403b9daddc. Source inspection
of `lzmafilter.go` shows that `ReaderConfig.DictCap` is increased to the
dictionary size advertised by a block before `NewReader2` allocates it.
Therefore setting DictCap does **not** impose a maximum on XZ decoding.
An output-size limit alone cannot bound that allocation. Do not introduce
this reader into the service under an assumed memory ceiling.

Before adoption, independently validate every XZ block's filter/dictionary
declaration against a fixed budget before allocation, or use a decoder with
an enforceable maximum. Also bound compressed input, decoded bytes, block and
stream counts, cancellation and declared file sizes; validate stream integrity
to terminal EOF before persistence. Reject unsupported filters explicitly.
No compression decoder is wired into the current offline candidate or runtime.

The subsequent offline candidate implements a bounded container preflight
before using the pinned reader. It checks every indexed block's LZMA2 dictionary
declaration, stream/block counts, indexed output budget and structural CRCs
before decoding. The reader then validates the compressed stream to EOF and
exact output length. Contract: [offline XZ decoding](contracts/mobile-backup-xz.md).
The new dependency is now pinned in go.mod/go.sum; its license is BSD-3-Clause.
Independent Python/liblzma and XZ Utils vectors cover check types, concatenation
and multiple blocks. This closes the offline dictionary-allocation boundary;
it does not establish mobile download, encrypted payload slicing, SQLite/account
mapping or recovery of the missing message. It is not wired into the service.

## Existing-listener mobile transport boundary — 2026-10-04

The native startup bundle independently identifies file-service routes
`pull_mobile_msg`, `cancel_pull_mobile_msg` and `get_crossdb`. Sync controls are
routed by act_type=syncmsgmb, with act=user_confirm/syncmsg_info/transfer_error.
Confirmation matches both request public key and pc_name; user_action=0 rejects,
2 reports mobile restoring, and 1/3 confirms. Backup success matches the request
public key. These values must not be collapsed into a generic successful control.
The current Go listener routes file/group/friend controls and does not publish
syncmsgmb controls; adding only an HTTP request would lose its asynchronous reply.

The native request also increments a nonzero last sequence before sending
from_seq_id; the reviewed PR passes its argument directly. A contract must define
whether its input means last retained sequence or first requested sequence,
preserve exact integer precision, and avoid replay guessing. Initial zero has
the same wire value in both implementations. Cancellation carries the same
public key/pc_name/IMEI. No transfer, cancel request, new login/listener or archive
download was initiated during this audit. The upcoming transport must correlate
controls within the existing session, expose only safe operation state, bound
wait/download work, and keep backup URL/key material out of logs and MCP output.

The native key generator in the inspected startup UI requests RSA-2048 with
publicKeyEncoding type=spki, format=der, encoded as base64; its private key uses
PKCS#1 PEM. This independently confirms the public wire form in the reviewed PR.
The optional SDK [initial request contract](contracts/mobile-backup-request.md)
now validates that form, fixes initial sequence/retry values at zero, and provides
same-key cancellation without adopting unresolved continuation semantics.
Neither method is invoked by the service yet. Native request acknowledgement
is distinct from asynchronous phone confirmation and usable archive evidence.


## Corrected complete-container boundary — 2026-10-04

Further bounded static disassembly contradicts the earlier header-only assumption.
In the installed arm64 addon, decompress_file at 0x43c0 seeks to offset 6, reads
and byte-swaps a length; at 0x43f0 it subtracts 14. It hashes that many bytes from
offset 14, compares the checksum, then seeks back to 14 to parse the file count
and table. At 0x4b4c–0x4b68 it subtracts ftell from that **same length** and passes
the difference to DecompressProcessV2. The latter at 0xd1b8–0xd20c bounds fread
by that remaining compressed byte count, then distributes decoded output by file
sizes. Indirect symbol references confirm fread=0x25690, fseek=0x256a8,
ftell=0x256b4, fwrite=0x256c0 and lzma_stream_decoder=0x256fc. No native code
was executed and no private archive was accessed.

Thus the length is the complete declared container end, not the table end; the
checksum covers table and compressed payload. The old table-only fixture could
not establish that boundary. The old ReadHeader candidate was removed and replaced
with complete bounded checksum validation, a separate bounded table parser and
exact plaintext XZ/file splitting. Encrypted padding/trailer, real SQLite schema
and account/plain-noise mapping remain unverified; no production history-source
claim follows from these synthetic components.


## Archive owner/sender ID translation — 2026-10-04

Bounded static review of the installed renderer/shared-worker independently
identifies the mapping step. Format-1 restore enumerates numeric .db/group_.db
owner IDs and imports direct/group IDs into NoiseIdStore. Row conversion uses
SenderId, GlbMsgId and CliMsgId; verifyCrossMsg also collects missing owner/sender,
mention and quoted-owner IDs. The renderer's REQUEST_NOISE_ID calls getnuid;
static implementation encrypts numeric fids/gids and posts /api/znoise to
zwid.api.zalo.me. Reply arrays are positionally matched with count checks, and
group results are g-prefixed for the native client's representation.

Native request preparation uses Number.parseInt, with precision risks above
2^53. The independent Go candidate retains canonical uint64 decimal identities
via json.Number and typed positional mappings, rejecting duplicates, count
mismatch, invalid numeric forms and ambiguous direct-to-group guesses. It never
uses a source ID as an implicit session-ID fallback. It is a bounded plaintext
codec only; no mapping request was sent. Real response representation and
account/operation-bound mapping remain to be verified. The native text MsgType
is 0; BinNet parsing and quote/attachment semantics still need independent
validation before full row conversion/import.


## BinNet structural framing — 2026-10-04

Installed binding dbUtils points to the same db-cross-v4 native addon. ParseBinNet
at 0x22dc calls parse_bin_net at 0x6de4, which invokes tlv::TlvBox::Parse at
0x9fe0. Static parser instructions read a big-endian length from prefix+4 and
a big-endian tag from prefix+0, then retain declared value bytes. GetValues and
the attachment loop establish repeated fields, so a single-value map would lose
metadata. Native top-level dispatch exposes attachment/quote/property/mention/
reference parsing paths; exact semantic validation remains separate.

An internet search located unrelated TLV libraries with similar names, but no
public result established Zalo BinNet compatibility. They are not a source of
copied decoder code or proof of the protocol. The Go candidate is an independent
bounded structural parser only, retaining unknown/repeated fields as opaque
values. Real BinNet fixtures, nested quote/attachment schemas and account-linked
ID translation remain required before converting these rows to corpus messages.
No native function was executed or private metadata accessed.

## Quote scalar widths — 2026-10-04

Static GetIntValue/GetInt64Value load then byte-reverse 4/8 bytes, establishing
big-endian scalar payloads independently of the TLV framing. Quote nested tags
80–85 map to ownerId/cliMsgId/globalMsgId/cliMsgType/ts/ttl; exact getter/property
addresses are recorded in contracts/mobile-backup-quote.md. Native signed-to-double
conversion can round identifiers above 2^53. The Go candidate retains signed
integer bits with explicit presence and rejects malformed widths/duplicate known
scalars. This does not validate owner identity or quote semantics. No private
BinNet, native function execution, network request or corpus import was used.

## Quote byte fields — 2026-10-04

Following each GetBytesValue branch through explicit-length N-API string creation
establishes nested tags 86/87/88/90 as msg/attach/fromD/quoteStatus. They are raw
UTF-8 strings; their labels do not establish JSON or attachment schemas. The
internal candidate validates UTF-8, retains presence/empty values/embedded NUL,
owns and clears byte buffers, rejects duplicate known fields, and counts skipped
unknown fields. This extends the scalar candidate but is still partial metadata
parsing: account identity, attachment semantics and real archive compatibility
remain unverified. No source/private message bytes or native executable code were
used to produce fixtures.

## BinNet envelope and partial coverage — 2026-10-04

Native top-level tag-7 branch (0x6f54 to 0x7108) feeds the nested quote parser
(0x74e8). The offline Go envelope now routes that field to the bounded quote
candidate. Unsupported top-level occurrences retain tag order and are counted,
including repeated attachments; unknown nested quote fields also contribute to
coverage counts. Duplicate quote tags and malformed selected metadata fail the
whole result. Unknown values are not recursively guessed. Recognition is not
identity/semantic validation, and no zero-unknown count establishes completeness.
No real BinNet or installed runtime was changed by this checkpoint.

## Prepared offer orchestration — 2026-10-04

The internal runner now connects an existing prepared ledger attempt to a guarded
MobileBackupSource and its durable observer. It accepts a private offer only after
its own successful BeforeDispatch commit and persisted offer_ready state. Failed
post-dispatch execution closes evidence with an independent bounded persistence
context; cancellation/unknown/authentication loss remain interrupted. An invocation
that never committed dispatch cannot finalize the attempt: another attached
observer may have won concurrently. Neither a rejected retry nor a stale observer
can interrupt the winner. No service/CLI entry, phone request, archive download or
message import was enabled by this checkpoint.

## Mention scalar subset — 2026-10-05

Native GetValues(8) at 0x7244 iterates repeated mention TLVs, parsed at 0x728c.
Nested tags 100/101/102/103 are signed 4-byte type/uid/pos/len respectively;
getter/property addresses are recorded in mobile-backup-mentions.md. The offline
BinNet envelope retains mention order, presence and signed values, rejects duplicate
known scalars/wrong widths, and counts unknown nested fields. This does not establish
uid/session mapping, mention type meanings or offset units. No text slicing or
resolved domain mention is performed; real archive acceptance remains open.

## Existing-session service port — 2026-10-05

The internal membership port now connects prepared mobile-offer execution to
current JoinManager.API, which production collector readiness sets to sessionGuard.
Caller/service cancellation is linked before session ownership, and the read lock
holds the current session through execution. Integration through the actual
collector lifecycle verifies one restore/source, one dispatch and no terminal-ID
redispatch. No new restore/listener, CLI/MCP route, download or message import is
introduced. The private offer goes only to an internal future archive consumer.

## Format-1 declared boundary assembly — 2026-10-05

Additional native static evidence: decrypt_file writes each fread-sized decrypted
chunk back unchanged in length at 0x4298–0x42a8. GetPaddingLength at 0xe324 rounds
to a block boundary without adding an extra aligned block. DecryptCBC copies the
supplied IV into its private working buffer at 0x10880–0x1088c, preserving zero-IV
restart for each chunk. No PKCS unpadding occurs. The established container decoder
bounds checksum/XZ by the declared end and does not feed physical tail bytes into
XZ. This does not establish the sender's padding content/length convention.

An independent bounded Go assembly now decrypts, checks that declared end lies
within the decrypted bytes and validates/splits exactly that region. Tail bytes
are separately counted inside the ciphertext budget, neither treated as padding
nor authenticated. Independent liblzma/official XXH32/OpenSSL fixtures cover aligned,
zero-tail and opaque-tail streams. This closes offline stage composition only;
real correlated archive, verified host and account/message conversion remain open.

## Exact selected-file identity — 2026-10-05

Internal archive selection now builds exact typed mapping requests from validated
canonical filenames, retaining positive uint64 IDs as decimal strings. Complete
mapping is required before choosing an index: no missing/extra/duplicate plain
pairs or same-type many-to-one session pairs are accepted. Equal numeric IDs in
direct/group namespaces stay distinct. Only the exact typed session conversation
is selected; source IDs never act as fallback. File contents are neither copied
nor opened at this stage. This narrows selected-file routing, not authenticated
archive/account ownership or complete history. Guarded same-operation mapping and
real filename/schema compatibility remain required.

## Selected staged fetch and session scope — 2026-10-05

The internal staged path now combines independently configured download policy,
exact ciphertext bounds, format-1 assembly, guarded filename mapping and exact
selected-file routing. Failure returns no private partial result; downloaded bytes
and decompressed archive regions are cleared. Success transfers only the selected
region, clearing unrelated regions before returning. It remains opaque file data,
not a validated SQLite/message import or completeness claim.

The service wrapper holds its existing session port across the stages and borrows
revocation cancellation from sessionGuard, including non-SDK network/CPU work. It
requires offer/mapping/scope interfaces and numeric selection before dispatch;
late success after scope cancellation is discarded. There is no public route,
default download host or actual phone/HTTP action at this checkpoint. Real host,
archive/account/row acceptance and local entry remain open.

## Native row filtering and page preparation — 2026-10-05

Format-1 SQL applies MsgStatus > 0, non-null client IDs and an excluded type list;
the static client type-name table distinguishes webchat/photo/voice/sticker/file/
location and other payload kinds. Delete/undo have separate type names and cannot
be silently treated as ordinary text. The client preserves sender/global/client
IDs, MsgContent, millisecond timestamp and raw TTL alongside parsed BinNet.

The internal candidate now prepares at most 50 selected rows under exact interval,
request-record and 8-MiB aggregate bounds, with complete unique sender mapping and
explicit direction. Direct senders must be the selected peer or current account.
Deferred controls/unknown types/missing or invalid metadata remain counted gaps;
known nontext kinds keep raw payload rather than pretending it is decoded text.
It returns private mapped rows only, not domain messages, Events or persistence.
Real metadata/TTL/control/content semantics and selected silent import remain open.

### Mobile archive TTL evidence (2026-10-05)

Read-only client source tracing confirms millisecond TTL preserved through format-1
conversion and message normalization. `Xu5j` adds it to the archive server timestamp;
`wlxX` contains a client-clock fallback which our importer must not silently use.
The internal exact-integer helper rejects missing timestamps and overflow. See
[expiry contract](contracts/mobile-backup-expiry.md). No live archive acceptance,
expiry scheduler, control-state restoration or historical persistence is implied.

### Archive deletion and quote expiry boundary (2026-10-05)

The installed format-1 importer maps source types 33/36 to `chat.delete` and
`chat.undo`; `_filterDeletedMessages` separates normalized undo messages and calls
`_sendUndo`, whose inspected implementation is empty. A separate format-2/export
transformer remembers normalized `msgType == 20` IDs for recalled quote handling.
These are different representations: do not assign the exporter's type 20 meaning
to format-1 SQLite type 20, or infer a delete target from an arbitrary MsgContent.
Neither observation proves correct cross-page deletion restoration for our corpus.
Those controls remain explicitly deferred.

Internal candidate expiry now uses an explicit clock and whole-page validation.
Expired messages are removed and owned buffers cleared; expired quoted payloads
are cleared without deleting the containing message. The redundant raw BinNet copy
is cleared as well, and repeated checks count no second transition. No database
writes, notification Events or ongoing expiry scheduler are provided by this step.

### Mobile archive download session behavior (2026-10-05)

Rechecked the public synchronization PR #269, which remains an unfinished proposed
implementation. Its source obtains chat.zalo.me cookies and forwards them with a
fetch to the supplied archive URL; neither exact archive hosts nor safe credential
scope are established by that example. The installed official desktop client
separately confirms cookie-enabled file downloading in its sync download method
(`cookies: true`, no URL transformation). These two observations make the current
anonymous Go downloader an unaccepted compatibility candidate, not a proven real
archive transport. See the [download compatibility gate](contracts/mobile-backup-download.md).
Do not infer that a Cookie header is mandatory for every signed URL, and do not
copy the third-party arbitrary-host cookie forwarding behavior. No actual phone
request, offered URL, cookie extraction or archive download was performed.

### Recovery prerequisite audit (2026-10-06)

Rechecked upstream [PR #269](https://github.com/RFS-ADRENO/zca-js/pull/269):
it remains open and explicitly unfinished. Its author points to PR #260, but
[merged PR #260](https://github.com/RFS-ADRENO/zca-js/pull/260) changes 11 files
for contacts, profile and group APIs. The reviewed file list contains neither
mobile dispatch nor archive import/history pagination. A checked body task saying
"implement pull message" is not implementation evidence for those capabilities.
No verified production download host or completed archive decoder follows from
these references. Do not treat the merged PR as closing mobile-history acceptance.

The current production path was reviewed at the exact source checkpoint:
`membershipPort.walkPreparedMobilePages` holds one existing account/session scope,
fetches an archive, borrows pages to visit and clears the selected bytes on return.
`WalkPreparedArchive` clears its aggregate summary on callback/context failure;
that does not roll back side effects a visitor already committed. The prepared
cursor is bound to request/fingerprint/account and a selected SQLite digest, but
is currently an in-memory value. The mobile attempt ledger records dispatch
ownership and reaches offer_ready; it does not own a resumable persisted archive
or an atomic imported-page checkpoint. The generic history-operation path already
commits its supported page and checkpoint transactionally, while the separate
expiring-history writer commits TTL records without a mobile checkpoint.

Therefore production wiring cannot consist of a visitor calling the standalone
silent writer and reporting aggregate counts. Before dispatch/import is exposed,
the executable contract must bind an attempt to an account-owned exact archive
snapshot, define private lifetime/cleanup and crash behavior, and atomically
persist each page's records, original expiry, source counts and continuation with
cancellation/revision checks. Restart must use the same snapshot or stop with an
explicit unavailable source, never dispatch another phone request implicitly.
Unsupported content and control semantics must remain explicit, with no Events.
These are prerequisites identified from code, not implemented capabilities or
proof that the requested old Strangers record can be obtained.

No phone request, archive download, credentials extraction, Zalo send or
subscription mutation was performed for this audit.
