# Rich content: upstream evidence and investigation boundaries

Checked: 2026-10-15. These are implementation references and user reports, not
an official Zalo protocol guarantee or a substitute for the local live checks.

## Reaction references

The [upstream report #360](https://github.com/RFS-ADRENO/zca-js/issues/360)
describes mobile reactions with a positive client reference and a zero global
reference, while desktop reactions can include a global reference. This is
consistent with the observed local incoming/outgoing shapes; it does not prove
that direction itself causes the difference. Our local fix resolves only a
unique visible client-reference match in the exact typed conversation. See the
[contract](contracts/rich-content-events.md) for bounds and upgrade compatibility.
The same live input subsequently resolved to the exact synthetic target and
matched the signed callback, source and MCP journal. No real IDs from that public
report or the local test are copied into fixtures.

## Photo captions and albums

The pinned [sendMessage implementation](https://github.com/RFS-ADRENO/zca-js/blob/0586dc5e4981985c5f16b9b3a5f285a02feb6f9b/src/apis/sendMessage.ts)
places user-supplied image text in outgoing `params.desc`. For multiple images it
also supplies `groupLayoutId`, `isGroupLayout`, `idInGroup` and
`totalItemInGroup`. These are outgoing request fields, not an established mapping
to incoming attachment `title`, `description`, `params` or `childnumber`.
The inspected incoming model defines these attachment fields without identifying
which carries an author caption. The later controlled incoming direct single-photo
batch established title provenance for that restricted shape, as specified in
the [contract](contracts/rich-content-events.md#controlled-direct-media-refinements).
Neither that result nor the outgoing `desc` field justifies promoting every
attachment title to author text or interpreting `childnumber` as an album length.
The native album form remains pending. Media URLs
and arbitrary native parameter bags remain excluded from public content.

## Controlled direct media batch

The 2026-10-16 batch independently confirmed `chat.voice`, `share.file`,
`chat.location.new` and `chat.recommended` with exact `recommened.link` action.
The file title exactly matched the instructed synthetic filename; the photo
title exactly matched its instructed caption. These establish restricted
direct/incoming mappings, not all attachment/direction/namespace semantics.
Numeric width/height, fileSize and duration parameter types were observed, but
their values were not checked against known physical properties; no units or
coordinates are published from those observations. Native source evidence is
recorded separately from the offline refined-projection tests in the
[current acceptance matrix](rich-content-acceptance.md).

## Calls

The pinned zca-js listener at
[`0586dc5e4981985c5f16b9b3a5f285a02feb6f9b`](https://github.com/RFS-ADRENO/zca-js/blob/0586dc5e4981985c5f16b9b3a5f285a02feb6f9b/src/apis/listen.ts)
has no explicit call observation handler. The local Go SDK likewise exposes no
call channel. The upstream feature requests
[#184](https://github.com/RFS-ADRENO/zca-js/issues/184) and
[#366](https://github.com/RFS-ADRENO/zca-js/issues/366) were still open when checked.
They discuss initiating calls, so they do not establish whether call summaries
or lifecycle notifications arrive through the web message connection.

The independently inspected
[`zjr_api` source, revision `e6614fb1c3d1b9dabb423c52fbb3a71b46e73efc`](https://github.com/PhuDev-2010/zjr_api/blob/e6614fb1c3d1b9dabb423c52fbb3a71b46e73efc/zjr_api/_client.py)
contains outbound call requests. Its inspected WebSocket dispatch handles
messages, controls/actions and reactions, without an explicit call observer.
The library was read as source only: it was not installed, run or given account
credentials. Outbound call endpoints are not proof of received event semantics.

At this research stage, call Events remained unadvertised. The later exact
source discriminator match below supports only a narrow summary classifier.
Do not invent missed/answered/duration from empty text, action names, numeric
fields or an increase in an unknown-frame counter.

## Bounded diagnostic preparation

The prior last-unhandled-command counter can be overwritten by an unrelated
heartbeat reply. The diagnostic extension keeps at most 16 distinct numeric
protocol header tuples with counts and an explicit overflow counter. Snapshots
are detached and synchronized. Separate counts record ignored actions/controls;
their arbitrary names/data are never logged. No native payload, call session,
media address or credentials enter this diagnostic model.

A controlled incoming call can now be compared against a private baseline of
message/activity sequences and these counters, through the same existing
listener. It may establish the carrier of an observation; correlation alone
does not establish lifecycle semantics. An absent observation does not prove
that Zalo has no call feature. A later need for sensitive raw payload capture
requires a separately scoped agreement under the task's diagnostic rules.

Baseline/result comparisons must retain the same connection generation: the
collector's `last_connected_at` is recorded privately before the action and
checked afterwards. It is updated by the existing Connected callback on each
listener start; retry runs with a fresh connection must not be compared as one
continuous diagnostic counter interval. A changed connection stamp invalidates
that comparison rather than proving a zero/negative event delta.

At diagnostic preparation, the first proposed case was one incoming voice call from the existing test peer,
left unanswered and then canceled by its caller. This describes the historical baseline; subsequent user-confirmed calls and
observed records are documented in the implementation checkpoint. No phone sync, second Zalo
listener, call initiation API or automatic media download is needed.


## Call discriminator research lead

The author's first-hand iOS forensic test comparison in
[the Zalo parser investigation](https://cp-df.com/en/blog/zalo.html) distinguishes
`recommened.calltime`, `recommened.misscall` and `recommened.groupcall` in local
backup blobs. This is a primary empirical source for a diagnostic vocabulary,
not proof that our listener uses the same field/action or numeric status units.
Before extending the closed diagnostic vocabulary, the retained record exposed
an unknown action and numeric parameters `callId`, `calltype`, `duration`,
`reason`. The subsequent exact match is recorded below. No production mapping
follows from this external source alone.


## Exact native discriminator confirmed

The owner-only probe subsequently matched `recommened.misscall` on the exact
retained direct `chat.recommended` record. All four params are JSON numbers;
no values were exposed. Both reviewers accepted this as sufficient evidence
for a narrow call-summary observation with unknown outcome/mode and null
duration/native status/call ID. It does not identify which of the two calls
produced this one record. A later fresh native misscall observation matched
source, signed callback and MCP journal. The paired fresh calltime record
established `recommened.calltime` with numeric callId/calltype/duration and no
reason field; its original message projection remains unchanged. The refined
calltime activity path is verified autonomously, separately from native input
provenance. Group-call actions and group namespace remain unverified.
The former unknown message and its delivery remain immutable; cross-ledger
identity accounting prevents its later replay from becoming a new notification.
