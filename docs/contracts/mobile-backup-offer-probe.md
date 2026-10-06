# Owner-only mobile offer probe

`mobile-backup-probe-offer <operation_id> <revision>` calls the existing service's
owner-only Unix socket through `cli_probe_mobile_backup_offer`. This is a diagnostic
phone execution route, not an MCP tool, a download or a message import. Invoke only
after the human authorizes the exact prepared request and is ready to confirm the
Zalo phone prompt. Preparation alone is not dispatch authorization.

Input is the strict executable envelope/arguments schema. Validate ownership,
collection policy, prepared state and the exact last-read revision before calling
the internal current-session offer runner. No session restore or second listener.
The durable observer claims dispatch once; retrying a dispatched/interrupted/terminal
attempt cannot issue another phone request. Timeout is bounded by the existing
180-second source limit. Cancellation/lifecycle/auth loss suppress late results;
read attempt status after an unknown/failed request, never automatically redispatch.

The selected typed conversation/interval constrains later processing; the upstream
phone may create an account-wide archive. The probe neither downloads nor examines
that archive. Return only contract-checked attempt state, an unverified DNS host
parsed from an HTTPS offer (no path/query/userinfo/port), archive size as an exact
decimal string and whether it fits the prepared byte budget. URLs, keys, cookies,
RSA material and message contents are never CLI output or logs. Host observation
is evidence for subsequent investigation, not permission to add it to a downloader
allowlist or forward credentials. The private offer is discarded after reporting.

This route is intentionally one-shot: losing its result requires reading the
ledger and a separate decision about another attempt. It cannot supply a resumable
archive or close production import, format/control or old-message recovery gates.
Existing prepare/status/cancel routes remain nonexecuting; cancellation through
the prepare-cancel command remains limited to prepared attempts.

## Verification

Implemented and installed from signed commit `051b581` on macOS. Root race tests,
vet, CGO-disabled macOS arm64/Linux amd64 builds and the worktree secret scan
passed. Synthetic tests prove revision/one-shot dispatch checks, private metadata
redaction, contract validation and no corpus/Events/send writes. The installed
owner CLI returned `NOT_FOUND` for a nonexistent attempt through the existing
service; its STDIO bridge still listed 21 tools and read connected/authenticated
status. SQLite integrity and selected subscription identity/filter/generation/
activation fields matched the private pre-update snapshot. No real phone request,
archive download or import has been performed for this diagnostic acceptance.

## Current deployment checkpoint

The last authorized phone probe confirmed the zero-code handling fix, then
expired without an archive offer. Source commit `ac394b3` increases the Unix
response budget to 200s and adds safe ignored-control diagnostics; its CI passed.
Its installation did not become ready. A Go stack showed schema meta-validation
of `zalo_get_join_status.output` before opening HTTP. Isolated contract and MCP
server construction completed quickly, so the startup cause is unresolved.
At that checkpoint the verified previous `5e92289` binary was restored without reverting SQLite,
session or configuration. Authenticated MCP reads then confirmed connected/
authenticated state, schema 12 integrity and the active v2 journal. That restored version still had the old 40s Unix write budget. A later separately
authorized diagnostic deployment passed readiness on `ac394b3`; the installed
200s write budget then returned the terminal failure instead of hiding it as a
collector transport error. No further phone request was sent.

## Failure diagnostics

The service records committed dispatch/progress states with operation ID and fixed
failure categories (timeout, cancellation, authentication, unavailable source,
unknown result, rejection, invalid source event or generic execution failure).
It never logs source error text, offers, keys, URLs, account/conversation IDs or
message content. These logs supplement the durable terminal status; they do not
permit retrying a terminal operation or establish archive compatibility.

The first authorized live probe reached the phone; the user confirmed it. The
attempt ended `failed` without an offer. Its installed version did not preserve
intermediate diagnostic categories, so the exact failure stage cannot be inferred
from its terminal revision. No download or import occurred. A subsequent live
attempt needs a separate decision, not an automatic retry.

