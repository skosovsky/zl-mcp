# Local upstream patch

Source: github.com/amrakk/zcago commit d4ff65b460577b2557e70220b68d08ce1f7431b4 (v0.3.1-0.20260908044439-d4ff65b46057). Runtime source and LICENSE retained; CLI/examples excluded.

Patch: model/message.go TQuote.UnmarshalJSON accepts cliMsgId, globalMsgId and ts as integer JSON numbers or decimal strings. Live Zalo Reply/replay supplied cliMsgId as a string; the original int64 alias failed decoding the entire event. Parsing uses json.Number.Int64, never float64; int64 overflow remains an explicit upstream type limitation. 

Regression coverage: internal/zalo/adapter_test.go includes synthetic numeric/string quote IDs above 2^53 and invalid/overflow inputs. Local replace makes the workaround reproducible without modifying the module cache. Remove the patch after an upstream version includes and verifies equivalent behavior.

Patch: internal/websocketx/client.go preserves an explicit HTTP 401 from websocket.Dial as errs.ErrAuthenticationRequired. Response body, headers, URL and raw dial error are not retained in this sentinel. Other HTTP failures preserve their existing classification; 403 is not assumed to mean expired authentication. A real local HTTP handshake regression covers 401/403/429/500 in internal/websocketx/authentication_test.go.

The application adapter maps this sentinel to a safe domain authentication error; collector stops reconnecting, persists authenticated=false/auth_required, and leaves an open gap. Cleanup and stale heartbeat preserve the required local-login action. Live logout on 2026-10-01 produced close code 3003 and saved-session login rejection code 102. The application now treats a server kick (3003) as requiring local login, while duplicate connection 3000 remains distinct. Natural timed expiry and other possible rejection codes are unverified.

Patch: session/auth/login.go preserves nonzero error_code from outer and decrypted login responses and the server-info response as typed ZaloAPIError, without retaining upstream message text. Explicit HTTP 401 returns the authentication sentinel. Server-info uses the wire envelope Response[T] so error_code is not discarded. Concurrent login rejection cancels server-info: makeServerInfoRequest no longer dereferences a nil response on request failure. Missing-response parser guards prevent panics. Regression coverage in session/auth/login_errors_test.go checks synthetic outer/decrypted/server-info codes, HTTP 401, omission of private response text, and cancellation without a response.

The application interprets typed code 102 only during saved-session login, based on its live reproduction after logout; it does not classify arbitrary API errors or text containing 102 as authentication loss.

## Upstream review, 2026-10-02

Current upstream main still points to d4ff65b460577b2557e70220b68d08ce1f7431b4. Regression probes confirm the quote and login/server-info fixes are still required. The WebSocket 401 fix can instead be implemented at the application's supported HTTPClient/RoundTripper boundary; an offline probe verified that the original Dial preserves an injected typed cause. This migration has not been implemented. `errs/authentication.go` is an added local declaration used by the auth patches; it is absent upstream. Full evidence and limits: [zcago-upstream-review.md](../../docs/zcago-upstream-review.md).

## Upstream issue tracking

