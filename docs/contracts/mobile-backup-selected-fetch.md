# Selected mobile archive fetch candidate

Status: internal staged pipeline; no CLI/MCP exposure or corpus import.

FetchSelectedArchive accepts a normalized explicit selection, correlated private
offer, independently configured Downloader and existing guarded identity mapper.
Validate dependencies/request/numeric selected ID before network. Fetch exactly
offer.FileSize under the request ciphertext budget; decode format 1 under the
fixed 512-MiB expanded archive ceiling. Build a complete typed filename request,
map through the current session and select the exact conversation. Any stage,
cancellation or incomplete mapping fails with a fixed error and no partial result.
Clear downloaded ciphertext and all decompressed files on failure. On success,
transfer ownership of the selected file alone; clear other file regions before
returning. Returned selected file owns its byte region and Clear zeros it. The
underlying allocation can remain the bounded full expanded archive until Clear;
this avoids an additional selected-file copy while clearing unrelated regions.

The service wrapper holds the current port/session for all stages, uses the
prepared attempt's durable selection and follows caller/service/guard lifecycle.
The guard exposes a borrowed MobileBackupContext linked to revocation and listener
shutdown; it creates neither a session nor a listener. A source must support
correlated offers, identity mapping and that guard scope before dispatch. All
source data remains private/redacted. Success proves only that opaque selected
file bytes were obtained under those stage checks, not SQLite/message validity,
complete history or authenticated archive provenance. No private URL/key is saved.

No default download hosts, public route, automatic retry, Events or message writes
are introduced. Real mobile archive, account mapping and selected row acceptance
remain required before enabling an import source.

The returned selected archive retains private request UUID, normalized request
fingerprint and typed conversation binding. These are assigned only after exact
file mapping/selection succeeds and cleared with the archive. The internal
[prepared-page reader](mobile-backup-prepared-page.md) requires this binding before
SQLite or sender-map access; this is in-process provenance, not cryptographic
account authentication or an external cursor token.
