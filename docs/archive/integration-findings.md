# Historical upstream integration findings

Authorized live checks established that a restored saved session could collect selected group messages, retain them across restart, and expose search and reply context through MCP. Membership operations covered joined, already-member and moderated outcomes; repeated request identifiers returned the original operation instead of repeating the mutation.

Observed upstream behavior required local dependency patches:

- Reply quote identifiers and timestamps can arrive as decimal strings; strict integer decoding discarded the event before the application received it.
- WebSocket handshake status and login/server-info error codes must be retained to classify known authentication failures safely.
- Server-info cancellation may return no HTTP response; error handling must not dereference it.
- In the observed membership flow, upstream code 240 indicated pending approval. Opaque success or ambiguous failure remains unknown until reconciliation.

Saved-session rejection included login code 102, HTTP 401 and a server kick with close code 3003. Unknown API/network errors are not automatically authentication failures. Patched synthetic tests cover classification; not every live path was repeated after each fix. See the [upstream review](../zcago-upstream-review.md) and [patch record](../../third_party/zcago/PATCHES.md).

Available replay recovered specific test messages after a controlled interruption. The replay window and completeness were not measured. Silent listener overflow, upstream retention, unsupported edits/deletions and Mac downtime can leave gaps. Coverage reports therefore describe the collected corpus, with history completeness remaining false.
