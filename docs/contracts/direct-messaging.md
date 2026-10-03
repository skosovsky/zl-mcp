# Direct messaging contract (implementation in progress)

This contract extends [the task](../task-direct-messaging.md). It is not a claim
that the installed service provides these tools yet.

## Sending text

`zalo_send_direct_message` accepts recipient_id (opaque string, 1–256 characters),
text (nonblank valid UTF-8, at most 2048 Unicode code points), request_id (UUID),
and optional reply_to_message_id. The text limit is a conservative application
limit, not a verified maximum imposed by Zalo. No attachments or group sends.
`zalo_get_send_status` accepts request_id and never performs a network send.

The send result contains request_id, recipient_id, status, message_id (nullable),
reason (nullable safe category), updated_at and retry_safe=false. Status is pending,
sending, sent, failed or unknown. Repeating the same request_id and exact arguments
returns or continues the same operation. Different arguments conflict. A new UUID
is a new send, not a safe retry of unknown.

Before any send, persist a SHA-256 fingerprint of canonical recipient/text/reply
arguments. Do not persist text in the operation ledger. Persist the sending claim
before calling upstream. Restart converts interrupted sending operations to unknown;
unknown and terminal operations never call upstream again. A caller can resume a
pending operation by repeating its original arguments. The service does not need to
retain message text to resume such a caller-driven operation.

The ledger retains request identities indefinitely to prevent an old request from
becoming a new send. It is bounded to 100,000 identities per account; on reaching
capacity, reject new requests explicitly. Existing status/retries remain available.
This bounds metadata and avoids silent expiry of deduplication. No automatic retry
of an ambiguous Zalo result and no exactly-once claim across the upstream boundary.
Sent means upstream accepted and supplied an ID, not that the recipient read it.

The pinned dependency uses `ZaloAPIError` for both protocol rejections and HTTP or
response-decoding failures. A missing/zero code and codes overlapping HTTP status
numbers are classified as ambiguous, except the documented invalid-parameters
protocol code 114. Distinguishable nonzero protocol codes produce `failed`; an
ambiguous result produces `unknown`. Raw upstream error messages are not returned.
This conservative rule may report unknown for a real rejection whose code overlaps
HTTP, because this dependency does not preserve the error's provenance.

Sending permission is independent of collection. Default allow_send=false;
recipient IDs can be restricted separately. An explicit allow_send=true with no
recipient restriction permits addressing any opaque peer ID allowed by Zalo.
Authentication and the trusted agent's user instruction remain required.

Reply metadata is resolved by the service from the exact recipient's retained
direct message. Missing, deleted, unsupported or incomplete quote data fails before
sending. It must not fall back silently to an unquoted message. No message from a
different conversation may be quoted. The upstream text and quote metadata remain
subject to normal deletion/retention, without a hidden copy in the send ledger.

## Filtered Events

Preserve both existing profiles and all their serialized queued payloads. A new
versioned profile `zalo.conversation.message.created.v2` carries schema_version=2,
direction (incoming/outgoing/unknown) and first_incoming (true/false/null), plus the
existing typed message fields. Version 1 remains byte-format compatible.

Arguments preserve all/direct/group/conversation scopes and add direction
(incoming/outgoing/all, default all) and first_incoming_only (default false).
The first-only form requires incoming and an exact direct conversation or direct
scope. Unknown direction/novelty cannot match a positive filter. Defaults are
canonicalized before hashing the subscription ID; filter changes create a distinct
subscription. Existing version-1 IDs and boundaries remain unchanged.

First incoming means first incoming insertion known to this service, not first in
the full Zalo history and not first since subscription activation. Migration seeds
known incoming peers from retained messages; incomplete historical evidence is
unknown, never asserted as a first. Outgoing records do not consume this marker.
Duplicates, retention, undo and restart do not reset it. Late replay can be the first
locally recorded incoming while carrying an old sent_at. Eligibility still requires
first insertion after subscription activation; no historical corpus push.

Direction and first-incoming evidence must be saved atomically with the original
message/event identity, not recalculated from retained text during fanout. A service
restart or later deletion cannot alter the decision for an already queued event.
