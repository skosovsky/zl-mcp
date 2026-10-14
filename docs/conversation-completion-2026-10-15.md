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

- A second new incoming message must have `first_incoming=false`; it is not yet
  present in the observed corpus. After an agreed restart, the fact must remain
  known and another incoming must not become first again. Do not reset repeatedly
  merely to repeat the already accepted first=true observation.
- Real incoming-only and first-only delivery require separately agreed temporary
  scopes and a client/receiver that processes those exact subscriptions. The
  existing all-direction subscription cannot prove exclusion.
- First sending outside the catalogue remains a separate scenario. Clearing a
  first-incoming fact or deleting message text does not remove catalogue metadata.
  A fresh send needs its exact approved recipient, text and request UUID.
- Strict real mobile import remains unaccepted; the retained archive's read-only
  acceptance neither enables import nor proves complete upstream history.
- Local migration commits through `6161075` were not on public main when this
  checkpoint was prepared. Publication requires reviewing and scanning that
  unpublished range, then successful CI; do not reuse October 7 CI as evidence
  for the upgraded backend.

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
