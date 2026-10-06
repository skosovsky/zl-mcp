# Private selected-archive snapshot

Status: internal storage prerequisite; not a phone request or public import tool.

Persist only the selected SQLite main image after authenticated archive mapping.
Do not retain the account-wide transfer, offered URL, transfer key, cookies or
unselected conversations. The single service owns the store beneath its private
state directory outside the checkout. Callers cannot supply a filesystem path.

Use a dedicated randomly generated 256-bit local key, stored with mode 0600 in a
0700 directory. Encrypt the selected bytes and private binding metadata with
AES-GCM and a fresh nonce. Authenticate the snapshot UUID as associated data.
Key and ciphertext survive service restart; neither is exposed through MCP,
logs, JSON or formatting. Do not reuse the MCP bearer token or Zalo credentials
as a cryptographic key. A missing/corrupt key fails closed; never rotate it
implicitly over existing snapshots.

Bind each snapshot to the normalized request UUID/fingerprint, exact account,
typed conversation, canonical source filename and SHA-256 of selected bytes.
The snapshot UUID is the request UUID; it is a reference, not authority. Reads
must match account and normalized request and validate authenticated metadata,
digest, standalone SQLite header and expiry before returning owned bytes.
Repeat saves with the same binding and bytes return the existing snapshot;
changed bytes/bindings conflict instead of replacing the source midway through
pagination. Corruption or a mismatched identity returns no record prefix.

Lifetime is 15 minutes from creation and cannot be extended by a retry. A source
expired or removed before completion is explicitly unavailable; restart must
not issue another phone request. The future driver persists source identity and
page checkpoint in its operation journal. Snapshot storage alone neither resumes
an import nor accepts an unverified WAL image for conversion.

A private append-only reservation index retains up to 100,000 snapshot UUIDs,
including expired, removed or incompletely published snapshots. It contains no
account IDs, filenames or message data. Reserve and fsync before publication;
reusing a spent UUID without its snapshot fails instead of renewing its lifetime.
An interrupted reservation may make its source unavailable, but cannot silently
redispatch the phone operation. The single service state lock is required;
multiple independent writers to the same store are unsupported.

Use generated fixed filenames, confined file operations, private permissions,
atomic publication and bounded reads. Reject symlinks and nonregular artifacts.
Bound storage to 20 snapshots and a configured total ciphertext byte budget,
at most 512 MiB; one main image remains bounded by 256 MiB. Expired snapshots are
removed at startup/cleanup and while admitting a new snapshot; reads remove an
expired selected snapshot. Clean up confined, private, UUID-named temporary files
left by interrupted writes; reject unknown files instead of deleting them.
Fsync publications, reservations and successful removals. Terminal operations
explicitly remove their snapshot. Compare opened-file identity and permissions
with the confined pre-open metadata before reading or appending.
Clear returned plaintext on completion. Snapshot lifetime is an internal staging
lifetime, not permission to expose expired messages: the reader/converter must
still apply original message and quote TTL before corpus persistence.

This store performs no SQLite corpus writes, history checkpoint advancement,
Events, subscription changes, phone dispatch or session restore. Production
driver wiring, source safety gates and full content/control semantics remain
required. Synthetic acceptance covers restart, idempotency, account/selection
binding, corruption, expiry, capacity, private cleanup and source-byte ownership.

## Synthetic acceptance: 2026-10-06

`TestSnapshot*` covers unchanged encrypted bytes on retry, owned plaintext after
restart, exact request/account/source binding, fixed expiry and spent UUIDs,
corrupt/missing keys, ciphertext tampering, symlinks, private permissions, byte
and count quotas, cancelled ownership waits, interrupted publication, malformed
reservation indexes and preservation of unowned files. Root and nested module
test/race/vet passed; CGO-free macOS arm64 and Linux amd64 builds passed. No model
eval, phone request, production import, subscription edit or journal ack was
performed for these checks. This evidence does not accept the unwired mobile
driver or establish the producer's WAL/control semantics.
