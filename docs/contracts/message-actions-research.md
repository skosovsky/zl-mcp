# Explicit read receipts and reactions: upstream evidence

Research date: 2026-10-16; updated 2026-10-18. Initial upstream findings below are
historical preparation. Approved direct native writes have now been performed;
see the [acceptance record](../message-actions-acceptance.md). Public executable
contracts are the schemas for `zalo_mark_messages_read`,
`zalo_set_message_reaction` and `zalo_get_message_action_status`;
this narrative is supporting evidence, not a replacement for those schemas.

## Primary sources

Pinned zca-js revision: `0586dc5e4981985c5f16b9b3a5f285a02feb6f9b`.

- [sendSeenEvent implementation](https://github.com/RFS-ADRENO/zca-js/blob/0586dc5e4981985c5f16b9b3a5f285a02feb6f9b/src/apis/sendSeenEvent.ts)
- [addReaction implementation](https://github.com/RFS-ADRENO/zca-js/blob/0586dc5e4981985c5f16b9b3a5f285a02feb6f9b/src/apis/addReaction.ts)
- [Reaction model](https://github.com/RFS-ADRENO/zca-js/blob/0586dc5e4981985c5f16b9b3a5f285a02feb6f9b/src/models/Reaction.ts)
- [Read-state observation in upstream PR 380](https://github.com/RFS-ADRENO/zca-js/pull/380)

These are unofficial Zalo implementations and author observations, not an
agreement with Zalo or proof of current account behavior.

## Read receipts

The library submits 1–50 messages from one typed conversation to direct/group
`seenv2`. Each entry carries server/client message IDs, sender and recipient,
original message type, ST/AT/CMD and original timestamp. Group requests include
IMEI; direct recipient equal to the account is encoded as `"0"`.

Upstream PR 380 reports a `clearUnreads` event with a last-message watermark after
read receipts. That supports treating a conversation-wide effect through the
selected message as possible; it does not prove a per-message-only operation.
The tool is named `zalo_mark_messages_read` and explicitly describes possible
watermark effects. API acceptance,
unread counters and sender-visible status are distinct observations.

Current zca-js ternaries collapse truthy ST/AT/CMD/TS values to zero; do not copy
this behavior. Preserve actual native values. The Go decoder previously collapsed
missing/null ST/AT/CMD to zero. Presence must be recorded at the wire boundary;
old records cannot be silently upgraded with invented zeros. Group decoding must
retain mentions despite the embedded message decoder. Media use their native
message type; calls, reactions and controls are not eligible message targets.

## Reactions

The request contains one target identified by numeric server/client message IDs,
fixed upstream `msgType=1`, chosen icon/type/source and a generated client ID.
Separate routes serve direct and group conversations. The upstream model has a
finite catalog; public tool input must not expose arbitrary protocol type/source.

The NONE enum is an empty icon with type `-1`, source `6`. zca-js accepts that
encoding, whereas local zcago previously rejected it. Allowing its encoding is
not evidence that it clears every accumulated reaction. The current Go method
also ignored ID parse failures; malformed IDs must fail before an HTTP write,
without float64 conversion or substitution with zero.

Primary source code does not establish whether repeated adds accumulate, replace
or toggle, nor whether NONE removes all of the account's reactions. The public
state-setting contract now uses clear/add based on the approved direct text case;
media/group, accumulated counts and other-user reaction preservation remain
separate acceptance cases. Do not infer state from API response
`msgIds`, or infer the target from the reaction event's own message ID. Existing
observations can contain client-only references.

## Implementation constraints

Both writes require explicit user instruction and independently configured chat
permissions. Ordinary reads, Events processing and journal ack never invoke them.
Each intent needs a durable UUID/fingerprint, single dispatch claim, restart
recovery to unknown after possible dispatch, read-only status and no blind retry.
A replacement involving multiple writes needs persisted step outcomes and cannot
promise atomicity. A repeated UUID must not add another heart.

Native acceptance will use existing Sergio and an agreed test group, prepared
before asking for concrete actions and without expiring test windows. No working
Ann subscription changes or unrelated account writes are implied by research.

## Durable operation storage contract (implementation preparation)

The shared action ledger is local to the bound account. A canonical request UUID
is unique across both action kinds; the saved plan is one of `read`, `add`, `clear`,
`clear_add`. Kind, plan and conversation are checked independently of the
fingerprint. The fingerprint includes the exact typed
conversation, ordered targets and chosen action/reaction. Reusing it with different
arguments is a conflict. Prepare precedes dispatch; a compare-and-set claim permits
one dispatcher and serializes action writes within a conversation. Replays read
the saved result and never re-enter an attempted upstream step.

States: `pending`, `dispatching`, `accepted`, `rejected`, `unknown`, `partial`.
A dispatched-but-interrupted operation becomes `unknown` before serving requests
on restart. `accepted` means that the requested upstream step(s) were accepted,
not proof of UI state. An action stage is checkpointed before each upstream call.
Replacement, if native semantics require clear then add, records those steps;
failure after a confirmed clear is `partial`, with no automatic continuation.
A process failure during a step is `unknown`, retaining the completed-step count.
Acceptance requires exactly the saved plan's final step and count; a clear-only
replacement cannot be reported as accepted.
No body text, credentials, raw upstream response or protocol metadata is stored
in the ledger. Capacity is bounded; old UUIDs are not silently evicted and reused.

## Preparation verification

Synthetic factory tests cover both reaction/seen routes, exact IDs above 2^53,
chosen reaction and explicit NONE encoding, original counters and timestamp, and
zero writes for invalid input. Root provenance tests cover old-record
unavailability, typed identity, bound-account checks, recall/expiry/retention and
atomic migration rollback. Ledger tests cover concurrent claims, cross-kind UUID
conflicts, planned step ordering, clear-only replacement rejection, partial and
unknown results, and actual SQLite close/reopen recovery.

Preparatory independent review identified the clear-only acceptance gap; it was
fixed by persisting the plan and enforcing its terminal step. This is not final
acceptance: the final cross-transport/native coverage, installed release, CI and
publication remain outstanding; see the acceptance record for current gates.

## Historical disabled native probe candidate

A private `probe-message-action` CLI route is available only through the existing
owner-only Unix socket, never through MCP discovery. It accepts one primitive
`read`, `add` or `clear` with a stable UUID and exact retained targets. Its schema
is `message_action_probe.input.json`; composed replacement is deliberately not a
probe input. Permissions are independent `permissions.read_receipt_chats` and
`permissions.reaction_chats` typed allowlists, empty by default. These lists grant
capability, not permission to act automatically. Every invocation still requires
the user's explicit instruction.

The source target is re-resolved after the dispatch claim and before every step;
recall/expiry between clear and add prevents the add and records partial progress.
Probe contexts follow both service and active-session cancellation. Neither a
pending intent nor restart recovery dispatches work.

The complete 55-icon enum (including NONE) and 54 numeric types were compared
with pinned zca-js. One local literal bug was found: tears_of_joy contained an
extra backslash. Its exact `:')` encoding is now covered by a regression.

Primary sources do not define seenv2 status values. At that preparatory checkpoint the candidate therefore
preserved presence/numeric `upstream_status` and recorded an unverified nonnil
reply as `unknown / upstream_unverified`, not `accepted`. Synthetic status 1 is
only a fixture. Native sender-side observations and safe protocol evidence must
establish the accepted status and actual scope before enabling the public tool.
A previously unknown probe UUID is never automatically replayed or promoted to
success based on a later code change.

The worktree additionally provides `zalo://catalog/reactions` with the executable
`reaction_catalog.output.json` contract. Version 2 lists stable codes, names and display labels and
performs no native request, action preparation or automatic selection. Native
write semantics are stated in the explicit write-tool contract and acceptance record.


## Historical checkpoint after the direct case

The user observed the first of two incoming text markers become read after a
single-target direct receipt (`status=0`). The second became read only after the
chat was opened, so that observation does not establish per-message isolation or
absence of earlier-message watermark effects. At that checkpoint, the adapter accepted only a
present zero status on the direct route. Other statuses and the then-unverified group
route remained unknown. Existing unknown ledger entries remain unchanged.

Heart add, clear then like, and final NONE removal each had protocol acceptance
and independent sender-side confirmation. Repeating the heart UUID returned the
same saved result without native dispatch. Public `set` selects `clear_add`
internally, rather than exposing raw additive behavior. This is non-atomic; the
ledger preserves partial/unknown results. Accumulated reaction counts, media and
group behavior still required their respective cases, since completed below and
in the acceptance record.

## Subsequent native evidence

Direct and group status0 were each observed together with sender-visible read
state in approved cases. Group confirmation used a fresh target after the user
located read-details UI, with no deliveredv2 fallback. Present zero can therefore
mean protocol acceptance on both routes; this does not prove watermark isolation.
Existing unknown operations remain unchanged. See the dated acceptance record
for installed versus newer source classification.
