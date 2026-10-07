# Remaining conversation live acceptance

Status: prepared, not executed or authorized for a selected peer/receiver.
Follows the [current audit](conversation-completion-current-audit.md). The accepted
October 6 plain/quote acknowledgement trial is not repeated. No model evals.

## Required agreement

Select a controlled test recipient by an exact verified Zalo ID. Before the first
send it must be absent from the current catalogue, corpus, permanent message
identities and first-incoming facts. Name similarity does not establish identity.
An archive-only acquaintance is not necessarily a new Zalo contact; the tested
property is locally known incoming history, not first contact in real life.
Do not delete records, add a contact or reset novelty to manufacture eligibility.

Prepare a private exact send plan: recipient, text, fresh UUID and one-recipient
permission. Proposed text: `zl-mcp: проверка первой личной отправки.` Approval of
that concrete plan is required; the previous two-message approval does not apply.
Same-UUID retry belongs to that plan; no quoted or replacement send is needed.

Separately agree three temporary, conversation-scoped v2 subscriptions, a trusted
test client/HTTPS receiver, and a short restart window. The client supplies its
callback URL and signing material privately, performs verification and routes
by subscription ID. Credentials are never copied into this document or chat.
Production disallows loopback/private callbacks. Local TLS fixture receivers
prove synthetic tests only; they are not a replacement for a live receiver.
Do not reuse the production automation's callback unless its owner explicitly
supports independent test-subscription routing and processing.

## Exact subscription arguments

These are argument templates, not executable subscriptions. Replace the peer
placeholder only from the approved private plan. The event name is
`zalo.conversation.message.created.v2`; use `cursor=null`, `ttlMs=1800000` and
the trusted client's private `delivery`. Each subscription has its own returned
ID, generation and activation boundary.

Control C (proves the test source actually emitted each direction):

```json
{"scope":"conversation","conversation_type":"direct","conversation_id":"<approved-peer-id>","direction":"all","first_incoming_only":false}
```

Incoming I:

```json
{"scope":"conversation","conversation_type":"direct","conversation_id":"<approved-peer-id>","direction":"incoming","first_incoming_only":false}
```

First F:

```json
{"scope":"conversation","conversation_type":"direct","conversation_id":"<approved-peer-id>","direction":"incoming","first_incoming_only":true}
```

Record the existing production subscription definitions before activation. Never
edit, replace, reactivate or acknowledge them for this test. Subscribe through
the installed service; use no second listener, cloned live state or fake production
messages. Before messages, verify all three subscriptions and their empty journals.
If a subscription response is lost, reconcile returned/verified IDs and the active
catalogue; do not blindly create another subscription.

## Procedure and expected evidence

1. Preserve a verified state/config backup. Record the peer-absence/novelty
   preconditions and private send/subscription plans. If the peer is already
   known, retain that fact and choose another agreed peer; do not erase history.
2. Activate C/I/F and record successful verification, returned IDs, generations,
   activation boundaries and expiries. Coordinate the test recipient to wait
   until readiness. Unexpected earlier incoming/replay must be recorded; it can
   invalidate the intended first-send/first-incoming precondition.
3. Send approved outgoing Q through MCP. Record upstream status and genuine ID.
   Repeat only the same approved UUID/arguments; require the identical receipt.
   `unknown`/`failed` does not permit a fresh replacement UUID. A distinguishable
   upstream restriction is a documented outcome, not permission to add friendship.
4. Have the controlled recipient send incoming A. Record its genuine message ID
   and the stored first-incoming fact. C must include Q and A; I includes A only;
   F must contain the actual first locally known incoming record exactly once.
   If a genuine earlier replay became the first record, record that source/time
   honestly rather than relabel A or reset the fact.
5. Read test journals by exact IDs, compare signed callback envelopes with their
   journal event/message identities and expected direction/first fields. Q must
   appear in C and must create no delivery/journal record for I or F. A missing
   I/F callback proves exclusion only after C/source persistence is established;
   timeouts, source silence or HTTP errors are inconclusive.
6. Restart the same installed Go service in the agreed window, with no other
   listener. Re-read existing test records and compare event IDs/order,
   generations, activation boundaries and novelty facts. Replay must not create
   another first fact or duplicate record. Read the saved Q status; never resend
   with a new UUID. Preserve the existing production subscription/ledger.
7. Have the recipient send incoming B after recovery. Its first-incoming flag must
   be false for a known fact; C/I receive B, F does not. A genuine null/unknown
   flag must never pass F, but manufacturing uncertainty is not a live test.
   Synthetic null cases remain separate evidence if none occurs naturally.
8. Distinguish persistence, callback acceptance and client processing. Test client
   checkpoints only its own C/I/F records after successful processing; read-only
   diagnostics never acknowledge on behalf of the production automation. A
   notification-before-ack crash can duplicate notices and is not exactly-once.

## Cleanup and acceptance

In finally/expiry reconciliation, cancel only the captured C/I/F IDs. Verify no
test subscription remains active, and restore the exact original sending policy.
Do not restore an old corpus over accepted send receipts. Compare prior production
subscription definitions and preserve all new messages/identities, send results
and first-incoming facts. Record current authentication, collector connectivity
and integrity. No phone synchronization or mobile history import is part of this
scenario; the permanent captured archive remains unchanged.

Publish only anonymized outcomes: exact checks passed/failed/inconclusive, counts,
versions and limits. Private plans, real IDs, messages, URLs, signing keys and
callback bodies stay outside Git. A synthetic test, arbitrary wait or empty journal
cannot close a missing live scenario. If a recipient/receiver is unavailable,
leave the precise requirement pending instead of repeating accepted send trials.