Control diagnostics additionally record whitelisted action/status/error scalars,
correlation and presence booleans, and archive size. Fixed validation reasons
cover control decoding and offer envelope/sequence/account/database/cipher checks.
Rejected field values and raw source errors are excluded. The authorized probe
with the status fix reached `waiting_for_backup` and then `INVALID_SOURCE_EVENT`;
no offer/download/import was obtained. That version did not identify the rejected
control, so the active/idle correction alone is not a proven resolution.

The owner-only Unix HTTP response deadline is 200 seconds, above the 190-second
CLI request budget and the bounded phone wait/finalization. A shorter write
budget previously hid the final timeout as `COLLECTOR_UNAVAILABLE`; it did not
mean that the collector stopped. Receiver diagnostics report only fixed reasons
for ignored correlation/account/host controls, and only while a receiver is active.
They neither reveal rejected values nor weaken matching requirements.

The authorized zero-code-fix probe received confirmation and the zero-code
control, then correctly continued waiting. No accepted archive offer or decoder
failure was observed before the source deadline. The ledger ended `interrupted`
with `TIMEOUT`; archive download/import remain unverified. This proves the
zero-code interpretation fix, not successful phone archive delivery.

The user observed the phone progress completing quickly during this attempt.
That observation establishes completion of the visible phone flow, not receipt
of an archive by this service. Read-only comparison with the installed native
client confirmed the same `syncmsgmb` / `syncmsg_info` routing. The deployed
version can still silently discard a mismatched key or account; absence of an
accepted offer therefore does not establish absence of an upstream response.
The newer candidate binary subsequently opened HTTP in 0.82 seconds with an
isolated temporary configuration and no live credentials. This does not resolve
its earlier startup failure with the installed service state. No new phone
request or live service change was made during these checks.

A separately authorized repeat subsequently ran on the newer diagnostic binary.
Installation readiness and preserved subscription boundaries passed. After phone
confirmation, the receiver logged `ACCOUNT_MISMATCH` for a `syncmsg_info` control
whose request public key matched. This establishes local rejection of an upstream
offer, rather than absence of an upstream response. Native restore code passes
the offer's `rawUid` as `plainUserId` and the current session's user ID as
`noiseUserId`: direct equality between these namespaces is not a valid account
binding. The current implementation also repeats that equality in offer validation.
The fix must distinguish these identities and establish a verified binding before
archive use; do not simply remove the account guard. No archive was downloaded
or imported by this repeat.

## Plain-owner binding correction

The receiver now forwards an exact-key offer without comparing its plain uid
to the session noise ID. The operation resolves the plain uid through the
existing authenticated identity-mapping API and accepts only one direct pair
whose session ID equals the captured owner. Missing/foreign/error mappings
fail closed, and an account change invalidates the result. Both IDs are private
redacted offer fields. This does not bypass archive-content ownership checks.

Root and nested-module race tests and vet passed; CGO-disabled macOS arm64 and
Linux amd64 builds passed. The installed macOS binary SHA-256 is
`4806e1da0c1595dd6f7aac31137d1d2de5661ebaa26c25d5de45dde6b0eaf8c2`.
Deployment readiness and preserved subscription boundaries passed. One further
phone request was explicitly authorized and dispatched. It remained in
`waiting_for_confirmation` until the 180-second source deadline and ended
`interrupted` / `TIMEOUT`. No correlated confirmation or offer was recorded
during that window. The user subsequently reported confirming on the phone,
but the exact phone confirmation time is unknown. This run does not verify
live mapped-owner acceptance or archive receipt.
No archive download or message import is included in this probe.

The next authorized repeat received phone confirmation and a correlated
`syncmsg_info` offer (246,660 bytes). The receiver no longer rejected the plain
owner ID. The operation then failed at authenticated mapping (`MAPPING_FAILED`),
without download/import. The existing generic failure does not identify whether
the SDK request/envelope or the positional mapping codec failed; stage-specific
redacted diagnostics are required before repeating this probe.

