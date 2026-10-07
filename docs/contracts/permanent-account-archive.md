# Permanent local account archive

Status: owner promotion, restoration and inspection are deployed. Bounded public
message reading is deployed through the separate [MCP read contract](archive-mcp-reading.md).
The selected real September 26 read and all-file first-page gate check are recorded
in [current acceptance](../conversation-completion-open-gates.md); unknown controls
and actual cloud-agent archive invocation remain open.
The owner requested a local copy that avoids repeated phone
synchronization. A private encrypted backup has already been preserved.

`cli_preserve_account_archive` is an owner-only operation on an existing source,
with explicit `retention=until_owner_deletion`. It performs no acquisition,
identity remapping, phone request, corpus import, send or subscription mutation.
It must not appear in public MCP discovery or grant cloud access to groups.

The durable library is outside iCloud, beneath
`StateDir/account-archive-library`. It uses private regular files and directories,
authenticated account binding, size limits and atomic publication. Copying a
container without its decryption key and complete typed mapping is insufficient.
Verify the complete decrypted container and every file digest before admitting
a saved source. Keys and plaintext never appear in logs, receipts or the repo.

The service-managed store is the `sources` subdirectory, with an independent key,
two-source limit and 1 GiB ciphertext budget. Dated private recovery copies live
outside that subdirectory and are never automatically imported. The cache expiry
policy is unchanged. Owner promotion re-encrypts authenticated source bytes under
the library key while preserving original manifest and source digests; the
original stored byte count describes the captured container, not library framing.

The owner command reads JSON from stdin:

```sh
printf '%s\n' '{"source_id":"<source UUID>","retention":"until_owner_deletion"}' | zl-mcp -config /absolute/config.toml account-archive-preserve
```

A repeat first authenticates the library receipt and does not require cache or a
connected collector. Missing or corrupt durable sources under spent UUIDs fail
instead of republishing them. Use `source_storage=library` for owner inspection; the output identifies both
store and effective retention. `account-archive-remove <source UUID> library`
explicitly removes the managed durable source and retains its spent claim.
Neither route silently falls back to cache. Bounded public message reading does
not prove complete history or permit unclassified controls.

Keep original source UUID, capture time, source digest, file bytes and mapping.
The original manifest's `expires_at` describes the temporary capture cache;
durable retention is reported separately and never inferred from a changed date.
Expired cache must not destroy a separately preserved source. It must remain
possible to authenticate a preserved backup after cache expiry without obtaining
a new export. A removed source cannot be resurrected by automatic retry.

An exact repeat returns the original preservation receipt without rewriting the
source. A changed account or digest under the same UUID conflicts. Read and
restart never refresh data or synchronize the phone. No supported code path
silently falls back to acquisition after a missing, corrupt or expired cache.

Known message TTL and recall rules remain applicable; permanent container
retention does not mean indefinite visibility of every message. WAL uncertainty,
unsupported controls and unknown payloads remain explicit source limitations.
Promotion does not waive the strict corpus-import contract.

Acceptance requires restart and post-cache-expiry reads of the same source,
account/digest mismatch and tamper rejection, typed-ID collisions, capacity and
cancellation checks, idempotent promotion/removal and restoration of an encrypted
backup. Verify absence of network requests and writes to corpus, Events, sending
journals or subscriptions. Actual public reading and skills are a subsequent
integration step governed by the retained-source reading contract.

## Explicit backup restoration

`cli_restore_account_archive` requires the existing source UUID, original expected
source digest, a dated `backup_name` (YYYY-MM-DD) and explicit permanent retention.
Only the corresponding private child of `StateDir/account-archive-library` is
eligible; the command accepts no arbitrary filesystem path. It reads the backup
key, spent claims and complete encrypted container without modifying the backup,
creating a key or applying cache expiry cleanup. Original cache metadata must
still validate structurally, but its historical expiry does not delete this
explicitly preserved copy. Authenticate account, UUID and every source file,
then match `expected_digest` before publishing under the independent library key.
Changed provenance under an existing UUID conflicts. An explicitly removed
library UUID cannot be restored by this operation. Retry revalidates the named
backup and returns the existing receipt; it does not silently choose a different
backup, source or acquisition path. Backup tampering, missing keys, unknown IDs,
private-permission violations, directory symlinks and cancellation fail closed.

```sh
printf '%s\n' '{"source_id":"<source UUID>","backup_name":"<YYYY-MM-DD>","expected_digest":"<original source digest>","retention":"until_owner_deletion"}' | zl-mcp -config /absolute/config.toml account-archive-restore
```

The CLI allows a bounded 140-second owner request for promotion/restore/inspection;
caller cancellation propagates to local work. Tests restore an already expired
backup after deletion of the source cache, preserve original bytes and metadata,
and reject foreign accounts, mismatched digests, corrupted ciphertext, absent
keys, unsafe permissions, symlinks, cancellation and revival of a removed UUID.
The owner RPC test verifies exact dated-path selection and unchanged acquisition
counters and corpus/Event/subscription/send tables.

## Live acceptance, 2026-10-07

The clean signed source build was installed after successful Linux/macOS CI and
native startup/shutdown acceptance, using the existing single LaunchAgent and a
verified private backup of all runtime state. Configuration and agent definition
were unchanged; prior corpus, identities, tombstones, first-incoming facts, send
and quote journals and subscription definitions survived. No second listener,
phone acquisition, send or acknowledgement was used.

Explicit restoration from the dated encrypted backup verified the original
source digest and all 56 mapped files (54 direct, two group). Repeating restore
returned the identical receipt, and backup bytes/key/claims remained unchanged.
Owner inspection paged through all files in the permanent library with effective
retention `until_owner_deletion`; no corpus import or public archive access was
enabled. This proves durable local availability, not complete Zalo history or
verified WAL checkpoint/unknown-control semantics.

The actual connected MCP client, loopback HTTP and installed STDIO bridge returned
connected/authenticated without a last error. Existing v2 recovery discovery and
the active subscription remained available; the journal was empty and no ack was
sent. Installed binary SHA-256:
`5c29ebdc84c715c795888c64c4e13a624b761bd88d3b69e52b1e561266eb5970`.
[Linux/macOS CI](https://github.com/skosovsky/zl-mcp/actions/runs/37590841020)
completed successfully for source `f7038ce`.
