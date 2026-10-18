# Callback contract

Production contract for subscription verification and event delivery. Design sources: [MCP Events](https://developers.openai.com/plugins/build/mcp-events) and [Standard Webhooks](https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md). The exact application Events shape is defined in [events.md](events.md); universal client support is not assumed.

1. By default callback URLs use public HTTPS without userinfo or fragments; private, loopback, link-local, multicast and other non-public addresses are rejected. An operator can explicitly allow exact same-host HTTP/HTTPS URLs through `events.local_callback_urls`; see the executable [local delivery contract](local-callbacks.md). Local targets use literal `127.0.0.1`/`[::1]`, explicit ports and canonical paths, without DNS or proxies. Redirects are always rejected. Public HTTPS still validates DNS for each connection and pins an approved IP while preserving the TLS hostname; HTTPS certificate verification is never disabled.
2. Secrets start with `whsec_` and decode from standard base64 into 24–64 key bytes. Errors do not expose URLs, secrets or response bodies.
3. POST bodies are serialized once and limited to 262144 bytes. HMAC-SHA256 signs the exact bytes of `webhook-id + "." + unix_timestamp + "." + body`. The signature header contains `v1,` followed by the standard base64 signature.
4. Headers: `Content-Type: application/json`, `webhook-id`, `webhook-timestamp`, `webhook-signature`, and `X-MCP-Subscription-Id`.
5. Verification sends an object with `type=verification` and a random challenge. The ID and challenge are unique for each attempt; requests are time-bounded. Activation requires a 2xx response echoing the same challenge, compared in constant time. Verification responses are limited to 64 KiB.
6. A 2xx delivery response acknowledges receipt, not agent processing. Persistent subscriptions, retry, deadlines, cancellation and capacity limits are described in [events.md](events.md). Receivers must verify signatures and deduplicate event IDs.

Synthetic receiver tests exercise this contract, including an actual configured loopback receiver without injected transport. Compatibility with a specific agent is separate evidence. Public endpoint publication and tunnels are external infrastructure; local delivery needs neither. Test-only injected transports do not grant production exceptions.

## Delivery observability

Each claimed delivery emits `mcp_event_delivery_started` and
`mcp_event_delivery_finished` through the service logger. Both carry a random
per-attempt correlation ID, attempt count and body byte count. The finish record
adds elapsed milliseconds, bounded HTTP status (zero when unavailable), a finite
outcome/reason and whether the outcome was persisted. The explicit safe field
allowlist below applies to both the service logger and diagnostic file.
A start without finish requires checking lease recovery. HTTP acceptance remains
distinct from model-visible event processing.

Logs never include message text, payload/body, native account/chat/message/event/
subscription/delivery IDs, receiver request IDs, tokens, response bodies or raw
errors, including nested fields. No payload digest is retained.

### Opt-in bounded diagnostic trace

`logging.event_trace_file` remains disabled by default. A nonempty absolute path,
separate from `logging.file`, enables a private rotating JSON log. T08 replaces
its former full-content format; no full-trace or bypass mode remains.

The explicit field allowlist is: standard logger time/level/message; randomly
created per-attempt `correlation_id`; integer `attempt`, `body_bytes`,
`elapsed_ms`, `http_status`; closed `delivery_kind` (business/journal_recovery);
optional state-keyed `delivery_reference` (dlv_/rdlv_) and control-only opaque
`recovery_reference` as defined in [delivery diagnostics](event-delivery-diagnostics.md); finite `outcome` (pending/delivered/failed) and
`reason` (network/accepted/callback_gone/payload_too_large/http_transient/
http_rejected/attempts/deadline); boolean `receipt_persisted`. HTTP status is
0 if unavailable, otherwise a valid 100..599 status (other values collapse to 0).
Request, response and receipt records share a random correlation ID; it is
independent of all native IDs. These fields cannot reconstruct the event body.

The file and archives remain 0600 in a 0700 directory with existing size/backup
limits; write failures retain existing service behavior. The delivered Events
format, callback signing, leases, receipts and retry policy do not change.
An empty trace is expected until the next naturally collected matching event.
