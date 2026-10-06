# Bounded mobile backup offer operation candidate

Status: internal guarded adapter; not in MCP/CLI discovery and not invoked by
the service. Uses current API/session/listener, with at most 180 seconds total.
Generate an ephemeral RSA-2048 key, register the correlated receiver before
dispatch, and make one initial request. Unknown acknowledgement keeps waiting
for the already correlated reply; it never sends another initial request.
Cancellation releases the receiver, without a fresh network context or
unapproved retry/cancel request. The separate cancellation API remains available
for an explicit operation-layer decision.

BeforeDispatch is a trusted local callback accepting the generated public key.
It must durably commit the operation identity and dispatching state before HTTP.
A callback failure prevents dispatch. Context is checked again after the commit.
Progress callbacks are trusted local functions returning an error and receive only
closed states: waiting_for_confirmation, request_result_unknown,
mobile_restoring, waiting_for_backup, offer_ready. A callback failure terminates
waiting and discards any offer; it never triggers a resend. A matching rejection fails;
restoring does not imply confirmation. Correlated transfer_error status 1/2
means mobile active/idle and continues the existing bounded wait, matching the
native client status-first handling. A correlated transfer_error with error_code=0 also continues the bounded wait,
including when status is absent; it is not archive success. Other transfer_error
events terminate: explicit nonzero error_code is rejected, absent code without
a recognized status is invalid. Log only the fixed status category and numeric failure code;
never the event body or private fields. Queue failure
fails the operation. Context expiration ends waiting without claiming that the
phone action failed. An unknown dispatch remains unknown if no reply arrives.

Validate account, metadata and backup format before use. This candidate supports
only explicitly declared format 1. RSA PKCS#1 v1.5 decryption produces 16–128 key
bytes, rendered as uppercase hex text for the separately verified block decoder.
The internal offer carries only bounded metadata and key text; all fields are
excluded from JSON/default formatted output. It must never be logged or returned
through MCP. Private RSA state remains in memory and is released at termination;
complete erasure of Go/library-owned buffers is not claimed.

No archive download, SQLite access, message import, Events, durable operation
creation or automatic restart retry occurs here. The operation layer still needs
a journal, download-host policy, complete archive validation/account mapping and
silent persistence before this becomes an available history source.
