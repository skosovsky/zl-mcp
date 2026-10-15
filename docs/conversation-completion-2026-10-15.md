# Conversation completion: October 15 checkpoint

Status: partial live acceptance. The full [task](task-conversation-completion.md)
remains open. This checkpoint supersedes older installation descriptions, not
the remaining requirements. No model evals were run for this checkpoint.

## Installed backend

On October 14, the existing single macOS service was upgraded from `8032172`
to clean commit `6161075565affe2992d12928f2c75481a717b141`, built with Go 1.27.2,
CGO disabled and trimpath. Installed arm64 SHA-256:
`5e6cc81694bd2d8f678c35af1a887f9a62c3aa0b688f0c83fa49655ddc761e2a`.
The candidate was ad hoc signed and its signature verified.

Publication sanitation maps that original code commit to
[`b393282`](https://github.com/skosovsky/zl-mcp/commit/b393282c28d935d6e74bbdba8463a8125871b863).
Their production sources and executable contracts are identical. Installed VCS
metadata remains the original `6161075`; rewriting documentation/history does not
change the running binary or justify restarting it. See the
[complete original/sanitized map](publication-history-map.json).

The focused `go test` command passed for `internal/service` and
`internal/mcpserver`; `cmd/zl-mcp` reported no test files.
The exact candidate passed `TestNativeBinaryTerminalStartupAcceptance` with
temporary synthetic state. These focused deployment checks do not constitute
a fresh full root/nested race/vet/lint or model evaluation run.

After a stopped-service, hash-verified private backup and atomic replacement,
the same LaunchAgent resumed. Configuration and LaunchAgent contents were
unchanged. Messages, identities, tombstones, quote metadata, first-incoming facts,
send receipts, subscription activation boundaries and the permanent archive
were preserved at deployment. SQLite quick_check passed.

Initial reads timed out during unusually high host load and upstream restore
reported transport failures. Temporary process background-policy relaxation
was reverted. Subsequent direct HTTP initialize, tools/list and calls to status,
conversation listing and archive inventory all succeeded in the original policy;
22 tools, 82 stored messages and a 56-file permanent source were observed.
The connected MCP client independently returned authenticated/connected with
no last error. No persistent scheduling change or tunnel change was made.

## Authorized first-incoming reset

The user explicitly allowed deletion of the existing controlled test chat's local
history instead of requiring another peer. Before resetting, the service was
stopped and the entire state, binary, config and agent were backed up privately.
Seventeen message rows, sixteen associated quote-metadata rows and that peer's
one first-incoming fact were removed atomically. Other messages and novelty
facts, permanent deduplication identities, send receipts, subscriptions, delivery
records and the captured archive were preserved.

After restart, upstream preload restored the same seventeen message identities;
there were no new message IDs or Events from this restoration. The first-incoming
fact remained absent. This is an explicitly artificial novelty-state reset for
an existing chat. It does not prove that the peer is a new Zalo contact or absent
from the catalogue. Keeping deduplication prevents known historical replay from
being misclassified as a fresh test message.

## Observed live result

As of the October 15 check, exactly one post-reset incoming message with a new
identity was stored through the live listener. The saved first-incoming fact
points to that message with certainty `known`. Its v2 callback envelope agrees
with the corpus message identity and contains `direction=incoming` and
`first_incoming=true`. The callback was accepted on its first delivery attempt.
The collector remains authenticated/connected, without a last error; the corpus
contains 83 messages. Private message content, peer/message/event IDs, callback
URLs and credentials are intentionally omitted from this report.

The active subscription is still `scope=direct`, `direction=all`,
`first_incoming_only=false`. Callback acceptance is not proof of model processing
or of first-only/incoming-only exclusion. Diagnostics did not acknowledge events
or mutate subscriptions.

## Remaining evidence

### Known-peer delivery preflight, October 15

The user selected an existing controlled direct peer for a preflight before using
the sole fresh account. A read-only baseline captured the exact peer, known
first-incoming fact, subscription generation/start boundary, corpus/delivery
high-water marks and processing checkpoint. The real client saw one active
all-direction direct v2 subscription and two older unprocessed delivered records.
No subscription was changed and no record was acknowledged by diagnostics.

The user then sent the agreed unique marker once from that peer. Exactly one new
matching message was stored from `live`, with matching sender/peer identity:
sent `2026-10-15T06:42:48.537Z`, received `06:42:49.856249Z`. Its exact message ID
matches one callback payload with `direction=incoming`, `first_incoming=false`
and the full marker text. Delivery completed on attempt one at
`06:42:54.114311Z`. The prior first-incoming fact and subscription generation/start
boundary are unchanged. Collector status remains authenticated/connected without
errors. This accepts this message's ingestion, repeat-peer classification and
callback acceptance; it is not a filtered-subscription trial.

The actual connected MCP client independently recovered the same marker and
flags through the exact subscription journal. There are now three unprocessed
delivered records, including the two older records, no delivery block and no next
page. The processing checkpoint has not advanced relative to baseline. Actual
assistant notification and processing acknowledgement remain unconfirmed; neither
HTTP acceptance nor journal readability proves them. The fresh account was not
used. Private peer/message/event IDs and callback credentials are omitted.

- The known-peer preflight above now confirms another new incoming with
  `first_incoming=false`. The post-first-message agreed restart trial remains
  open: after restart the fact must remain known and another incoming must not
  become first again. Do not reset repeatedly
  merely to repeat the already accepted first=true observation.
- Real incoming-only and first-only delivery require separately agreed temporary
  scopes and a client/receiver that processes those exact subscriptions. The
  existing all-direction subscription cannot prove exclusion.
- First sending outside the catalogue remains a separate scenario. Clearing a
  first-incoming fact or deleting message text does not remove catalogue metadata.
  A fresh send needs its exact approved recipient, text and request UUID.
- Strict real mobile import remains unaccepted; the retained archive's read-only
  acceptance neither enables import nor proves complete upstream history.
- Publication and fresh CI are now accepted by the evidence below. Live filter,
  restart and outside-catalogue sending gates remain separate.

At test completion, reconcile the controlled peer's original known novelty state
from the private backup without replacing newer corpus/send/delivery state or
rewinding the production subscription. Preserve the new test evidence and state.

## Publication review of the unpublished range

Public main was still `8d913c4` when checked. The 25 local migration commits add
or change 12,089 files, including 11,571 documentation/evidence files. Scanning
that commit range with gitleaks 8.30.1 found 217 `generic-api-key` matches. Every
matched retained artifact belongs to the offline `join-approved` case and the
matched field is `plan_token`. The scenario obtains it through
`Fixture.ApproveSyntheticJoin` in separately created synthetic fixture state.
These findings are synthetic approval capabilities, not evidence of exposed
production credentials. They should still be redacted from public traces rather
than hidden by a broad secret-scanner exclusion.

A separate documentation scan found machine-specific absolute home paths in
287 changed files, including lint, overlay and performance evidence. Public push
is withheld until the unpublished artifacts/history are sanitized and their
provenance/checksum references remain coherent. Sanitizing generated evidence
must not silently relabel historical results as fresh verification or alter the
executable schemas and synthetic fixture inputs. No credential revocation is
claimed necessary from these synthetic findings alone.

The [current-tree publication copies](publication-evidence-sanitization.md) are
now sanitized, checksum-linked and checked independently. All 27 unpublished
commits have also been transformed, signed and scanned without findings; every
original/sanitized pair preserves production sources and executable contracts.
The checkout now follows the clean history; original history remains recoverable
from the verified private archive. Full local `make check` passed on the identical
tree. These were intermediate review observations; final publication follows.

## Completed publication and fresh CI

Public main reached
[`09de2a1`](https://github.com/skosovsky/zl-mcp/commit/09de2a1b03dba77879b885c8de0df9d28fb1cbda).
The final 29-commit published range passed secret scanning after history sanitation;
the private original history remains recoverable. Both Ubuntu and macOS jobs in
[CI run 38632094257](https://github.com/skosovsky/zl-mcp/actions/runs/38632094257)
completed successfully, including root/nested tests, race/vet, lint, architecture,
offline synthetic checks, CGO-free builds and native service startup acceptance.
This is fresh publication evidence, not additional live Zalo acceptance.

## Fresh-account readiness

The user now has one newly registered test account. The
[live plan](conversation-completion-live-plan.md) prioritizes its genuine first
incoming before any outgoing or friendship changes. Subsequent outgoing cannot
also prove first sending to an unknown peer without a separately scoped test.

A read-only check through the actual connected MCP client still reports
authenticated/connected, no last error and 83 stored messages. Exactly one active
v2 subscription covers all direct chats in both directions, without first-only
filtering. No temporary subscriptions were created, no journal was acknowledged,
and no message was sent. A verified peer identity and independently routed trusted
test receiver are still required before the first incoming readiness signal.
