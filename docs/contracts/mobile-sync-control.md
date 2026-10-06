# Mobile sync control candidate

Status: optional SDK control receiver, not an MCP source or active transfer.
Use the existing authenticated listener; its public Listener interface remains
compatible. Register one in-memory receiver before any mobile request. Bind it
to the current account, exact request public key and pc_name. Unregister on
completion/cancellation; no second login/listener is created.

Only act_type=syncmsgmb with user_confirm, syncmsg_info or transfer_error is
accepted. Control data can be an object or a JSON string. Cap encoded and
decoded data at 64 KiB. Preserve uid/from_seq_id as canonical uint64 decimal
strings without float conversion. Confirmation retains user_action 0/1/2/3;
the operation layer interprets rejection/restoring/confirmation. The receiver binds the active session and exact request key; confirmation/error
also match pc_name. The offer uid is a plain mobile ID, not the session noise ID.
Deliver correlated offers to the operation layer, where the authenticated identity
mapping must resolve that plain ID to the captured session owner before acceptance. Missing or
malformed correlation fields are rejected. Unknown actions are ignored.
Malformed sync data marks only this control invalid; it does not discard other
controls in the frame. An active receiver fails with one fixed error; without
a receiver it is ignored, preserving normal collector behavior.

Sensitive event fields are internal SDK data, excluded from JSON serialization
and redacted in default formatted representations. No raw payload is retained.
Application diagnostics may log only the closed action category, numeric
status/error/user-action, correlation/presence booleans and archive size.
Never log identity, PC name, public key, URL, encrypted key or database body.
Offer validation logs fixed reason codes rather than rejected values.
Download URL is bounded HTTPS without
userinfo/fragment; this is syntax validation, not download-host authorization.
Encrypted key and database metadata remain untrusted until subsequent checks.

The receiver has eight event slots and a separate one-slot failure channel.
Never block the collector or overwrite queued controls. Overflow emits one
fixed failure and stops accepting controls for that receiver; the operation
must fail rather than claim successful transfer. Absent receivers discard
sync controls. No automatic requests, downloads, key decryption, state writes
or message Events are performed by this layer.
