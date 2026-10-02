# Historical task: native macOS service

Superseded by the [unified service task](task-unified-ann-service.md).

The original task replaced a Python supervisor with a directly launched Go binary. Deployment required one LaunchAgent, recovery after process failure, bounded shutdown, private state/configuration, rotated logs, and a diagnostic sink for startup failures. Authentication failure had to stop upstream retries while preserving local data.

Migration required backup and rollback, preservation of the session, corpus and membership ledger, and removal of the old supervisor only after successful verification. The unified service inherited these requirements; a standalone collector is no longer a supported architecture. Current commands and paths are documented in the [deployment guide](../mac-deployment.md).
