# Local callback delivery acceptance

Date: October 15, 2026. Scope: explicit same-host callback delivery and its
[executable contract](contracts/local-callbacks.md). This does not close the
remaining fresh-account live acceptance gates.

The zero/default configuration retains public HTTPS-only callbacks. Exact
operator-configured HTTP/HTTPS loopback URLs are enforced before subscription
activation, at delivery and in the transport. The local transport bypasses DNS
and environment proxies. HTTPS certificate verification and redirect rejection
remain enabled; HTTP loopback does not provide encryption.

Verified by autonomous checks:

- Canonical literal IPv4/IPv6 targets, explicit ports and exact URL matching;
  rejection of hostnames, alternate loopback representations, LAN/public
  allowlist entries, encoded paths, dot segments, queries and duplicate entries.
- The real configured MCP HTTP endpoint activates a local v2 subscription with
  a signed challenge. An independent loopback HTTP receiver checks the HMAC and
  exact event/message ID and text. No injected callback transport, DNS override,
  public endpoint or external tunnel is used.
- Queued callbacks are not sent after the exact URL permission is removed during
  endpoint reconfiguration. Subscription cancellation remains available then.
- Default mode rejects loopback, exact path/authority mismatches never reach a
  local server, redirect targets receive no requests and an untrusted local TLS
  certificate is rejected.

Full local `make check` passed: lint, root/harness/zcago tests, race and vet,
architecture host/all-build boundaries and negative controls, CGO-free macOS
arm64/amd64 and Linux amd64 builds. The mandatory offline synthetic profile
passed 50/50 cases and 480/480 assertions; model and live profiles were not run.
No real Zalo listener, send, history import, production subscription edit or
journal acknowledgement was used by these checks.

## Installed backend and independent live preflight

The installed service was upgraded from a stopped, verified private backup to
clean signed source `754f91056f853c8bc88a73821bef4a58a643e225`. The ad hoc signed
arm64 binary SHA-256 is
`09f82dfdfd0c9f57b349007768bda2ba83501e009751f98a369ecb5f387a666a`.
The exact candidate passed native terminal startup acceptance. Its
[Ubuntu/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/38697162754)
completed successfully. The original single LaunchAgent remains unchanged.
All pre-upgrade rows in eight protected corpus/identity/novelty/quote/send/
subscription/processing tables were verified preserved after restart, and SQLite
quick_check passed. The collector reconnected without authentication errors.

The only configuration addition is one exact HTTP loopback URL for a private,
temporary Go receiver. It rejects an invalid HMAC and independently verifies the
real subscription challenge. One v2 subscription targets only the existing
controlled direct peer, `direction=incoming`, `first_incoming_only=false`, with
a 30-minute TTL. The actual connected MCP client sees the additional subscription
and its initially empty journal. The original public web subscription definition,
generation, activation boundary and callback/signing configuration are unchanged.

The user sent the agreed unique marker once. Exactly one matching message arrived
through `live`, sent at `2026-10-15T07:27:58.329Z` and persisted at
`07:27:59.276254Z`. One callback was accepted on attempt one at
`07:27:59.611809Z`. The independent Go receiver recorded a valid HMAC and the exact
persisted callback envelope, with the correct message/event/subscription identity,
marker text, `direction=incoming` and `first_incoming=false`. The previously known
first-incoming fact was preserved. No tunnel or assistant notification was used
as delivery evidence.

The actual connected MCP client read the same exact event/message identity from
the local subscription journal. After checking the receiver evidence, it
acknowledged only that test record. Repeating the same receipt was idempotent;
the next read was empty, without a delivery block or continuation. No production
journal record was acknowledged. The web subscription definition remained
unchanged. Private IDs, payloads, signing keys and callback URLs remain outside Git.

A controlled direct HTTP replay of the exact persisted live payload, with its
same event ID and a fresh valid signature, returned 204 and did not add another
receiver record. The same actual payload with an invalid signature returned 401.
There is still exactly one receiver event record. This proves receiver-side
deduplication of this event; it is a deliberate local replay, not evidence of a
spontaneous delivery retry or another upstream Zalo send.

This closes the known-peer local signed-delivery and processing-checkpoint
preflight, including retention of the existing known-peer novelty fact across the
actual backend upgrade/restart. It does not prove outgoing exclusion, first-only
delivery or the genuine fresh-account first/repeat scenario. The sole fresh test
account remains reserved.

