# Archive control taxonomy: current evidence

Status: the real blocking type is identified; recall target/visibility semantics remain under investigation. Message visibility rules have not changed.

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

Next: observe the exact whole-file deferred type through the owner diagnostic,
then trace the corresponding backup consumer branch and visibility effects.
Classify ordinary unsupported content separately from recalls/deletions only
when that path is established. Keep whole-source visibility checks, genuine-ID
suppression, TTL and strict import admission intact. No fresh phone export is
needed for inspecting the saved source.

## Real retained source observation

The installed owner diagnostic reports 24 source/period rows, zero type-33/36
controls and exactly one deferred type-20 row. The sample contains 18 webchat,
five photo and one unsupported record, with no scalar rejections. No new source
was acquired, imported or modified. This identifies the blocker as type 20; the
matching desktop binding is MSG_UNDO. Determining whether this row replaces an
original message or targets another row is still required before exposing a
message prefix. Neither the constant name nor counts establish the target ID.

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
