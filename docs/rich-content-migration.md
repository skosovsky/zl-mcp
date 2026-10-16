# Migrating a message consumer to v3

Status: operator procedure; the working v2 consumer has **not** been switched.
V3 is a new subscription contract, not an in-place edit of signed v2 payloads.
See [the content contract](contracts/rich-content-events.md) and
[the candidate deployment evidence](rich-content-candidate-deployment.md).

## Preconditions

Use actual discovery from the client. The consumer must handle v3 content,
metadata resources and the exact subscription journal. Preserve the approved
scope/direction/first-incoming filters, recipient and notification rule. Selecting
v3 does not authorize reactions/calls; those require separate rules/subscriptions.
Do not widen a direct-only rule to all conversations.

A trusted consumer/harness needs durable processing state keyed by notification
rule plus `(conversation_type, conversation_id, message_id)` across old/new
subscriptions. A prompt or an in-memory set is insufficient. Serialize processing for that
consumer rule, or atomically claim/lease each typed source identity before an
external action. Two old/new triggers must not both observe an uncompleted source
and act concurrently. A counterpart record is acknowledged only after the shared
completed outcome is durable; an in-progress claim is not completion. After a
crash, recover the recorded phase and resolve any ambiguous external action before
retrying it: lease expiry alone does not authorize another notification. Do not assume every
event family shares message identity or eventId. Each subscription retains its
own ordered receipts; acknowledging one does not acknowledge another.

Callback acceptance, journal acknowledgement and user-visible action are separate
facts. A crash after an external notification but before its durable receipt can
still duplicate that action unless the destination supports the same idempotency
key. This procedure reconciles the normal cutover; it does not promise an atomic
external exactly-once transaction.

## Overlap and reconciliation

1. Save the old definition/generation, consumer rule and processing checkpoints
   privately. Account for the old backlog and notifications already completed;
   do not replay them merely because v3 is available.
2. Create the new v3 subscription while the old remains active. Use the same
   approved filters. Record both identities and their activation boundaries.
   Validate its actual callback, journal and content resource before changing the
   notification consumer. Neither historical timestamps nor a temporarily empty
   callback queue establish the insertion boundary.
3. During overlap, one durable consumer rule reconciles both message streams by
   typed source identity. For a source already completed, account for its other
   subscription record without sending another notification; acknowledge each
   exact receipt only after that accounting is durable. Keep prefix ordering.
   Unavailable/gap records require explicit accounting, not guessed content.
4. Establish the **pre-v3 barrier** before cancelling old delivery. The trusted
   local operator records the new subscription's `start_seq` and verifies the
   message `event_fanout.seq` as W and new `start_seq` as B in one consistent
   read snapshot. Prove there is no unfanned `message_events` row with
   `W < seq <= B`. `W >= B` is sufficient but not required: silent history
   insertion and ignored/duplicate AUTOINCREMENT allocations can leave B above W
   without any missing event. Do not interpret their numeric difference as a loss
   count. All old work at or before B must have a terminal, accounted-for outcome
   and completed consumer processing. An empty delivery queue alone is
   insufficient: an unfanned source could otherwise be lost on cancellation.
   Do not use the activity sequence for this message boundary.
5. Reconcile overlapping sources after the boundary and switch the consumer rule
   to v3. Only then cancel the old subscription. Messages inserted concurrently
   after the new boundary remain eligible for v3. Recheck the new subscription's
   backlog/checkpoints and preserved source identities; do not undo receipt state
   or blindly recreate the old subscription as a fallback.

The barrier uses owner-only read access to subscription activation/fanout metadata;
it does not grant an ordinary event-handling skill access to SQLite or credentials.
Current public read tools do not expose that complete barrier. If the trusted
operator/client cannot prove it or lacks cross-profile processing state, keep the
old subscription and prepare the consumer first. Do not claim a gap-free cutover.
No automated consumer cutover tool is supplied by this candidate.

## Reversal and rollback

Before reversing the consumer choice, reconcile actions performed through v3 with
the old rule and its exact retained state. A new old-profile subscription starts
at a new boundary; it does not recover an interval silently. Preserve all new
message and processing facts.

A binary downgrade is a separate operation. Unload the service, preserve current
state and apply the read-only [rollback gate](contracts/rich-content-events.md#rollback-precondition-and-proof).
Active new profiles, unsupported unprocessed work, unfanned activity or retained
activity payload block a pre-rich binary. Keep the current runtime if the gate
refuses; an older stopped backup is not a lossless replacement for current data.
