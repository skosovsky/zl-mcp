# Mobile backup identity mapping candidate

Status: bounded offline codec; no upstream request or service integration. Native
format-1 restore imports filename owner IDs and message sender IDs through its
NoiseIdStore before insertion. REQUEST_NOISE_ID calls getnuid, whose advertised
client implementation encrypts numeric fids/gids and posts /api/znoise on
zwid.api.zalo.me. Returned fids/gids are matched by array position, with lengths
checked. This establishes a mapping step; real reply compatibility remains open.

Accept 1–1000 canonical positive uint64 decimal IDs total, separated by direct
and group type. Preserve integers beyond 2^53 via json.Number, never float64 or
JavaScript Number.parseInt. Reject duplicates within a type and malformed input.
The payload contains only nonempty fids/gids arrays of exact JSON integers.

Decode at most 256 KiB of already unwrapped plaintext data. Require exactly one
closed object with every requested fids/gids array present. An unrequested category
may be absent or an empty array; exact counts must match the original request. IDs may
be canonical decimal strings or JSON integers. For the group category only,
accept the native g-prefixed string representation and normalize its prefix;
never reinterpret a direct mapping as a group based on a local catalogue guess.
Reject zero, overflow, fractions, exponent/sign/leading-zero forms, null entries,
unknown/duplicate fields and many-to-one mappings within a type. Return typed
positional pairs only; no fallback to the archive ID itself. Unknown reply forms
fail before message persistence and must be resolved against real evidence.

This codec does not bind an archive to an account. Request/reply uses the guarded
current-session adapters below; associating it with the same correlated archive
operation remains required. Message sender/quote mappings and real schema need
verification before selected silent import. No Events/messages/files are written.

The optional SDK adapter now issues one encrypted form POST to the statically
observed HTTPS /api/znoise route using the existing session. It validates exact
request IDs independently, uses RequestOnce (no application redirect/retry), a
10-second bound, 512 KiB wire/HTTP-expanded limits and 256 KiB decrypted limit.
Require explicit success codes and non-null encrypted mapping data; HTTP 401
returns the shared authentication sentinel. Other failures are fixed errors.
Returned plaintext still requires the typed codec validation above. No live
request, new listener/login, service wiring or verified mapping is claimed.

The internal Client/collector adapters now combine the SDK request with complete
typed reply validation. Client snapshots and rechecks current account identity;
the shared session guard cancels in-flight mappings and discards even successful
results if another operation revoked authentication before return. Plaintext
reply buffers are cleared after decoding; raw SDK failures are not exposed.
This connects adapters only, not a service operation or MCP endpoint.
