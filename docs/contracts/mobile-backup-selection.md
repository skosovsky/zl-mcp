# Selected mobile archive file candidate

Status: internal offline selection, not authenticated import.

ArchiveIdentityRequest validates 1–1000 unique canonical archive filenames and
nonempty opaque file bytes under existing per-file/total limits. Strip only the
known group_ prefix and .db suffix; require canonical positive uint64 decimal
identity (no zero, overflow, signs, exponent or leading zero). Preserve IDs as
strings, grouped into direct/group arrays in source order. A colliding numeric
ID in different typed namespaces is allowed; duplicate full filenames are not.

SelectArchiveIndex accepts the archive, complete typed mapping pairs and an exact
numeric session ConversationRef. Require one pair for every declared file and no
extra/duplicate source pair. Source/session IDs must both be canonical positive
uint64 strings; reject many-to-one mapping within a type, but allow equal session
IDs across direct/group namespaces. Match both conversation type and mapped
session ID. Missing selected conversation is a distinct fixed unavailable error;
malformed/partial/ambiguous mapping is an invalid selection error. Return only a
borrowed file index; do not copy/clear archive bytes or open other databases.
Check cancellation before/between/after bounded validation and return no index
on failure. Never substitute a source ID when mapping is unavailable.

Complete mapping is required to establish which file belongs to the selected chat,
not permission to import every file. Caller must separately bind operation,
account, collection policy, guarded mapping and current archive provenance.
A mapping alone does not authenticate the archive or prove that it contains all
history. Sender/quote ID translation and real row semantics remain independent.
No network, filesystem, corpus or Events writes occur.