- [TQuote.UnmarshalJSON rejects decimal-string message IDs and timestamps](https://github.com/amrakk/zcago/issues/2)
- [WebSocket Dial discards handshake HTTP status needed to classify authentication failures](https://github.com/amrakk/zcago/issues/3)
- [Login ignores nonzero error_code in outer and decrypted response envelopes](https://github.com/amrakk/zcago/issues/4)
- [Server-info parses the normalized response type and silently ignores wire error_code](https://github.com/amrakk/zcago/issues/5)
- [Login and server-info ignore HTTP 401 and can return success for an unauthorized response](https://github.com/amrakk/zcago/issues/6)
- [makeServerInfoRequest panics when transport fails without an HTTP response](https://github.com/amrakk/zcago/issues/7)

## Ingestion diagnostics

`groupMessageOrUndo.UnmarshalJSON` now returns malformed-message decoding errors rather than accepting a nil message/undo pair. Synthetic regression tests cover valid messages, numeric timestamps rejected by upstream string types, malformed quote IDs and missing identifiers. This exposes rejection without guessing unsupported wire conversions.

An optional `Diagnostics()` extension reports per-listener atomic counters for incoming frames, ignored text frames, unmatched commands, key exchanges, direct/group/replay frames, decoded messages, successfully queued live messages and errors (direct/group live and separately attributed mixed replay failures). It contains no payloads, account/message IDs, URLs or key values. Replay envelope shape is classified as other=0, wrapped=1, unwrapped-group=2 or unwrapped-user=3. The application logs snapshots every 15 seconds to its existing private rotating sink. Successful enqueue does not prove persistence or upstream completeness.

Raw WebSocket frames now wait for space in the bounded receive queue rather than evicting older frames. Listener message, replay, undo and cipher-key channels use cancellable backpressure; parsing errors also wait for a consumer. Nonessential reaction/typing/status notifications retain the upstream best-effort policy. Listener diagnostics count queue pressure and cancelled emissions. Shutdown releases blocked producers. Synthetic burst tests verify frame/message ordering and cancellation; backpressure does not guarantee recovery of messages upstream withheld or lost during disconnection.

## Direct and mixed replay ingestion

The direct-message union decoder now returns invalid JSON/type/missing-ID failures and clears reused values. Mixed old-message responses emit both direct and group batches rather than discarding the direct array. Batch type remains homogeneous for existing consumers. Regression tests cover mixed responses, invalid timestamps/quotes, missing IDs and message-to-undo reuse.

## Direct sending contract verification

`api/send_message_contract_test.go` adds synthetic wire-contract tests without
modifying the upstream send implementation. A local HTTP receiver decrypts the
actual form payload and verifies plain `/api/message/sms` and quoted
`/api/message/quote`, Unicode text, peer routing, decimal-string quote IDs above
JavaScript precision, quote owner/type/timestamp/TTL and encrypted acceptance.
No real account or session key is used. The application separately classifies
HTTP/decode errors conservatively because `ZaloAPIError` does not preserve their
provenance; this is not a new dependency runtime patch.

## Listener cleanup and reaction identifiers

`listener.Stop` now clears the socket, cipher and request state after all workers
exit. Previously cancellation could end `run` before it consumed the socket close
notification, leaving `client` non-nil; the collector's next `Start` then failed
with `Already started`. A cancellation regression fails on the previous cleanup
and passes with the patch. A real loopback websocket regression also verifies
two successful handshakes on the same listener after consumer-triggered stops;
it passes ten repetitions with the race detector. Cleanup remains idempotent.

`ReactionMessageRef.UnmarshalJSON` accepts numeric and decimal-string `gMsgID`
and `cMsgID` without a float64 conversion. Invalid, fractional and overflowing
IDs remain errors. This compatibility change targets a live type mismatch on
`data.rMsg.gMsgID`; the private wire payload was not captured, so its exact form
and live recovery are not established by the synthetic tests. Existing public
`int` fields and their platform size limit are retained.

The fixes were subsequently installed after an authorized private-copy trial.
Repeated startup and replay processing succeeded with no listener error. This
does not establish reproduction of the original private reaction payload or
end-to-end notification delivery; see the application acceptance report.

## Numeric send acknowledgements

The pinned upstream `api.SendMessageResult` declared `msgId` as `string` only.
The maintained JavaScript reference declares a numeric result:
[sendMessage.ts](https://github.com/RFS-ADRENO/zca-js/blob/dadfef18bcac53537741855c99f084152da1fad9/src/apis/sendMessage.ts).
The encrypted local wire test now covers both numeric and string acknowledgements
on `/sms` and `/quote`. Before the patch, numeric cases failed with
`ZaloAPIError[0]: Failed to parse response data`; the application consequently
reported an ambiguous send. The original string-only fixture missed this case.

`SendMessageResult.UnmarshalJSON` now preserves string IDs and exact decimal
integer digits without float64 or fixed-width integer conversion. Missing/null
acknowledgements remain empty; fractional, exponent, negative and structured
numeric alternatives remain decoding failures. A number above 2^53 is verified
through the encrypted HTTP path, with larger integers covered by unit tests.

This is a reproduced decoder defect and a plausible explanation of the live
quoted-send acknowledgement. The actual response was not retained, so it cannot
prove this was the exact cause of that operation. Its `unknown` ledger status must
not be rewritten or retried automatically based on this patch.

## Bounded offline queue continuation

The installed native client static-code review found that an offline response with
`more` requests the same queue again using `first=false` and `lastActionId`.
Previously this dependency discarded these response fields and only exposed the
first-page request. The response model now retains raw continuation metadata,
validates boolean/0-or-1 flags and decimal action IDs without float conversion,
and attaches it once to the final homogeneous batch of a response. Queue origin
is preserved independently of the direct/group message arrays. Malformed
continuation metadata does not discard otherwise valid messages.

`RequestReplayPage` is an optional concrete-listener extension; existing
`RequestOldMessages` and the public Listener interface remain compatible. The
application continues only after persistence and bounds each queue by pages,
messages and repeated cursor detection. Tests cover mixed and empty batches,
large numeric IDs, malformed metadata, encoded request headers and the first
flag. This is offline queue continuation, not an arbitrary historical-message
API or a completeness guarantee. Installed live recovery remains unverified.

## Group-cloud history page candidate

An optional concrete API method `GetGroupHistoryPage` implements one bounded
read of `/api/cm/getrecentv2` on the authenticated session's advertised
`group_cloud_message` service. Protocol provenance is upstream JS PR #370,
head `4eeceafad031ce4f594c4532e3363dbdf01450b0`, and the installed native
client's endpoint inventory. This is an independent Go implementation; it does
not copy the reference's float-based cursor conversion. The existing public
API interface and endpoint initialization remain compatible.

Encrypted loopback wire tests verify the exact numeric cursor beyond 2^53,
GET route, request limits, account identifier, source flag, nested response
string, filtering flags and raw message digits. Missing/null arrays, malformed
continuation metadata, oversized pages and invalid requests are rejected;
missing service metadata never guesses a domain. The extension returns raw
protocol records only. A bounded page was live verified on the installed local
candidate through the silent import worker; external-client import discovery
and deeper phase traversal remain open. The SDK read itself creates no stored
records, Events or completeness claim.

## Conversation preload page candidate

An optional concrete `GetConversationPreload` reads the existing advertised
conversation service with an empty thread mapping. It preserves exact raw
records, bounds encrypted input/record counts and distinguishes missing message
categories from observed empty arrays. It never guesses a domain, starts a
listener or persists data. Primary reference: zcloud commit
`10f752b431102b71e3185a2077041efff907ed3e` and the reviewed native endpoint.
Synthetic wire tests cover encryption, route, precision, nullable categories,
whole-page rejection and missing source. The installed local candidate uses
this source for catalogue metadata and explicitly selected snapshot imports.
Live catalogue and snapshot reads passed; recovery of the requested older
Strangers message remains unproven.

Both preload wire and expanded response reads now use an 8 MiB budget plus one
overflow sentinel byte before decoding. A valid encrypted JSON prefix followed
by excessive whitespace is rejected, for plain and gzip responses alike.
This closes a truncation loophole in the earlier LimitReader-only guard.
Regression tests check whole-page rejection and bounded reads. This update is
not yet installed; the current service remains the earlier phase-aware build.

## Optional mobile synchronization controls (candidate)

The existing listener now has an optional `SubscribeMobileSync` extension,
without adding requirements to the public Listener interface. It accepts one
bounded receiver correlated by current account, exact request key and host.
Native syncmsgmb controls preserve confirmation states and exact metadata IDs;
unknown actions are ignored. Sensitive event fields are excluded from JSON and
redacted in default formatting. Overflow or malformed controls fail only an
active receiver, without blocking ordinary collection or losing other controls.
Synthetic router/precision/bounds/redaction/cancellation race tests cover this
boundary. No mobile HTTP request, archive download/import or live capability
is enabled by this patch. The service has not registered this receiver and the
patch is not installed. Primary protocol evidence is recorded in the project
research report and mobile-sync-control contract.

## Initial mobile request and cancellation (candidate)

Optional concrete `RequestMobileBackup`/`CancelMobileBackup` methods use only
the authenticated advertised file service. Initial sequence/retry values are
fixed at zero. Native RSA-2048 SPKI DER/base64 public keys are validated before
dispatch. A separate authenticated `RequestOnce` helper disables application
redirect traversal for these side-effecting GETs, without changing existing API
calls. Each acknowledgement has independent 64 KiB wire/expanded limits and
strict error-code presence; HTTP 401 remains authentication-required and errors
omit source bodies/URLs. Network/server/redirect ambiguity remains unknown,
without automatic application retry or an exactly-once transport claim.
Synthetic encrypted-wire, cancellation, invalid-input, redirect, auth/error and
overflow tests cover this layer. The service does not invoke it yet; no real
phone transfer or archive recovery was performed and the patch is not installed.

The optional group-history candidate resolves encrypted envelope and group leaf
separately so local normalization failures retain a typed safe field reason
rather than a synthetic upstream code zero. Source flags accept boolean or exact
integer 0/1 only; missing/null remains unknown. Tests retain exact cursors and
reject noncanonical flags. A bounded local live read passed after the flag fix;
this is not proof of complete upstream history or ordinary direct-cloud support.

The group SDK reader bounds both wire and decompressed response bodies to 8 MiB
before envelope decoding. A one-byte overflow sentinel rejects oversized input,
including otherwise valid JSON followed by excessive padding. Synthetic plain
and compressed oversized-envelope tests cover this resource boundary. This
hardening remains pending installation until the next checked candidate.

The optional phase-aware group method uses the advertised service's getoldv2
only when the caller explicitly supplies the old phase. The original method
retains recent routing. Native getCM/client-retry code is the phase provenance;
encrypted synthetic wire tests verify route selection and exact numeric cursor.
This extension is pending installed/live phase acceptance.


### Optional mobile identity mapping request (offline candidate)

GetMobileIdentityMapping issues one encrypted form POST to the statically observed
zwid.api.zalo.me/api/znoise route through the current session. Exact uint64 JSON
integers avoid native JS parseInt rounding; requests are capped at 1000 typed IDs.
RequestOnce prevents application redirect/retry, total timeout is 10 seconds,
wire/expanded limits are 512 KiB and decrypted data is capped at 256 KiB. Explicit
outer/inner success codes are required and duplicate envelope fields rejected.
HTTP 401 preserves the shared authentication sentinel; other errors are fixed.
The response is bounded plaintext requiring caller-side typed positional codec
validation. No endpoint initialization/interface expansion or live request is
introduced. Native route evidence is documented in the repository research file;
real mapping reply compatibility and production wiring remain unverified.

### Current-session archive transport candidate

`ConsumeMobileArchive` is an optional API method using the existing account's
URL-scoped cookie jar with a caller-supplied validated HTTP transport and bounded
body consumer. It rejects injected credential headers, never falls back to the
ordinary SDK transport or follows redirects, ignores archive Set-Cookie, closes
the response and clears late/failed results. This is original Go implementation;
no code from the experimental zca-js synchronization PR was copied. Root session
guards keep body consumption inside revocation ownership. Real archive hosts and
phone/archive acceptance remain unverified; HTTP 401 here is not an account-auth
sentinel. No public API interface method or additional listener is required.

Mobile control decode diagnostics log closed validation reason codes through the
service logger, without payloads, URLs, correlation keys or identities. They do
not relax parsing or initiate synchronization. Regression tests check private
field exclusion and preservation of the existing error sentinel.

During an active mobile receiver, ignored key/account/host correlation controls
log only a fixed rejection reason. Matching rules and foreign-control handling
are unchanged; rejected values and raw events are never logged.

Mobile offer correlation distinguishes plain mobile owner IDs from session noise
IDs. The receiver preserves exact key and current-session binding, but delegates
plain-owner resolution to the caller's authenticated identity mapping rather than
comparing different namespaces. Regression coverage checks delivery of a
correlated plain-ID offer while rejecting foreign request keys and hosts.
