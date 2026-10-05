# Initial mobile backup request candidate

Status: optional SDK request, not called by the service or exposed as an MCP tool.
The native client generates a 2048-bit RSA SPKI DER public key encoded as base64.
The initial request uses advertised file service, encrypted GET parameters:
pc_name=Web, public_key, from_seq_id=0, is_retry=0, min_seq_id=0, temp_key="",
and the existing IMEI. Standard protocol query parameters and nretry=0 apply.
This candidate deliberately exposes only initial synchronization. Nonzero
continuation/retry semantics are not implemented or guessed.

Cancel uses the same advertised service, key, pc_name and IMEI. Validate canonical
base64 SPKI RSA-2048/exponent 65537 before network access. No private key, URL,
session value or retry switch is accepted from callers. Missing advertised
service fails explicitly, without guessed fallback. Each call is bounded to ten
seconds and makes one HTTP client dispatch with no redirects or application
retries. This does not promise exactly-once execution by the HTTP transport or
Zalo server; the correlated operation must handle an unknown result honestly.

Read at most 64 KiB + one sentinel byte for wire and expanded acknowledgement.
Reject overflow, bad envelope, non-success status and transport failures. Explicit
HTTP 401 retains authentication-required semantics. Upstream error codes may be
retained with a fixed message; raw body, URL and transport messages are excluded.
Timeout/transport failure after dispatch is an unknown result, not proof that
the phone request failed; the operation layer must not automatically retry it.

A successful acknowledgement means only that the request was accepted. Register
the correlated existing-listener receiver before dispatch, wait for phone
confirmation/archive metadata under a separate bounded operation, and unregister
after termination. Acknowledgement does not prove transfer, download, decryption
or historical message coverage. No request is made automatically at startup.
