# Archive control taxonomy: current evidence

Status: the real blocking type is identified; its format-1 visibility semantics remain under investigation. Desktop model integers are not backup format integers. Message visibility rules have not changed.

The snapshot reader blocks every type in the pinned decoder's deferred set.
Owner inspection previously reported only types 33/36, so its zero control count
could not explain a rejection by the broader set. The diagnostic extension in
`de80416` adds whole-file type counts without rendering control contents.

Read-only inspection of the currently installed desktop consumer found the
following exported model bindings in `compact-app-pc.d6af5465eb38c618bfe0.js`:

| Desktop model constant | Integer |
| --- | --- |
| MSG_UNDO | 20 |
| MSG_OA | 21 |
| MSG_ECARD | 25 |
| MSG_POLL | 26 |
| MSG_ZINSTANT | 52 |

Bundle SHA-256: `29d935caddf620b0b4b16a1631460e9aedf5cadd237ae7044fdfd53221cdf5bf`.
The exported getter symbols were matched to their numeric initializers in the
same module; no vendor module was executed. This separates different desktop
model categories, but does not prove how the mobile backup producer encodes each
row or whether the consumer remaps it. It therefore cannot justify ignoring a
source control on its own. Existing type-36/status-3 admission evidence remains
separate from these desktop bindings.

