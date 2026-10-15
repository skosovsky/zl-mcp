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

This report describes verified source behavior. The installed backend must still
be upgraded and an exact operator allowlist configured before a live local
subscription can use it. Independent real-message delivery and receiver processing
remain separate evidence; the existing assistant's selective notifications are
not their acceptance criterion. The sole fresh test account remains reserved.