The stage-diagnostic repeat received confirmation and another 246,660-byte
correlated offer. Mapping was rejected by the outer upstream envelope with
`error_code=600`, before decryption or positional decoding. Meaning of this
private endpoint code is not established. Static native/SDK comparison found
API version 691 versus default 665; the endpoint-specific correction is a
candidate, not a proven explanation for code 600. No download/import occurred.

The endpoint-version candidate passed nested API race tests, API vet and the
CGO-disabled macOS build. It was installed with rollback material retained;
readiness and selected subscription-boundary comparisons passed. Installed
SHA-256: `522e4626fbc1480a6e8dcee78b6824c02ca72adea9c5c6ef6731a552b1c67cad`.
No phone request was sent during this deployment. Live version-691 mapping
acceptance remains open; this checkpoint does not establish code-600 semantics.

The authorized version-691 probe received confirmation and a correlated offer,
but the identity API again returned outer `error_code=600`. The version change
did not resolve the failure and is not an established root cause. This run
performed no archive download/import. Further phone repeats without a distinct
request/session hypothesis provide no additional evidence. Native source uses
the same encrypted-form POST route and fids/gids payload; endpoint-specific
session/cookie compatibility remains unverified.

The authentication-scope investigation found chat-only zpw_sek absent at zwid
under normal web-session jar rules, whereas native setAppCookie provides it at
the parent domain. The exact-endpoint borrowed-cookie correction passed API race
tests, vet, synthetic wire/redirect/jar-scope tests and macOS build. Installed
SHA-256 is `961f78362c1d7065a0b6ad541ab108c7e6411a850eb6d02c5199772285cf939b`;
service readiness and subscription-boundary preservation passed. No phone
request was made during this investigation/deployment. Live correction remains
unverified; code 600 is not yet definitively attributed to the missing cookie.

## Successful authenticated offer acceptance — 2026-10-06

The separately authorized borrowed-cookie probe received confirmation, a
correlated format-1 offer and authenticated plain-to-session owner mapping.
Logs recorded `mobile_backup_account_verified`; the durable attempt reached
`offer_ready`. The CLI returned 246,660 bytes, within the prepared byte budget,
and observed host `trans-bin.zaloapp.com`. It explicitly reported
`download_performed=false` and `import_performed=false`. No URL, archive key,
cookie value, account ID or message text was exposed by diagnostics.

With the same endpoint version 691, adding the scoped zpw_sek corrected the
previous outer code-600 rejection in this live case. This establishes the
auth-cookie fix and owner mapping acceptance for this session, not a universal
definition of error code 600. It closes the correlated control/offer/owner
acceptance gate. The observed host still requires independent download-policy
verification; the discarded one-shot offer cannot be reused for import. Real
archive download, decoding, selected silent persistence and old-message recovery
remain open. A successful offer is not a completed history import.

### Selected archive diagnostic

The owner-only `mobile-backup-probe-archive <operation_id> <revision>` consumes
one fresh prepared attempt. It downloads format 1, maps archive filenames through
the authenticated identity endpoint, reads only the selected conversation/date
window, and returns counts and conversion eligibility. It does not persist messages
or emit Events. Offer-ready remains a phone-ledger state, not an import result.
Terminal attempts cannot be reused; retries need a new explicitly authorized UUID.

The download allowlist pins `trans-bin.zaloapp.com` independently of offer contents.
Read-only verification found public DNS, a valid DigiCert TLS certificate covering
`*.zaloapp.com`, and the native client's first-party use of `zaloapp.com`.
The exact CDN hostname was observed in the authenticated offer, not found literally
in the reviewed native bundle. TLS/DNS are operational checks, not a guarantee of
future availability. Public-address validation and certificate verification also
run for each download. No redirects are allowed. URL-applicable cookies accompany the session-owned
request. Following read-only native `setAppCookie` evidence, the archive transport
may borrow exactly one `zpw_sek` from `https://chat.zalo.me/` only for the exact
HTTPS host `trans-bin.zaloapp.com` without an explicit port. It never changes jar
scope, copies unrelated cookies, or forwards this token to other CDN hosts. Missing
or ambiguous source tokens fail closed; a destination token takes precedence.

