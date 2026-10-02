# Historical task: unified Go service

Implemented architecture: one persistent Go service owns the Zalo session, account lock, internal collector, MCP interface and delivery worker. The collector has no external transport. Optional STDIO clients connect through a bridge to the running service. Interactive login remains a separate operation performed while the service is stopped.

Requirements preserved the existing search/context contracts, allowlist, account binding, trusted CLI approval, idempotent membership operations, persistent limits and coverage diagnostics. Known authentication failure keeps MCP and stored messages available while stopping upstream operations; network failures remain retryable.

Message persistence and event creation must be atomic. Durable subscription boundaries and delivery jobs survive restart. A new event represents the first service insertion after subscription activation, including available late upstream replay, without publishing the pre-existing corpus. Refresh and restart preserve the boundary; cancellation creates a new generation on subsequent subscription.

Events include group/message identifiers, author, time, and at most 2048 Unicode characters of text. Truncated text has an explicit flag and a full-text MCP resource. Delivery is at-least-once within bounded retries and queue capacity, independently of collection. Subscriptions may remain active until cancellation, but individual deliveries have finite deadlines. Access revocation, cancellation and expiration stop delivery. The [Events contract](../contracts/events.md) defines the executable wire format and limits.

Acceptance covered an independent MCP client and synthetic signed TLS receiver, contract tests, restart/recovery, cancellation, access filters, Unicode and full-text resources, authentication failure, safe logging and native launchd deployment. Connection to a particular agent and external HTTPS routing were excluded. Runtime state was preserved during migration; sources and installed runtime are separate. See the [verification summary](unified-service-completion-audit.md).