## Test lifetime correction

The user subsequently required test windows without time limits. The initial
finite subscriptions had already expired and the receiver's original stop timer
had fired; no outgoing marker was present in the local corpus during that gap.
The same receiver address, private keys, verified subscription bindings and
deduplication evidence were resumed with no automatic shutdown. Both existing
known-peer `all`/`incoming` test subscription identities were reactivated with
`ttlMs=null`, creating new generations/boundaries as required after expiry.
Historical window metadata and processing records were retained privately;
no earlier corpus was replayed into the new journals. Both journals were empty
and unblocked through the actual MCP client, with an authenticated/connected
collector. The production web subscription remained unchanged. Cancellation,
rather than a user response deadline, owns subsequent test cleanup.

## Direction-filter trial: incoming control, outgoing still pending

The next marker was named for the intended outgoing trial, but its name did not
establish direction. The actual live record was sent at
`2026-10-15T08:12:21.048Z` and persisted at `08:12:21.125296Z`. Its sender matched
the controlled peer rather than the bound collector account. The service recorded
`direction=incoming`, `first_incoming=false`.

Both the `all` and `incoming` test subscriptions delivered that record on attempt
one. The independent receiver verified each signature and exact persisted
payload; the actual MCP client read the matching event from each journal. This
is a successful incoming positive control, not an outgoing-exclusion result.
The production web subscription definition remained unchanged. No outgoing send
was performed by the assistant. The two journal records had not yet been
acknowledged at this observation.

The test instruction/interpretation was incorrect, not the observed MCP routing.
The remaining direction check requires a distinct message from the bound main
account to the controlled peer. Acceptance requires a stored live outgoing
record, its signed delivery to `all`, and no corresponding event in `incoming`
after fanout has processed the source record. A quiet journal alone does not
prove exclusion. Keep both test subscriptions without an expiry; preserve the
fresh account for its separate first-contact tests.

## Outgoing direction filter accepted

The user sent a distinct marker from the bound main account to the same peer at
`2026-10-15T08:22:23.428Z`; the collector persisted it at `08:22:23.609833Z`
through `live`. Fanout had processed its source sequence. The `all` subscription
created exactly one delivery, marked `outgoing`, accepted on attempt one. The
independent receiver verified its signature and exact persisted envelope.
The `incoming` subscription created no delivery for this message.

The connected MCP client independently returned the outgoing event in `all`
and no corresponding event in `incoming`. Both journals were unblocked without
continuation. The earlier incoming control remained visible in both journals,
so its presence was not mistaken for an outgoing leak. Collector status was
connected/authenticated without an error. This closes the known-peer outgoing
exclusion check; first-only/fresh-account acceptance remains separate.

After accounting for both test markers, only their local journal receipts were
acknowledged in order. Subsequent reads of both test journals were empty and
unblocked. No production journal acknowledgement was performed.

## Known-peer first-only exclusion prepared (result pending)

A third exact loopback receiver route was allowlisted after a stopped, verified
private state/configuration/binary backup. The same Go binary was restarted once.
All prior subscription definitions, processing records and peer novelty facts
were verified preserved. The production web subscription was not edited.

The additional subscription targets only the controlled known direct peer,
`direction=incoming`, `first_incoming_only=true`, `ttlMs=null`. Its real challenge
signature was verified by the independent receiver. The connected MCP client
confirmed the active subscription and an empty, unblocked journal; collector
status was connected/authenticated without an error. The peer already has a
persisted first-incoming fact. Existing `all` and `incoming` controls remain
active without expiry. Receiver lifetime is until explicit stop.

The pending user action is an incoming marker from the controlled peer to the
main account. Acceptance requires the same live message in both control
subscriptions, `first_incoming=false`, and no delivery in the first-only
subscription after source fanout. Preparation alone does not pass this test.
The sole fresh account has not been used by this preparation.

## Known-peer first-only exclusion accepted

The prepared trial subsequently completed: one matching live incoming message
from the controlled peer was sent at `2026-10-15T08:40:02.270Z` and persisted at
`08:40:04.214049Z`. Source fanout had completed. Both `all` and `incoming`
subscriptions delivered exactly one event on attempt one with
`direction=incoming`, `first_incoming=false`. Each delivery used its current
generation; the independent receiver verified the signature and exact persisted
envelope. The active, unexpired first-only subscription created no delivery for
that source message.