Budgets: phone offer 180 seconds, HTTP download 120 seconds, selected page walk
120 seconds; service operation 420 seconds, owner CLI 445 seconds, private Unix
response timeout 450 seconds. The public MCP HTTP timeout is unchanged. Plaintext
scratch files are private and removed on exit; URL, keys, identities and message
text are excluded from diagnostics. Fixed stage logs distinguish download,
container decryption, file index, identity mapping and selection failures.

Two authorized live selected-archive diagnostics reached `offer_ready` but failed
at download before format-1 decryption. No messages were imported and no Events
were emitted by the diagnostic. The earlier fixed stage log identifies the
boundary but does not distinguish transport, status or size failure; it cannot
justify claiming a CDN/authentication defect.

Download diagnostics now separate `HTTP_REQUEST`, `HTTP_STATUS`,
`CONTENT_ENCODING`, `CONTENT_LENGTH`, `BODY_LENGTH`, `RESPONSE_CONTEXT` and
`RESPONSE_CONSUMPTION`. Only fixed stage names, numeric status/size and boolean
cancellation/ownership checks are logged. HTTP response bodies, full error
strings, URLs, header values and credentials remain excluded. Failed offer
material is discarded; another live diagnostic requires a fresh phone attempt.

The next live diagnostic confirmed `HTTP_STATUS=400`, with no cancellation or
account change. Absence of a URL-applicable `zpw_sek` and native parent-domain
cookie setup motivated the narrowly scoped correction above. Its causal effect
remains unverified until a fresh diagnostic succeeds.

A fresh diagnostic with the scoped archive-cookie correction accepted phone
confirmation, then failed during the authenticated owner mapping request
(`mobile_identity_failed: REQUEST`). It never reached archive download; therefore
this run neither confirms nor disproves the cookie hypothesis. The collector
remained connected/authenticated and the recovery journal empty. The attempt
ledger still showed `waiting_for_backup` after bounded failure finalization;
this discrepancy is a separate unresolved recovery defect and must be resolved
before another phone attempt. No automatic redispatch is justified by that state.

Failed dispatched attempts use an independent 15-second finalization budget,
longer than SQLite's 5-second busy timeout and a short wait for its single shared
connection. A regression test holds that connection beyond the old three-second
budget. Read/write finalization failures emit fixed diagnostic stages; finished
phone attempts never automatically retry. Startup recovery marks abandoned
attempts interrupted before any new dispatch. This fixes bounded contention,
not arbitrary database unavailability.

The offer-only owner CLI budget is 205 seconds, including bounded failure
finalization; the selected-archive CLI budget is 445 seconds.

The next authorized run after recovery reached authenticated `offer_ready`,
completed the bounded download, and entered `decrypt_container`. Thus the scoped
cookie correction changed the observed failure boundary from HTTP 400 to format
validation; no cookie/URL values were logged. The format reader still rejected
the archive, so archive compatibility, selected SQLite read and silent import
remain unaccepted. The old abandoned phone attempt was independently verified
as `interrupted` after startup recovery. Subscriptions remained unchanged.

A subsequent live diagnostic identified `CIPHERTEXT_LENGTH=246660` with AES
remainder 4, within the download budget. The boundary reader now permits a partial
physical tail only after the fully decrypted and checksum/XZ-verified container,
following the independently reviewed native declared-end handling. Synthetic tests
cover tails 1..15 and reject a container requiring a missing final cipher block.
Live compatibility of this correction remains pending a fresh authorized run.

