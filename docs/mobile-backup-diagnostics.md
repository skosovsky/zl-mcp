# Mobile history diagnostics

Status: experimental internal source. The owner-only CLI supports the attempt
ledger, phone-offer probe, selected-archive inspection and retained whole-account
export. Authorized installed
probes passed download, decryption, identity mapping and SQLite inspection.
Durable mobile import is still unavailable; none of these diagnostics is an MCP
tool. An older installed binary may not have these commands. Do not describe
preparation or a successful probe as loading messages into the corpus.

## Retained whole-account export

When the user authorizes the complete account scope, prefer the separate
[retained archive workflow](contracts/retained-account-archive.md) for repeated
date-window investigation. It retains every validated direct/group file and
its original typed mapping in an owner-only encrypted runtime cache. Capture
uses one phone-confirmed transfer; status and bounded per-file coverage work
offline after restart. Repeated reads do not redispatch or extend the seven-day
default retention. Explicit removal keeps the source UUID spent.

This is diagnostic source retention, not message import or a public MCP tool.
Coverage is a bounded summary, not full message pagination; WAL, controls and
unsupported attachment semantics still need source-specific evidence. The whole
received bundle is not a guarantee of complete Zalo history. Selected-conversation
one-shot probes below retain their existing disposable-scratch behavior.

## Selected-conversation one-shot probes

Use the current service and its existing account. The commands below connect to
its owner-only Unix socket; they do not restore a session, start another listener
or open the corpus database independently. Preparation can work while the
collector is disconnected if its stored account is known. It grants no permission
to execute a phone operation later.

Prepare a private request JSON file matching
[the executable selection contract](contracts/mobile_backup_attempt.input.json):
stable request UUID, exact typed conversation ID, RFC3339 `[since, until)` and
optional bounded message/archive budgets. Omit optional budgets to use defaults;
explicit zero is invalid in the input schema. Reuse the same UUID and exact args
for retries. Do not include session values, credentials, URLs or archive keys.

With an already running service and absolute configuration/request-file paths:

```sh
zl-mcp -config /absolute/path/config.toml mobile-backup-prepare < /absolute/path/private-request.json
zl-mcp -config /absolute/path/config.toml mobile-backup-status OPERATION_UUID
zl-mcp -config /absolute/path/config.toml mobile-backup-cancel OPERATION_UUID REVISION
```

The response contains the normalized request, operation UUID, state, revision and
creation/update times. Preparation writes only the attempt ledger. Public RSA
correlation material and private transport values are excluded. Treat response
IDs as private operational metadata; do not publish a real request/response.

Cancel only a `prepared` attempt using its last-read revision. If cancellation
races with dispatch, it fails instead of claiming phone work stopped. An already
cancelled attempt can return its current state with its current revision. No
network cancellation is attempted. Read status after a lost response; never make
a fresh phone attempt automatically.

After the user authorizes the exact selected conversation and period, an owner
may execute one prepared attempt with its current revision:

```sh
zl-mcp -config /absolute/path/config.toml mobile-backup-probe-archive OPERATION_UUID REVISION
```

The phone may require confirmation. Read status after an observation timeout;
do not redispatch an active attempt. A terminal attempt cannot be reused to make
another phone request. New dispatch requires a fresh UUID and user authorization.
The request selects what is inspected from the account-wide transfer; it does not
ask the phone to produce an archive of only that dialogue. Only the selected
file is read, and private scratch is removed after the diagnostic. No archive
URL, key or message body is returned. Prior scratch cannot be reused to change
the interval after completion.

`source_rows` describes the selected main image; `source_period_rows` counts all
rows in the exact interval before validity/type/TTL filters. Min/max timestamps
do not prove continuous history coverage. `text_candidates` counts validated
plain-text candidates, while `convertible_messages` excludes pages with source
safety gates. `conversion_block_reasons` explicitly identifies unverified WAL
snapshots or source controls. Unsupported content and unresolved quote/mention
counts remain visible even for a blocked page. Malformed candidates fail the
operation instead of being mislabeled a safety gate. `import_performed` is always
false. A WAL marker is not itself proof that the phone lost transactions.

