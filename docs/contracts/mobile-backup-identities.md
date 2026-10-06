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

Mapping failures expose only a closed diagnostic stage (REQUEST, HTTP_STATUS,
WIRE_LIMIT, HTTP_DECODE, BODY_LIMIT, OUTER_ENVELOPE, CIPHER_ENCODING, DECRYPT,
INNER_ENVELOPE, SDK_REQUEST, AUTH_COOKIE_SCOPE or MAPPING_SHAPE), numeric HTTP/upstream codes when
applicable, and no source text, IDs, credentials or response payload. These
stages distinguish transport/envelope failures from typed mapping rejection.

The native client inspected in this investigation advertises API version 691
(type 30), whereas the vendored SDK defaults to 665. The znoise adapter pins
version 691 for this endpoint only, retaining the existing authenticated session
and API type. Other endpoints and login defaults remain unchanged. A live outer
response returned error code 600 at version 665; a subsequent authorized version-691 probe returned the same outer code 600.
The version change did not resolve the failure and does not establish its cause.

### Authentication scope compatibility

Saved web-session metadata shows zpw_sek scoped to chat.zalo.me, so the normal
jar omits it at zwid.api.zalo.me. Static native main-process code sets zpw_sek
also on zalo.me; its identity request therefore includes that cookie. This is
a proven request difference, not yet proof of the outer code-600 cause.

For exactly `https://zwid.api.zalo.me/api/znoise` (no alternate port, userinfo or
fragment), the adapter may borrow only zpw_sek from the existing authenticated
chat.zalo.me jar for the one bounded nonredirecting request. If the destination
already has it, use normal jar handling. An absent/ambiguous source token fails
closed. Never rewrite cookie domains, persist copied credentials, forward other
host-only cookies, broaden this to generic URLs or log cookie values. Current
session ownership, cancellation, exact IDs and complete reply validation remain
required. Other endpoints retain their existing jar policy.

Live acceptance subsequently passed with the same version 691 and the scoped
borrowed-cookie fix: the correlated offer's plain uid mapped to the current
session owner, and the offer operation reached `offer_ready`. This verifies
current-session account mapping for this case. It does not establish archive
conversation/sender mapping completeness or successful history persistence.
