# Explicit local callback delivery

The default Events callback policy remains public HTTPS with DNS pinning, no
proxy and no redirects. An operator can additionally authorize up to 16 exact
local callback URLs in the service configuration:

```toml
[events]
local_callback_urls = ["http://127.0.0.1:19001/events"]
```

The executable configuration contract is `service_events.input.json`. Missing or
empty `local_callback_urls` disables local delivery. Changes take effect at service
startup. Both subscription verification and every delivery enforce the same policy.
Existing subscriptions keep their generation, activation boundary and processing
checkpoint. Removing a URL prevents further HTTP requests to that URL, including
queued retries; normal bounded failure/retry rules apply. Cancellation still uses
the subscription identity and must remain possible after URL removal.

Local URLs must use the literal address `127.0.0.1` or `[::1]`, an explicit port
1..65535 and an absolute canonical path. HTTP and HTTPS are allowed. Hostnames,
other loopback spellings, wildcard hosts/ports/paths, private LAN addresses,
credentials, query strings, fragments, encoded paths and dot segments are rejected.
The complete URL must exactly match an operator entry. A caller cannot enable the
mode through subscription arguments, message text or a delivery-mode override.
The MCP endpoint remains authenticated; its listen policy does not change.

`delivery.mode` remains `webhook`. The additive subscription schema admits the
local URL shape but schema validity alone does not grant access. A non-allowlisted
local target fails subscription verification before any network request. Local
transport dials the literal endpoint without DNS or environment proxies and never
follows redirects. HTTPS retains normal certificate/hostname validation; there is
no insecure-TLS option. Public HTTPS retains the original SSRF/DNS protections.

All callbacks, including HTTP loopback, carry the unchanged Standard Webhooks
HMAC-SHA256 signature, timestamp, webhook ID and subscription header. The receiver
must verify the exact signed bytes, reject stale timestamps, bind the subscription
after verification and deduplicate event IDs. The service signs callbacks; it
cannot prove that an arbitrary receiver verified them. Live acceptance requires
independent receiver evidence. HTTP loopback provides no payload encryption or
server authentication: use only a trusted same-host receiver with a private random
signing key; processes with privileged host access remain outside this protection.
No session credentials or MCP token are included in callback bodies.

Callback 2xx, durable processing ack and user notification remain separate facts.
The local mode neither creates a receiver nor changes subscription scopes, journal
semantics, ordering, retry limits or historical-import Events suppression. Endpoint
publication/tunnels are unnecessary for same-host HTTP loopback delivery.
