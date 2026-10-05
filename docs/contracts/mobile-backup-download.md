# Mobile backup download candidate

Status: internal component only, not called by the service or exposed through MCP.
A trusted local caller supplies an exact allowlist of verified download DNS hosts.
No default hosts are guessed and no host is learned from the untrusted offer.
Production integration and actual archive host verification remain open.

Accept HTTPS on port 443 only, without credentials, fragment, IP literal, trailing
DNS dot or wildcard host. Resolve at dial time and reject the entire answer if
any address is private, reserved, loopback or non-public; dial the validated IP
without a second hostname lookup, while retaining normal TLS hostname validation.
Do not use environment proxies, cookies, Zalo credentials or redirect following.
The signed URL query stays in memory and is never part of returned errors.

Use one GET with a 120-second total timeout and bounded handshake/header waits.
No application retry is performed. Require status 200, identity content encoding,
and an exact positive size declared by the correlated offer within the caller's
archive budget (at most 512 MiB). An advertised Content-Length must match. Read
at most expected size+1 and require exact EOF length; discard owned buffers on
failure. The result is ciphertext only; it proves neither format nor ownership.
A successful download does not generate Events or persist messages/files.

### Session-cookie compatibility gate (2026-10-05)

Installed client 26.9.10.2959 `SyncMessageController.download` passes its offered
URL to `DownloadType.File` with `cookies: true`, `preventTransform: true` and
`noiseDownload: false`. This establishes cookie-enabled client behavior, not
proof that every archive host requires a Cookie header. The third-party
[experimental synchronization PR](https://github.com/RFS-ADRENO/zca-js/pull/269)
explicitly claims cookie requirements and obtains cookies for chat.zalo.me before
fetching the arbitrary offered URL; that cross-host forwarding is not adopted.

The existing `NewDownloader` is a bounded anonymous transport candidate. It is
not proven equivalent to the authenticated native client. Before production
archive acceptance, provide a current-session SDK port that attaches only cookies
applicable to the exact independently verified URL through its cookie jar, while
preserving the root downloader's validated DNS/TLS/no-redirect transport and body
limits. Do not export cookie values to domain results, CLI, logs or MCP; do not
copy all chat.zalo.me cookies to an offered CDN hostname. The offered URL does not
authorize credential forwarding or add itself to the host allowlist.

If real acceptance requires a special cross-host authorization mechanism, record
that evidence and define a separate exact scope before implementing it. A 401
from an archive/signed URL alone must not revoke the Zalo session: expired signed
URLs and missing archive authorization are distinct from account authentication.
No session cookies or private archive were read for this static investigation.

### Internal current-session port

`ConsumeMobileArchive(ctx, request, validatedClient, consume)` executes one GET
through the current SDK account's cookie jar, retaining the supplied validated
transport. Requests must be HTTPS with no explicit Cookie/Authorization/proxy
credentials; SDK never substitutes its ordinary API transport. Clone the client,
force no redirects and a maximum 120-second timeout, and attach only the existing
jar's URL-applicable cookies. Ignore archive Set-Cookie headers instead of
changing authentication cookies. The response body is owned/closed by this port;
consume it inside the same guarded session lifetime with the downloader's exact
size/encoding checks. Return only owned private ciphertext or a fixed error.

The guard keeps cancellation through full body consumption, rejects revoked or
late-success data and clears returned buffers on failure. SDK/client snapshot
checks reject an account change. Archive HTTP 401 remains an archive failure,
not an account-authentication sentinel. `WithSession` creates a bound downloader
copy without modifying the original validated transport/allowlist. The service
binds its existing guard before requesting the phone offer. No automatic retry,
new account, listener, generic credential accessor or MCP capability is added.