The actual MCP client confirmed the matching event in both control journals and
an empty first-only journal, all unblocked without continuation. After processing
the two local test records, their receipts were acknowledged and both control
journals read empty and unblocked. Collector status was connected/authenticated
without an error. No production acknowledgement or fresh-account interaction
was performed. This passes exclusion of a known peer's repeated incoming;
positive delivery of a genuinely fresh peer's first incoming remains pending.

## Fresh-account first incoming prepared (result pending)

Three additional direct-wide subscriptions, each with its own independent local
receiver route and signature key, were activated without expiry: all directions,
incoming, and first incoming only. Their real signed verification challenges
passed. The actual connected MCP client confirmed all three active with empty,
unblocked journals. Collector status was connected/authenticated without errors.
The receiver has no automatic shutdown. Prior subscription definitions,
processing records and novelty facts survived the verified backup/configuration
restart unchanged; the production web subscription was not edited.

A private pre-message snapshot preserves catalogue/corpus/permanent-identity/
directory/novelty evidence for comparison against the genuine sender ID once
received. Exact-ID absence is not asserted before that comparison. No fresh-peer
lookup, send, friendship, history preload or reset was performed. Await exactly
one user-sent first marker before any follow-up or reply; first-only positive
delivery and fresh-account acceptance remain unproven until receipt.

## Fresh-account first incoming accepted

Exactly one agreed marker arrived through `live`, sent at
`2026-10-15T08:46:16.761Z` and persisted at `08:46:17.462863Z`. Comparing its genuine
sender/conversation ID with the private pre-message snapshot confirmed absence
from catalogue, corpus, permanent message identities, directory and novelty
facts before the trial. The new direct conversation was subsequently available
through the actual MCP catalogue with collection enabled and metadata source
`live`. Friendship remained `unknown`; this does not independently prove the
upstream application's Strangers classification.

All three direct-wide subscriptions delivered exactly one event on attempt one
with `direction=incoming`, `first_incoming=true`. Receiver HMAC verification and
exact persisted envelope comparison passed for every delivery. Actual MCP
journals returned the same event/message/sender identity in all three cases,
without blocking or continuation. Source fanout completed, and the saved known
first-incoming sequence matches this source message. Collector status was
connected/authenticated without an error; prior subscription definitions were
unchanged.

The three test journal records remain deliberately unacknowledged for the
subsequent restart/recovery comparison; no production journal was acknowledged.
This passes genuine locally new-peer discovery and first-only positive delivery.
Restart persistence and the same peer's second-message exclusion are pending;
no second message, outgoing reply or additional fresh account is required yet.

## Fresh-account restart recovery accepted; second incoming pending

The user authorized a restart before the second message. The sole installed Go
service was stopped, its state checkpointed and backed up with verified file
hashes and SQLite integrity, then started with unchanged binary/configuration.
A different process ID confirmed the restart. Existing subscription definitions,
processing records, permanent identities, novelty facts and send-operation
records were preserved. The three test subscriptions retained their generations,
activation boundaries and indefinite lifetimes. The peer's known first-incoming
sequence was unchanged; each original delivery retained its event ID, state and
attempt count, with no duplicate delivery for the source message.

After restart, the actual MCP client recovered exactly the original first event
from each of the three journals with matching event/message IDs and
`first_incoming=true`. Collector status was connected/authenticated without an
error. Only after this recovery comparison were the three processed test
receipts acknowledged; all three journals then read empty and unblocked.
No production journal was acknowledged. The local receiver remains running.

The next prepared user action is one distinct incoming marker from the same peer.
It must produce `first_incoming=false` and delivery in `all`/`incoming` only.
Second-message exclusion for this genuinely new peer is still pending receipt.

## Fresh-account second incoming after restart accepted

Exactly one agreed second marker from the same genuine peer arrived through
`live`, sent at `2026-10-15T09:00:15.102Z` and persisted at `09:00:16.119379Z`.
Source fanout completed; the original known first-incoming sequence was unchanged.
Both `all` and `incoming` delivered once on attempt one in their original
generations with `direction=incoming`, `first_incoming=false`. Each receiver
record passed HMAC and exact persisted-envelope verification. The still-active,
unexpired first-only subscription created no delivery for this source message.

