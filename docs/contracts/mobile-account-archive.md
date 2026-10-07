# Owned account archive acquisition

Status: internal acquisition prerequisite; retained storage and owner CLI remain
separate. No public MCP capability is added by this contract.

`FetchAccountArchive` consumes one bounded authenticated transfer using the
existing session transport. It validates the complete container, all canonical
file names and an exact one-to-one typed identity mapping for every file before
returning any account data. Missing, extra, duplicate or ambiguous mappings reject
the whole result. Download and mapping each occur once. No period filter or
single-conversation selection may discard files at this boundary.

The result owns every decoded file and the verified mappings. JSON and formatting
must expose neither file bytes/names nor identities, URLs or keys. `Clear` clears
all owned file buffers and mappings. Cancellation or any failed stage returns no
prefix and clears temporary data. Byte and file quotas remain the existing
container/download limits; rejected formats are not silently skipped.

The existing selected-chat fetch uses this boundary, then transfers ownership of
exactly its verified selected file and clears all other files. Its request and
fingerprint semantics remain unchanged. Account acquisition does not open SQLite,
establish export completeness, accept WAL/control semantics, import data, create
Events, write a persistent archive or issue a phone request. The service must bind
acquisition to the unchanged authenticated account before retaining any result.
