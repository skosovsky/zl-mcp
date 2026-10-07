# Retained account archive

Status: internal persistence contract with owner-only capture/status/coverage/remove
routes. The user authorized the complete account export.

Use a dedicated private directory beneath runtime state, outside iCloud and the
checkout. The single service holds the account lock. Reject directory/file
symlinks, nonregular files and permissions allowing other users. Do not share the
15-minute import staging directory, key, claims or lifetime.

Each canonical source UUID reserves a permanent local claim before publication.
Save all validated files and their complete one-to-one typed mappings together;
never omit a file to fit a quota. Bind source UUID, account, source digest,
creation/expiry and original transport byte counts inside AES-256-GCM ciphertext.
Use a fresh nonce and UUID/version as associated data. No URL, transfer key,
session or credentials are needed in this retained representation.

Default lifetime is seven days. Owner-selected lifetime is one hour to thirty
days. An exact save retry returns the original manifest without extending expiry;
changed account, contents or lifetime conflict. Removed, expired or incompletely
published UUIDs remain spent and cannot request or publish another export.

Retain at most two archives within an explicit ciphertext byte budget (maximum
2 GiB). Each decoded source remains bounded by the existing 512 MiB total,
256 MiB per file and 1000-file limits; framing metadata is at most 1 MiB. Capacity
failure must not partially publish or silently truncate an account archive.
Atomic private publication and successful removal fsync the containing directory.

Read authenticates and validates the complete source before returning any file.
UUID, account, digest, typed mapping, framing, counts and expiry must agree.
Corrupt/unknown artifacts fail closed. Owner expiry removes the encrypted source;
explicit owner removal is idempotent once absent and retains its spent claim.
Private UUID temporary artifacts left by interruption are cleaned up, never
replayed as captures. Missing keys are not silently regenerated over old archives.

Offline reads return owned buffers and verified original mappings; they require
no Zalo session, download, identity remapping or phone synchronization. The caller
clears returned data. Stored file bytes remain unrendered source evidence: SQLite,
attachment, TTL, control and WAL validation are still required for inspection or
import. Neither storage nor successful download proves complete Zalo history.

The safe manifest contains source UUID, scope=account, captured/expiry times,
digest, file counts and stored byte counts, with history_complete=false and
import_performed=false. It contains no filenames, peer IDs or message bodies.
Save/read/remove perform no corpus, Event, subscription or send-journal writes.

## Owner commands

All commands use the running service's owner Unix socket. They are not public MCP
tools and never change collection permissions. Configure absolute runtime paths.

```sh
printf '%s\n' '{"request_id":"<fresh UUID>","archive_scope":"account"}' | zl-mcp -config /absolute/config.toml account-archive-prepare
zl-mcp -config /absolute/config.toml account-archive-capture <operation_id> <revision>
zl-mcp -config /absolute/config.toml account-archive-status <source_id>
printf '%s\n' '{"source_id":"<source_id>","since":"2026-09-25T17:00:00Z","until":"2026-10-06T17:00:00Z","offset":0,"limit":25}' | zl-mcp -config /absolute/config.toml account-archive-inspect
zl-mcp -config /absolute/config.toml account-archive-remove <source_id>
```

Prepare creates a durable authorization reference; only capture dispatches the
phone request. Run capture with the user present to approve synchronization.
Successful repeats return the original source and do not extend expiry.

Inspection is bounded coverage diagnostics, not a searchable message export.
The result authenticates the full source, returns up to 25 file ordinals and
source/period counts, bounds, WAL observations and the kinds in the first 50
examined rows per file. Use offset for remaining files. `sample_has_more` applies
to the row sample, `has_more_files` to files. Unknown SQLite stays visible as
`unreadable_sqlite`; successful reading does not authorize WAL imports. No raw
text, URLs, peer identities or attachment values are output. Message pagination
and detailed offline attachment decoding are not yet exposed by these commands.

The runtime cache is `StateDir/account-archives`, with a 1 GiB ciphertext budget
and a two-source limit. Expired files are removed during store inventory or
selected-source access, not by a wall-clock timer. UUID claims remain spent.
If the diagnostic cache cannot be authenticated at startup, archive operations
fail and the collector continues; the cache is not automatically repaired.

### Exact-file metadata investigation

Optional flat `conversation_type`/`conversation_id` select exactly the original
mapped conversation from the authenticated cache, without a new mapping request.
For that form, offset must be zero and limit must be absent or one. An absent
identity fails; another file is never inferred from similar counts or dates.

`include_metadata_diagnostics=true` adds bounded diagnostics for the same first
50 examined rows: presence/invalidity of BinNet, unsupported field counts,
attachment/title presence, source/title equality, fixed action-literal categories
and per-distinct-action digest/byte-length/UTF-8/NUL observations. The literals
are observations, not accepted semantics. Unknown actions remain `other`; digests
do not make unknown attachments readable. No action value, title, source text,
parameters, URL or credential is returned. These observations do not change
conversion, WAL/control/TTL admission or authorize import. The source remains
immutable; no new phone request is made.

`metadata_diagnostics.text_projection_classes` reports counts from the same
converter text rule (`plain`, `rich`, `unsupported`, `invalid`, or
`missing_metadata`) without returning text or corpus records. Plain projection
for one absent/empty-action attachment follows the verified native MSG_TEXT
fallback; unknown field counts remain visible. These counts never waive WAL,
control, identity, expiry or history-import admission.

The optional `files[].rejected_row_reasons` partitions examined rejected rows by
the first failed scalar check, using fixed field-category keys only. It reports
no source value or guessed repair and does not classify unexamined rows. Paging
continues past rejected rows exactly as before.
