# Remaining conversation live acceptance

Status: known-peer direction/repeat filters and genuine fresh-peer first incoming,
restart recovery and second-incoming exclusion passed. Six local test
subscriptions were cancelled and both receivers stopped after acceptance.
The separately approved [artificial local-absence send](remaining-live-send-plan.md)
also passed; original rows/configuration were restored and sending disabled.
See [local callback acceptance](local-callback-acceptance.md) for executed evidence.
Follows the [current audit](conversation-completion-current-audit.md). The accepted
October 6 plain/quote acknowledgement trial is not repeated. No model evals.

The user subsequently authorized a controlled existing-chat history/novelty reset.
Its exact preservation and partial results are in the
[October 15 checkpoint](conversation-completion-2026-10-15.md). That variation
does not meet the outside-catalogue first-send precondition below, authorize fresh
sends or prove first-only/incoming-only subscription filtering. The no-deletion
rule below applies to an unapproved attempt to manufacture a new peer, not that
separately approved reset. Do not repeat the reset or silently broaden its scope.

## Fresh-account priority (October 15)

The user has one newly registered test account. Preserve its genuinely first
incoming contact: do not send from either account, add friendship, seed the local
conversation catalogue or trigger history preload for this peer before the
receiver and subscriptions are ready. Read-only identity verification must not
create a conversation or first-incoming fact. Do not request another new account.

The sequence below prioritizes the first incoming/Strangers scenario over the
outside-catalogue first outgoing scenario. Once incoming A is stored, subsequent
outgoing messages are replies to a known peer. They cannot prove first sending
to an unknown peer. A separately approved local reset could test local catalogue
absence later, but cannot restore genuine upstream first-contact conditions.

That separate local-reset exception was subsequently explicitly approved and
executed with the existing controlled peer. Its accepted result covers absent
local catalogue/corpus at send time, not genuine upstream first contact. The
no-reset defaults below remain in force for any additional unapproved attempt.

## Required agreement

Select a controlled test recipient by an exact verified Zalo ID. Before incoming
A it must be absent from the current catalogue, corpus, permanent message
identities and first-incoming facts. Name similarity does not establish identity.
An archive-only acquaintance is not necessarily a new Zalo contact; the tested
property is locally known incoming history, not first contact in real life.
Do not delete records, add a contact or reset novelty to manufacture eligibility.

Prepare a private exact send plan only for the later outgoing check: recipient,
text, fresh UUID and one-recipient permission. Approval of
that concrete plan is required; the previous two-message approval does not apply.
Same-UUID retry belongs to that plan; no quoted or replacement send is needed.

Separately agree three temporary, conversation-scoped v2 subscriptions, a trusted
independent test receiver, and a short restart window. The client supplies its
callback URL and signing material privately, performs verification and routes
by subscription ID. Credentials are never copied into this document or chat.
Use the [explicit local callback mode](contracts/local-callbacks.md) on the same
Mac once the verified build and exact URL allowlist are installed. Public HTTPS
remains an alternative. Test-only injected TLS transports prove synthetic tests
only; actual live delivery must use the configured production transport.
Do not reuse the production automation's callback unless its owner explicitly
supports independent test-subscription routing and processing.

Receiver infrastructure check (October 15): the installed OpenAI Tunnel profile
forwards MCP requests to the existing service. It is not an arbitrary public
webhook ingress for a separate local test receiver. The
[official tunnel documentation](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)
describes queued MCP JSON-RPC forwarding; its allowlisted HTTP callout feature is
not a general-purpose proxy. No tunnel configuration or access was changed.
The user selected independent subscription/receiver evidence rather than the
assistant's selective user notifications. Provision the local signed receiver and
subscribe through authenticated MCP HTTP; no automation or Ann handoff is needed.
Local mode requires an upgraded service and exact operator allowlist; its presence
in source is not proof of deployment or of created live subscriptions.

The user agreed to three test direct-wide subscriptions when the fresh peer ID
is unavailable, and later explicitly removed test-window deadlines. That consent
does not make them active. For the
initial existing-peer preflight, narrower exact conversation scopes suffice.

