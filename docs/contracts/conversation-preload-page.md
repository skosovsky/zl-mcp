# Conversation preload page candidate

Status: guarded source and metadata refresh installed and checked with the
existing session. Explicit silent snapshot import is implemented locally;
external-client discovery remains pending. This is not complete history recovery.

The native client and pinned zcloud implementation identify an encrypted GET
`/api/preloadconvers/get-last-msgs` on the authenticated account's advertised
`conversation` service. Initial parameters are `threadIdLocalMsgId="{}"` and
existing IMEI, plus standard protocol parameters. No guessed domain, second
login/listener, read receipt, send or friend action is allowed. The SDK adds an
optional concrete method; its existing public API interface stays compatible.

Executable shape: `conversation_preload_page.output.json`; the shared total
message-category bound and encrypted-body limit are enforced by SDK guards.
The schema is source evidence and does not add a tool to discovery.

A page preserves raw JSON object records without float conversion:
`clearUnreads` metadata, optional `msgs` and `groupMsgs` message arrays. Missing
message arrays differ from empty arrays. `pageMsgs` is an unsupported category
and may be counted but not interpreted as personal chats. Nullable/missing
metadata does not prove exhaustion or a complete inbox.

Bounds: one request, at most 8 MiB wire bytes and expanded HTTP JSON, 5,000 metadata records and
1,000 total message-category records. Source contexts inherit session
cancellation; the caller supplies no URL, credentials, hidden PIN or opaque
cursor. Missing metadata, oversized arrays and non-object records fail the whole
page. Wire and expanded bodies are read with a limit+1 overflow sentinel before
JSON resolution, so a valid JSON prefix followed by excessive padding cannot
be mistaken for a bounded response. No partial subset is returned after an error.

No persistence happens at this layer. A later domain adapter must validate exact
typed IDs, classification and account ownership before updating the catalogue.
Any explicit import of available historical records uses the existing silent
identity transaction, never ordinary live/replay ingestion. Unknown IDs/names do
not confer send permission. No historical first-incoming fact may be fabricated.

The author's finite recent-message results are not a per-peer paginated history
API. No `has_more`, full-history window or stable Strangers flag is inferred.
Installed source/page shape and absence of read effects require separate evidence.

## Domain and directory boundary (candidate)

The adapter requires a bound account and explicit numeric isGroup=0/1 metadata.
It preserves exact numeric/string IDs, typed namespaces and optional name/last-ID
fields; duplicates, missing classification or foreign direct-message ownership
reject the entire snapshot. Sender zero and the actual owner ID both normalize
self-sent direction. OA/page records are counted but never converted to direct
chats. Available records retain empty persistence source and nullable novelty.

The optional guarded port inherits shared authentication cancellation. The service
refreshes metadata at startup and periodically; explicit history operations can
import the limited snapshot silently. Metadata merge accepts at most 5,000 validated
entries in one transaction, checks bound account and current collection policy,
keeps stronger existing names/provenance and exposes preload_catalog for newly
discovered records. It never inserts messages, Events, first-contact facts or
send permissions. lastMsgId is evidence only, not a stored guessed history cursor.
Existing catalogue completeness stays false. Live normalization and metadata
refresh were verified; the source returned only one already stored record for
the requested historical peer, so its older message remains unrecovered.

## Trusted local probe

`zl-mcp -config CONFIG probe-preload` sends the owner-only
`cli_probe_preload` RPC through the existing private control socket. Executable
input/output contracts are `cli_probe_preload.input.json` and
`cli_probe_preload.output.json`. No arbitrary URL, IDs, credentials, source cursor
or additional argument is accepted. It is not in MCP discovery or the public
membership Call route.

The probe uses the service's guarded current session with a 30-second source
budget. CLI transport has a 35-second deadline; the private server permits a
40-second response. It returns bounded counts, category availability, a fixed
error category, a closed normalization-reason enum and an optional numeric source code. No identity, name, text,
raw response or error string is returned. metadata_persisted/messages_persisted,
catalog_complete/history_complete are always false. Source availability is not
historical completeness. This probe never writes the source page into storage.

Synthetic production-service acceptance proves the Unix route uses one restore
and one source call while message, identity, catalogue, novelty, subscription,
delivery and send tables stay unchanged. Malformed/injected RPCs are rejected
before network access; protocol failures omit private underlying error text.
Installing a candidate binary and live probing require the verified migration
procedure. The probe is currently installed as a verified local candidate; publication remains pending signed commits.

Observed live normalization on 2026-10-04 confirmed empty optional SDK string
fields must be accepted (actionId was the blocking field). Empty actionId,
cliMsgId, userId and realMsgId carry no message/peer identity. Required msgId,
uidFrom, idTo and ts remain strict. A bounded owner probe successfully normalized
one live response. Periodic refresh updates metadata only and does not establish
complete message ingestion or history coverage.
