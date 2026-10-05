# Callback contract

Production contract for subscription verification and event delivery. Design sources: [MCP Events](https://developers.openai.com/plugins/build/mcp-events) and [Standard Webhooks](https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md). The exact application Events shape is defined in [events.md](events.md); universal client support is not assumed.

1. Callback URLs use HTTPS without userinfo or fragments. Private, loopback, link-local, multicast and other non-public addresses are rejected. Redirects are rejected. DNS is validated for each connection and the connection is pinned to an approved IP while preserving the original TLS hostname.
2. Secrets start with `whsec_` and decode from standard base64 into 24–64 key bytes. Errors do not expose URLs, secrets or response bodies.
3. POST bodies are serialized once and limited to 262144 bytes. HMAC-SHA256 signs the exact bytes of `webhook-id + "." + unix_timestamp + "." + body`. The signature header contains `v1,` followed by the standard base64 signature.
4. Headers: `Content-Type: application/json`, `webhook-id`, `webhook-timestamp`, `webhook-signature`, and `X-MCP-Subscription-Id`.
5. Verification sends an object with `type=verification` and a random challenge. The ID and challenge are unique for each attempt; requests are time-bounded. Activation requires a 2xx response echoing the same challenge, compared in constant time. Verification responses are limited to 64 KiB.
6. A 2xx delivery response acknowledges receipt, not agent processing. Persistent subscriptions, retry, deadlines, cancellation and capacity limits are described in [events.md](events.md). Receivers must verify signatures and deduplicate event IDs.

Synthetic receiver tests exercise this contract. Compatibility with a specific agent has not been verified. Endpoint publication and tunnels are external infrastructure. Test-only injected transports do not relax the production address policy.

## Delivery observability

Each claimed delivery emits `mcp_event_delivery_started` and
`mcp_event_delivery_finished` through the service logger. Both carry event and
subscription IDs, queue delivery ID, attempt, body byte count and SHA-256 of the
exact persisted body. The finish record adds elapsed milliseconds, HTTP status
(zero when no response), bounded ASCII `x-request-id` / `openai-request-id`
values when present, categorized outcome, and whether the outcome was persisted.
A start without finish requires checking lease recovery. HTTP acceptance remains
distinct from model-visible event processing.

Logs never include body text, sender/conversation IDs, callback URL, signing
headers/secrets, response bodies or raw transport errors. Request IDs accept only
ASCII letters, digits, dot, underscore and hyphen, at most 128 characters; other
values are omitted. Existing private log permissions and rotation apply.

### Opt-in full diagnostic trace

`logging.event_trace_file` is disabled by default. A nonempty absolute path,
separate from `logging.file`, enables a private rotating JSON log. It records the
full outbound event body, response status, bounded receiver request IDs, and the
response body up to 64 KiB with truncation/read-failure indicators. Known callback
URL and signing key values are redacted from responses. Authentication/signature
headers and cookies are never recorded. This trace contains private conversation
text and IDs: keep it outside the repository/iCloud and disable after diagnosis.
The file and archives are 0600 in a 0700 directory, using the existing size/backup
limits; write failures stop the service rather than silently losing diagnostics.
An empty trace is expected until the next naturally collected matching event.
