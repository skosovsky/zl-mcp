# Local upstream patch

Source: github.com/amrakk/zcago commit d4ff65b460577b2557e70220b68d08ce1f7431b4 (v0.3.1-0.20260908044439-d4ff65b46057). Runtime source and LICENSE retained; CLI/examples excluded.

Patch: model/message.go TQuote.UnmarshalJSON accepts cliMsgId, globalMsgId and ts as integer JSON numbers or decimal strings. Live Zalo Reply/replay supplied cliMsgId as a string; the original int64 alias failed decoding the entire event. Parsing uses json.Number.Int64, never float64; int64 overflow remains an explicit upstream type limitation. 

Regression coverage: internal/zalo/adapter_test.go includes synthetic numeric/string quote IDs above 2^53 and invalid/overflow inputs. Local replace makes the workaround reproducible without modifying the module cache. Remove the patch after an upstream version includes and verifies equivalent behavior.

Patch: internal/websocketx/client.go preserves an explicit HTTP 401 from websocket.Dial as errs.ErrAuthenticationRequired. Response body, headers, URL and raw dial error are not retained in this sentinel. Other HTTP failures preserve their existing classification; 403 is not assumed to mean expired authentication. A real local HTTP handshake regression covers 401/403/429/500 in internal/websocketx/authentication_test.go.

The application adapter maps this sentinel to a safe domain authentication error; collector stops reconnecting, persists authenticated=false/auth_required, and leaves an open gap. Cleanup and stale heartbeat preserve the required local-login action. Live logout on 2026-10-01 produced close code 3003 and saved-session login rejection code 102. The application now treats a server kick (3003) as requiring local login, while duplicate connection 3000 remains distinct. Natural timed expiry and other possible rejection codes are unverified.

Patch: session/auth/login.go preserves nonzero error_code from outer and decrypted login responses and the server-info response as typed ZaloAPIError, without retaining upstream message text. Explicit HTTP 401 returns the authentication sentinel. Server-info uses the wire envelope Response[T] so error_code is not discarded. Concurrent login rejection cancels server-info: makeServerInfoRequest no longer dereferences a nil response on request failure. Missing-response parser guards prevent panics. Regression coverage in session/auth/login_errors_test.go checks synthetic outer/decrypted/server-info codes, HTTP 401, omission of private response text, and cancellation without a response.

The application interprets typed code 102 only during saved-session login, based on its live reproduction after logout; it does not classify arbitrary API errors or text containing 102 as authentication loss.
