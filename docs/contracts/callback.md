# Callback contract

Production contract for subscription verification and event delivery. Design sources: [MCP Events](https://developers.openai.com/plugins/build/mcp-events) and [Standard Webhooks](https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md). The exact application Events shape is defined in [events.md](events.md); universal client support is not assumed.

1. Callback URLs use HTTPS without userinfo or fragments. Private, loopback, link-local, multicast and other non-public addresses are rejected. Redirects are rejected. DNS is validated for each connection and the connection is pinned to an approved IP while preserving the original TLS hostname.
2. Secrets start with `whsec_` and decode from standard base64 into 24–64 key bytes. Errors do not expose URLs, secrets or response bodies.
3. POST bodies are serialized once and limited to 262144 bytes. HMAC-SHA256 signs the exact bytes of `webhook-id + "." + unix_timestamp + "." + body`. The signature header contains `v1,` followed by the standard base64 signature.
4. Headers: `Content-Type: application/json`, `webhook-id`, `webhook-timestamp`, `webhook-signature`, and `X-MCP-Subscription-Id`.
5. Verification sends an object with `type=verification` and a random challenge. The ID and challenge are unique for each attempt; requests are time-bounded. Activation requires a 2xx response echoing the same challenge, compared in constant time. Verification responses are limited to 64 KiB.
6. A 2xx delivery response acknowledges receipt, not agent processing. Persistent subscriptions, retry, deadlines, cancellation and capacity limits are described in [events.md](events.md). Receivers must verify signatures and deduplicate event IDs.

Synthetic receiver tests exercise this contract. Compatibility with a specific agent has not been verified. Endpoint publication and tunnels are external infrastructure. Test-only injected transports do not relax the production address policy.