The subsequently completed fresh-account trial used three distinct signed local
receiver routes with direct-wide subscriptions (`scope=direct`): `direction=all`,
`direction=incoming`, and `direction=incoming, first_incoming_only=true`, all
with `ttlMs=null`. Direct-wide arguments omit conversation type/ID. Actual MCP
reads confirmed all three active and their journals empty and unblocked;
collector status was connected/authenticated without errors. Existing production
and known-peer subscriptions, processing checkpoints and novelty facts were
preserved through the single configuration restart and verified state backup.

The exact fresh peer ID was unknown before its first event. A private
account-wide snapshot of catalogue, corpus, permanent identities, directory and
novelty records was saved before readiness. Comparison with the first marker's
genuine sender ID subsequently confirmed absence from those sources. This
evidence does not rely on a display-name lookup. Before the first message, no
request, friendship, history preload, send or state reset was performed against
the fresh account.

The user supplied the fresh account's phone privately. Official Zalo UI lookup
found a profile marked STRANGER, joined today, with no mutual groups. Searching
its displayed name in the local MCP catalogue returned no matches. This is
useful preliminary evidence, not proof of absence by exact peer ID from all
identity/novelty tables. No send or friendship action occurred. Private phone,
name and eventual peer ID must remain outside this public plan.

## Exact subscription arguments

These are argument templates, not executable subscriptions. Replace the peer
placeholder only from the approved private plan. The event name is
`zalo.conversation.message.created.v2`; use `cursor=null`, `ttlMs=null` and
the trusted client's private `delivery`. Each subscription has its own returned
ID, generation and activation boundary.

The user explicitly removed time limits for test windows. Test subscriptions run
until explicit cancellation; the independent local receiver must not stop on a
timer. Network request timeouts and signature freshness checks still apply. An
already expired subscription cannot retain its old activation boundary when
reactivated: archive the old generation/checkpoint evidence and record the new
boundary honestly. Do not replay prior corpus or reuse silence during an expired
window as evidence of direction filtering.

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
   known, stop and report which precondition failed; do not erase history or
   assume another new account is available.
2. Activate C/I/F and record successful verification, returned IDs, generations,
   activation boundaries and indefinite lifetime. Coordinate the test recipient to wait
   until readiness. Unexpected earlier incoming/replay must be recorded; it can
   invalidate the intended first-incoming precondition.
3. Have the controlled recipient send exactly one incoming A, only after an
   explicit readiness signal. Record catalogue discovery, conversation type,
   sender identity and whether Strangers classification is actually observable.
   Do not infer friendship or Strangers membership from a successful ingest.
4. Record A's genuine message ID and the stored first-incoming fact. C/I include A;
   F must contain the actual first locally known incoming record exactly once.
   If a genuine earlier replay became the first record, record that source/time
   honestly rather than relabel A or reset the fact.
5. Read test journals by exact IDs, compare signed callback envelopes with their
   journal event/message identities and expected direction/first fields. A missing
   I/F callback proves exclusion only after C/source persistence is established;
   timeouts, source silence or HTTP errors are inconclusive.
6. Restart the same installed Go service in the agreed window, with no other
   listener. Re-read existing test records and compare event IDs/order,
   generations, activation boundaries and novelty facts. Replay must not create
   another first fact or duplicate record. Preserve the existing production
   subscription/ledger.
7. Have the recipient send incoming B after recovery. Its first-incoming flag must
   be false for a known fact; C/I receive B, F does not. A genuine null/unknown
   flag must never pass F, but manufacturing uncertainty is not a live test.
   Synthetic null cases remain separate evidence if none occurs naturally.
8. Only after A/B, send approved outgoing Q through MCP. Record upstream status
   and genuine ID. Retry only the same approved UUID/arguments and require the
   identical receipt. Q must appear in C and create no delivery/journal record
   for I/F. This tests direction filtering and idempotency for a now-known peer,
   not outside-catalogue first sending. An unknown/failed result does not permit
   a replacement UUID or adding friendship.
9. Distinguish persistence, callback acceptance and client processing. Test client
   checkpoints only its own C/I/F records after successful processing; read-only
   diagnostics never acknowledge on behalf of the production automation. A
   notification-before-ack crash can duplicate notices and is not exactly-once.

## Cleanup and acceptance

At scenario cleanup, cancel only the captured C/I/F IDs. Verify no
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
