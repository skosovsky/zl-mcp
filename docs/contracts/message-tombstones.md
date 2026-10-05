# Permanent deletion markers

A trusted observed deletion/recall removes the exact typed conversation/message
identity atomically with a text-free permanent marker. Do not confuse a retention
cleanup with an upstream deletion: age retention can permit a later source replay,
but an observed deletion must not be undone by replay or historical import.

Preserve identity, subscription activation and existing first-incoming facts;
clear content/FTS, quote metadata, event snapshots and delivery payloads through
the existing removal transaction. A deletion received before its message also
creates a marker. A later live/replay/history record with that exact typed identity
is ignored without content, Events or a positive first-contact classification.
For a previously unknown direct peer, mark novelty unknown, not positively new.
Different conversations and direct/group namespaces remain independent.

Require valid typed reference, nonempty message ID and existing collection policy
permission. Markers survive restart and ordinary retention and contain no message,
attachment, sender name or quoted body. This is an internal collector storage
port; no new public MCP delete capability is added.

Source migration 11 creates markers and updates the visible-message view. It does
not infer deletions of already stored data or guess mobile control targets.
Mobile type-33/36 target semantics remain a separate compatibility gate.
