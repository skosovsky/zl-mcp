# Conversation completion: acceptance evidence

Status: work in progress. Tracks [the combined task](task-conversation-completion.md). No model evals are used.

## Local browsing implementation

The executable input/output schemas were added before the storage and MCP dispatch implementation. `zalo_list_conversation_messages` provides exact typed local chronological browsing without a keyword, bounded pages, RFC3339 `[since, until)` and signed snapshot continuation. Existing search behavior remains unchanged. The [browsing contract](contracts/conversation-browsing.md) describes snapshot versus current coverage, resource retrieval, empty reasons and limits.

Added storage regressions exercise same-time ties, both orders, timezone offsets/exclusive upper boundary, a late historical insertion excluded from an existing traversal, continuation after reopening SQLite, colliding direct/group IDs, direction, Unicode truncation, empty reasons and permission checks. An MCP client calls the new tool without a query and reads the returned escaped resource URI, validating the embedded output contract through the actual handler. Tool-count discovery expectations now include the additive tool.

This stage uses temporary synthetic databases only. No installed binary, session, configuration, subscription or runtime data was changed. Strangers protocol discovery, directory expansion, explicit history import, live v2/first-incoming checks, installed skills and final publication remain unfinished. Do not interpret source capabilities as capabilities already available in the connected client.

Validation for this checkpoint: root `go test ./...` and `go vet ./...` passed; focused `go test -race ./internal/storage ./internal/mcpserver ./docs/contracts` passed. Both repository skills now describe conditional use of the additive browse capability, without assuming the installed client already exposes it. Complete root/nested race checks, clean checkout validation, deployment and client reload remain later gates.
