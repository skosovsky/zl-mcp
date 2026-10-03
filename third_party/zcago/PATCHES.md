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

These source fixes have not yet been deployed to the installed service. They do
not establish restored collection or notification delivery.