Actual MCP journals independently confirmed the same second message in both
controls and an empty first-only journal, without blocking or continuation.
The two processed test receipts were acknowledged; control journals then read
empty and unblocked. Collector status was connected/authenticated without an
error. This completes the fresh peer's first incoming, restart recovery and
subsequent incoming exclusion sequence. No outgoing send or production journal
acknowledgement was performed. Subscriptions remain active until explicit
cancellation; this result does not close unrelated acceptance requirements.

## Completed test cleanup

After acceptance, the six exact local test subscriptions were cancelled through
authenticated MCP. The production subscription was excluded by ID and callback
identity checks. Its full definition and processing checkpoint were verified
unchanged. Both independently identified Go test receivers were stopped, with
private evidence retained. All six exact callback URL permissions were removed;
the local allowlist is empty, restoring default public HTTPS-only delivery.

A stopped-service, hash-verified backup preceded the configuration restart.
The installed binary and LaunchAgent were unchanged. After restart, actual MCP
status was connected/authenticated without errors and listed only the original
production direct/all subscription. All six test subscriptions were inactive;
SQLite quick_check passed. First/repeat messages, novelty facts and test evidence
remain preserved. Indefinite test lifetime ended by explicit scenario cleanup,
not expiry or a user-response deadline. No new send or phone sync was used.

### Original known-peer state reconciliation

The earlier authorized artificial reset had left the known peer's first-incoming
fact pointing to a test record. The original private backup was reconciled after
test cleanup. All 17 original message identities were present; all 16 original
quote-metadata tuples already matched, so no messages or quote rows were written.
The permanent identity still retained the original first sequence.

After a stopped-service, hash-verified backup, exactly that peer's known novelty
fact was restored to its original sequence/certainty. Message, permanent-identity,
quote, send, subscription, delivery and processing tables were compared unchanged
across this write. New test data and the fresh peer's novelty evidence remain
preserved. After restart, original-fact equality and SQLite integrity passed;
the production subscription definition and processing checkpoint were unchanged.
Initial client calls failed during readiness; a later actual MCP status returned
connected/authenticated without an error, without another restart in response
to those observation failures. No send or phone synchronization occurred.

## Approved local-absence send accepted

The user separately approved the exact [local-absence plan](remaining-live-send-plan.md),
including temporary removal, one plain text, its stable request UUID, restoration
and disabling sends. This is an explicit exception to the live plan's no-reset
default. It does not claim first upstream contact or genuine local novelty.

Preparation stopped safely before any deletion/send when an omitted default
permission field was not handled, when initial preload outlasted the diagnostic
wait, and when optional startup priority tuning was checked too early. These
maintenance-helper defects were corrected; no replacement request UUID or test
message was spent. Every aborted preparation restored disabled sending.

The successful run made a stopped-service, hash-verified full private backup,
temporarily enabled sending to only the exact approved peer, and used the same
installed Go binary/single LaunchAgent. After preload completion, one transaction
removed exactly the two approved corpus rows, their two quote-metadata rows and
the direct conversation catalogue row. Permanent identities, novelty evidence,
send/event/subscription journals were retained. The actual MCP catalogue was
paged and did not contain that exact peer; direct SQL immediately before the
tool call independently confirmed empty typed corpus and catalogue.

The installed HTTP MCP `zalo_send_direct_message` returned `sent` with a genuine
message ID. Retrying the exact same recipient/text/UUID returned the identical
receipt. The retained ledger contains exactly one new send operation for this
peer, and exactly one outgoing corpus record with the accepted message ID.
This is upstream acceptance and idempotent saved-result recovery, not read
receipt or recipient acknowledgement.

Both original messages and quote tuples were restored at their exact sequences;
new outgoing data and newer catalogue observations were preserved, with the
original known name filling an otherwise absent name. Original novelty and
permanent identities were verified unchanged. The original configuration,
LaunchAgent and binary matched the backup byte for byte after cleanup; sending
was disabled again. Temporary process-priority relaxation did not change the
agent's Background policy and ended with the maintenance process restart.

After the cleanup restart, the saved receipt was identical. The actual connected
MCP client independently read that same request/recipient/message result and
found the exact restored conversation. Status was connected/authenticated with
no error; SQLite quick_check passed. Prior send receipts and subscription
definitions were preserved. No production journal acknowledgement, phone sync,
friendship change or additional outgoing UUID was used.

This accepts the user-approved artificially absent catalogue/corpus scenario.
It does not retroactively turn the earlier incoming peer into a never-observed
account, prove sending to a genuinely new upstream stranger, or enable mobile
archive import.