This ledger is not a new history source in the research skill. Continue using
actually discovered history-import tools for group pages or a bounded conversation
preload snapshot. The mobile pipeline still needs verified producer snapshot
semantics, attachment and control semantics, persistent expiry and selected
silent import. Earlier failed transport/format probes are preserved as dated
evidence, not the current status of the inspected source.
Any agreed phone acceptance must specify the exact operation and avoid exposing
archive keys or message bodies. Historical imports remain without Events.

See [local ledger contract](contracts/mobile-backup-local-ledger.md) and
[current acceptance gates](conversation-completion-open-gates.md).

## Controlled recall comparison

The separately authorized before/after recall investigation uses the current
service's [owner-only recall diagnostic](contracts/recall-diagnostic.md). Send the
exact agreed message through `zalo_send_direct_message`, inspect the pre-recall
archive, then run `probe-recall SEND_REQUEST_UUID` and inspect a separately
prepared post-recall archive. It uses the saved send operation and live quote
metadata; an unknown send result cannot be recalled by guessing from text.
Never start another Zalo client or listener for this comparison. Each archive
request needs its own phone confirmation. An upstream numeric reply alone does
not establish collector receipt or a corresponding archive control record.

For an exact saved send-operation comparison, supply its UUID as the optional
fourth argument to both archive probes:

```sh
zl-mcp -config /absolute/path/config.toml mobile-backup-probe-archive OPERATION_UUID REVISION SEND_REQUEST_UUID
```

The saved operation must be `sent` and belong to the selected direct conversation.
The additional `comparison` reports matching global-message-ID rows, numeric type
and status counts, and examination/rejection/truncation bounds. It returns no IDs,
sender names or text. A zero match with rejected rows or `has_more=true` does not
prove absence. Even without those gaps, it describes only the selected main image
and period; it does not establish WAL completeness or identify a control whose
own global ID differs from the recalled target. Import safety gates stay active.

### Controlled diagnostic evidence (2026-10-06)

The authorized disposable message was accepted through the service's local HTTP
MCP `zalo_send_direct_message` route and retained by the collector with live quote
metadata. Before that dispatch, the ChatGPT connector rejected the nonblank text
against its cached `\S` pattern; the send ledger contained no operation for that
UUID. This isolates that rejection to pre-server connector validation. The new
full-string-compatible pattern preserves nonblank semantics; actual refreshed
connector acceptance remains to be checked and must not be inferred from local
contract tests.

The first pre-recall archive attempt entered `waiting_for_confirmation`, then
terminated as `interrupted` after approximately three minutes without an archive.
No archive comparison or recall was performed. The service remained connected;
this run establishes neither source recall semantics nor archive completeness.
A new phone attempt needs a fresh UUID and a ready user; the old attempt must not
be redispatched. The temporary one-peer send permission was restored byte for
byte to its original disabled configuration after the attempt terminated.

The subsequent command-dispatch check found that `probe-recall` was described and
parsed but omitted from the executable's command switch. An isolated Unix-socket
regression first reproduced `unknown command`, then passed after adding the route.
It uses only a synthetic receipt; no real recall is established by that test.
The route correction was subsequently installed from signed source `9f1f0b5`
after macOS/Linux CI passed. A verified nonexistent send UUID reached the owner
socket and received the expected refusal without an upstream recall. The original
real test send is now outside the diagnostic's 30-minute freshness bound. Never
automatically send a replacement or bypass that bound; agree a fresh live scenario
when the user is ready for both phone confirmations.

