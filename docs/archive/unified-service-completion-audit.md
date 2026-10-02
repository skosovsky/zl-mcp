# Historical unified-service verification

The unified Go service was verified with normal and race tests, static analysis, and CGO-disabled builds for macOS arm64 and Linux amd64. Independent SDK/HTTP clients exercised tools, resources, discovery and Events. A synthetic TLS receiver independently checked signed delivery and full-text retrieval.

Tests covered atomic message/event persistence, replay deduplication, transactional activation boundaries, durable delivery leases, stable event identifiers, restart, cancellation during verification, retry independence, bounded queue capacity, expiry/deadlines, attempt limits and allowlist revocation. Authentication failure retained MCP access to the corpus while cancelling upstream operations. Logging tests covered privacy, rotation and startup failures.

Native deployment checks confirmed direct Go execution through one LaunchAgent, session/corpus/ledger preservation, SQLite integrity, HTTP and STDIO access, and recovery after forced process termination. The previous Python supervisor was removed after migration checks. Private deployment evidence and account data are excluded from the public repository.

The synthetic 100,000-message performance test recorded p95 113.459583 ms for warmed first-page queries on the tested local macOS environment, below the 500 ms target. This is an environment-specific historical result.

Limits: the receiver was a test client, not a connected production agent. External routing and actual Mac reboot were not tested. Forced process recovery does not prove reboot behavior. Collection during sleep/shutdown and complete upstream replay are not guaranteed. Delivery has bounded retries and capacity, and 2xx acknowledges receipt rather than completion of agent work.

Research and event-handling skills were subsequently updated without model evaluations. The 65 earlier evaluations do not establish their current behavior. Current contracts and operational instructions supersede this record.