The next run reached selection without `mobile_archive_selection_failed`.
Consequently the earlier interpretation of the last stage marker as a selection
failure was incorrect: the selected file was accepted and the subsequent page
consumer failed. SQLite/page diagnostics now cover header, cursor/scope, private
scratch, immutable connection, limits, integrity, table/columns, control counts,
message query/row validation, sender/metadata mapping and expiry. Header failures
log only byte count and SQLite format read/write versions. No row values, IDs,
filenames, schema SQL, text or underlying error strings are exposed. Selected
conversation absence must only be asserted from its explicit diagnostic code.

The complete page diagnostic identified `SQLITE_HEADER` on the selected 65536-byte
main file with read/write versions 2/2. This is the documented SQLite WAL marker,
not evidence of corruption. Immutable main-file inspection now accepts 2/2 after
all existing header/integrity/schema checks; diagnostic results expose `wal_mode`.
Conversion remains blocked for WAL snapshots until producer checkpoint/export
semantics are established, since integrity alone does not prove absence of omitted
WAL transactions. Independently generated checkpointed WAL fixtures are tested.

The next authorized diagnostic completed the entire offer/download/decrypt/
selected-file/immutable SQLite/schema/page pipeline. The selected historical
interval returned examined=0, rejected=0, candidates=0, pages=1,
source_has_more=false, stop_reason=available_archive_exhausted, wal_mode=true.
This establishes successful bounded inspection of the supplied main-file image,
not absence of messages from the phone, omitted WAL or other source history.
Conversion was blocked by the unverified WAL snapshot policy; import=false and
no diagnostic Events were emitted. Date coverage of the whole selected image
and producer checkpoint semantics remain open before claiming historical coverage.

### Selected-image coverage and correction of the live comparison

Subsequent authorized probes on 2026-10-06 returned 24 source rows, with valid
millisecond timestamps spanning September 26 through October 6 UTC. The selected
September 30 Vietnam interval contained zero rows. Coverage counts precede
message-type, status, content and expiry conversion; zero here is not a rejection
by those filters. Download, decryption, typed identity mapping, selection and
immutable SQLite inspection completed; persistence was not performed.

A read-only local catalogue check confirmed the requested peer's identity. Its
live corpus contained 21 records, and the latest timestamp matched the selected
archive's latest timestamp to the millisecond. This supports the selected-peer
binding but does not establish equality of all records or complete phone history.
The user's newly sent control messages belonged to a different peer. Unchanged
coverage for the selected peer therefore does not establish a stale export or a
lost live message. The historical date was reported by the user; the original
native inbox screenshot shows a relative age, not an exact message timestamp.
The specific missing historical record has not been independently identified.

The native format-1 consumer enumerates per-peer `.db` files, maps their plain
identities into the current session, and reads `ChatContent.TimeStamp` directly
as milliseconds. Its consumer code does not establish how the phone producer
checkpoints its export. SQLite read/write versions 2/2 alone prove neither an
uncheckpointed transaction nor a missing sidecar. Do not label this evidence a
confirmed WAL loss, stale archive or confirmed historical omission. Remaining
work is verification of producer snapshot semantics and wiring the bounded,
silent importer with explicit coverage and durable page checkpoints. Repeating
phone requests with an unchanged selection is not a substitute for that work.

The user then explicitly changed the selected interval to September 26 Vietnam
(`2026-09-25T17:00:00Z` through `2026-09-26T17:00:00Z`, exclusive end).
A fresh authorized probe completed and found two source rows, examined=2,
candidates=2, rejected=0, missing_metadata=0 and invalid_metadata=0. The whole
image still contained 24 rows. Eight unsupported metadata-field occurrences were
reported; this is a field count, not eight rejected messages. Conversion remained
blocked by the existing WAL policy, and import_performed=false. This confirms
that the date-scoped archive reader can find older candidates; it does not yet
prove message-text conversion or persistence. No message text was exposed.