Public source searches returned implementations using wire-string message types,
for example [OpenClaw's Zalo adapter](https://github.com/openclaw/openclaw/blob/main/extensions/zalouser/src/zalo-js.ts),
without a demonstrated integer ChatContent backup mapping. These wire types
cannot establish backup visibility semantics.

Next: trace the corresponding backup consumer branch and visibility effects,
using the format-1 mapping rather than the desktop model enum.
Classify ordinary unsupported content separately from recalls/deletions only
when that path is established. Keep whole-source visibility checks, genuine-ID
suppression, TTL and strict import admission intact. No fresh phone export is
needed for inspecting the saved source.

## Real retained source observation

The installed owner diagnostic reports 24 source/period rows, zero type-33/36
controls and exactly one deferred type-20 row. The sample contains 18 webchat,
five photo and one unsupported record, with no scalar rejections. No new source
was acquired, imported or modified. This identifies the blocker as type 20; the
matching desktop binding is MSG_UNDO, but the format-1 evidence below contradicts
using that binding to classify this backup row as a recall. Its exclusion and
visibility effects are still required before exposing a message prefix. Neither
the desktop constant name nor counts establish a backup target ID.

## Format-1 consumer evidence

Read-only inspection of the current installed vendor bundle
`shared-worker.d6af5465eb38c618bfe0.js` found the format-1 SQLite reader, not just
the desktop message model. Bundle SHA-256:
`713c0c4469ba7467ea2c8dc35a07c1319b72566057fdf3091ca73a59d3f58b1c`.
No vendor module was executed and no account data was read for this inspection.

Its backup enum labels 33 as ChatDelete and 36 as Undo. Its conversion table maps
33 to `chat.delete`, 36 to `chat.undo`, and 20 to `webchat`. The table also maps
25 to `chat.video.live.msg` and 26 to `group.poll`; these differ from the desktop
model bindings above. The exact enum/conversion-table segment is 829 UTF-8 bytes,
SHA-256 `2be2703318114301e009db600b13d5941f167ff58b938ecb1f684c9127b768b9`.

The native message query excludes types 20, 21, 25, 26, 29, 32, 34, 35, 45, 51
and 52 before conversion, while admitting positive-status rows with a nonnull
client ID. Its count query uses the same type exclusions. Thus the type-20
conversion-table entry is unreachable through that message query. The exact
query-definition segment is 533 UTF-8 bytes, SHA-256
`034b46fc8f9246d29cee99fa4d9038d13b9a04f224590d3e5ae9e99acbf0b69c`.

The converter forwards the source global/client IDs, sender, content, timestamp
and TTL directly from ChatContent into the cross-version record. No separate
type-20 target interpretation was found in this branch. This proves the native
reader's exclusion and the enum distinction; it does not prove why the producer
wrote type 20, or whether excluding only that row is sufficient for visibility.

Separately, the desktop undo handler finds an existing message by its IDs,
changes its desktop model type to MSG_UNDO and replaces the stored message;
matching later quotes receive a recalled-message placeholder. That behavior is
evidence about the desktop model. It cannot be transferred to backup type 20
without the missing producer/translation evidence. Keep the public guard and
strict-import rules unchanged until the format-1 row semantics are established.

## Per-type metadata diagnostic and real-source evidence

An [independent iOS database investigation](https://cp-df.com/en/blog/zalo.html)
labels type 20 as a system entry based on comparison with the app UI. Its
[iLEAPP parser](https://github.com/abrignoni/iLEAPP/blob/main/scripts/artifacts/ZaloChats.py)
does not implement a separate type-20 visibility rule. The inspected parser blob
is `b1a1a736313f77f064e562e2179f822a3d192085`. These observations do not establish
whether type 20 targets other messages in our captured producer version.

The owner inspection extension adds optional
`sample_deferred_metadata_diagnostics` when metadata inspection was explicitly
requested. It groups existing fixed metadata observations by pinned deferred
format type, separately from ordinary-message metadata. Only scalar-valid rows
in the requested interval and existing 50-row sample participate. Whole-file
type counts remain separate; an absent sampled category is not absence evidence.
The response returns neither source text/identities nor raw attachment actions.
It performs no acquisition, import, Events or visibility-policy change.

Clean signed source `50f72d4095df4b31a0456e2a1ecea557a8098848` passed local root
tests/vet, affected-package race, macOS/Linux builds and exact-candidate native
acceptance, then [Linux/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37604047117).
Installed binary SHA-256 is
`a719f43ee776d1a8803c1c961c757cad665973d9a358e53e3cd068a501eff37e`.
The unchanged single agent started it; verified backup/state comparison preserved
corpus, identity/tombstone/send/quote/first-incoming records, subscription activation
definitions, configuration, agent, schema 14 and all encrypted archive files.

Owner inspection of the same retained file examined all 24 rows without scalar
rejection. Its single type-20 row has valid metadata, one attachment, nonempty
source text, a present title and three unsupported metadata fields. Its action is
valid UTF-8, 18 bytes, and matches the literal `msginfo.actionlist` in the installed
vendor code by SHA-256. No source text, identity or raw private action was rendered;
archive ciphertext remained byte-identical. This match uses the public vendor
literal rather than attempting to recover arbitrary private values from a hash.

The desktop normalization branch treats webchat objects with that exact action
as MSG_INFO_CHAT. Its interactive-card parser handles group-topic/calendar actions;
its reminder adapter also constructs such informational records. These branches
are distinct from the previously inspected recall handler. This establishes a
specific informational-action candidate for the captured row, not a classification
of every type-20 row or every possible action parameter. Whole-source classification
and a narrow tested snapshot-reading rule are still pending; the public guard and
strict import remain unchanged.

Post-deployment installed STDIO discovery reports 22 tools, archive inventory,
source_id arguments on both browse tools and three resource templates, with
tools/resources/logging capabilities and no Events. Both STDIO status and the
actual connected client's status return connected/authenticated without an error.
The actual client's archive-inventory discovery remains unconfirmed. No send,
acknowledgement, subscription change or new phone export occurred.

Diagnostic source `de80416` passed Linux/macOS CI and local native acceptance,
then was installed with a verified private backup. Installed SHA-256:
`ac58ec1ddd90c44ba256b7047d919edd7062ffd51c5f4197e6bcbcc200f3d890`.
State comparison preserved corpus, identity/tombstone/send/quote/first-incoming
records, subscription definitions, configuration, agent and encrypted archives.
The HTTP port was absent at the 45-second observation; a one-second stack sample
found storage migrations/SQLite startup work. The same process subsequently bound
the port without a second restart. This locates startup activity, not a proven
performance root cause. The collector was then reconnecting with an error; source
inspection succeeded offline and no ack or subscription mutation occurred.

A subsequent actual-client status read returned connected/authenticated with no
last error, without another restart or login. The initial reconnection observation
did not establish loss of the preserved session.

## Narrow snapshot information classification

The source-wide snapshot classifier now accepts only type 20 with valid BinNet
and exactly one attachment whose action bytes equal `msginfo.actionlist`. It
never renders or executes informational text, title or parameters. Other actions,
malformed/missing/multiple attachments and every other deferred type remain
blocking, including 33/36. The scan is independent of requested dates/cursors,
limited to 5,000 rows/8 MiB total metadata/256 KiB per blob and the existing SQLite
execution deadline. Strict mobile import admission is unchanged.

Public archive coverage adds optional `source_information_rows` for the whole-file
classified count. Examined-page omissions use `unsupported_content.native_information`;
their metadata gaps remain counted. Empty informational pages retain continuation.
Clean signed source `578e2a5` passed Linux/macOS CI and exact-candidate native
acceptance, and was installed with both skills after verified state preservation.
Authenticated public archive discovery returned all 56 catalogue entries; reading
the selected September 26 interval returned both historical records over two
pages without another phone request or corpus import. Source ciphertext and
digest were preserved. This proves this file's bounded read; unknown controls,
whole-account visibility and strict import admission remain open.

The first candidate 2eddce8 passed local checks and Ubuntu CI but macOS full-race
CI exposed a pre-existing owner-shutdown race in cancel-before-commit: the mobile
operation could return context cancellation from its final acquisition-journal
write after its previous cancellation check. No candidate was installed on that
failed run. The worker boundary now treats cancelled owner context as normal
shutdown, consistently with its pending-read and snapshot-cleanup boundaries;
errors under a live owner context still propagate. The cancelled-operation/no-prefix
regression passed 20 repeated race runs; the rebuilt source passed both CI jobs.