After renewed human authorization, one fresh disposable message was sent through
the same local HTTP MCP route. Its stable UUID was saved before dispatch; the
send ledger records `sent` with the returned message ID. The exact original
configuration was restored immediately after sending, with sending disabled and
the existing collector connected. No subscription was changed.
The new pre-recall attempt waited for confirmation from 12:55:34 to 12:58:36 UTC
and terminated `interrupted` without an archive. No recall, comparison or import
occurred. A further phone request awaits explicit user readiness; neither the
terminal attempt nor the send is automatically repeated.

### Successful controlled recall comparison (2026-10-06)

With renewed explicit authorization and the phone available, a fresh exact-text
send was accepted once through local HTTP MCP. Startup and MCP status reads
temporarily stalled before dispatch; the send ledger confirmed no operation
during that stall. After recovery, the same saved UUID was used for the single
dispatch. The exact original send-disabled configuration was restored before
both archive requests. No subscription was edited.

Both separately confirmed archive probes downloaded and inspected the same
selected direct conversation and interval. Their bounded exact-ID comparisons
reported no rejected records or continuation:

| Observation | Before recall | After recall |
| --- | --- | --- |
| Selected file rows | 129 | 129 |
| Rows in requested interval | 1 | 1 |
| Exact sent-message-ID matches | 1 | 1 |
| Matching type | `0` | `36` |
| Matching status | `3` | `3` |
| Matching controls | 0 | 1 |
| Whole-file source controls | 0 | 1 |
| WAL header mode | enabled | enabled |

The owner recall returned `upstream_response` with numeric status `0`. The normal
collector persisted the exact typed-message tombstone; the message is absent
from `visible_messages` and an actual connected-client context read returns an
error without an anchor. Reading the saved recall again returned the same
response with byte-identical private receipts; it did not reserve another call.
The permanent send ledger remains the evidence of the single accepted send.

This proves a type `0` to `36` transition on the same global message ID for this
own-message recall in a direct conversation. Status `3` alone cannot distinguish
the two states. It does not establish type `33`, received-message or group recall
semantics, arbitrary control payload interpretation, or WAL transaction
completeness. Both probes performed no import. Conversion remains blocked by
`unverified_wal_snapshot`, and the post-recall source additionally reports
`unverified_source_controls`. Source support must be specified and tested before
relaxing those gates; the live tombstone already prevents this exact message from
being restored by replay or a later historical import.

The subsequent source implementation classifies mapped own-direct type-36/status-3
rows separately and adds an optional `own_recall_candidates` diagnostic count.
It preserves exact target IDs privately, leaves controls deferred and performs no
tombstone persistence. The classification has regression tests for namespace,
sender mapping, cancellation, exact large IDs, redaction and unchanged source
gates. This change is not yet installed or verified against another phone export;
the successful paired comparison above used the prior diagnostic implementation.

### Atomic own-direct recall candidate

The internal source-to-journal path now supports silent own-direct tombstones and
checkpoint publication in one transaction, before ordinary records. Synthetic
integration tests cover an encrypted snapshot, first-page classification, restart,
later-page conversion, replay suppression and absence of historical Events.
Only a non-WAL image whose entire control count is classified in the first page
is eligible. A later-page control fails before any ordinary-record prefix.
The paired real phone images above remain WAL sources and were not imported.
The native candidate was subsequently installed from clean commit `d6889c7`
after successful macOS/Linux CI. HTTP/STDIO connectivity and preserved corpus,
tombstones, send ledger and subscription definitions were checked; the real
client still refused the recalled anchor. It has not been accepted against a new
phone export; public mobile-archive admission remains disabled.

### Whole-source prelude follow-up

The next internal candidate replaces the first-page restriction for new source
reads with a bounded complete control scan and atomic journal prelude. It handles
verified own-direct recalls outside the requested message interval and after its
first page, while preserving separate message coverage, original source expiry
and silent replay suppression. A 52-target production-worker restart scenario is
verified synthetically. The candidate has not been phone-accepted or deployed;
the previously paired WAL images remain unimported.
