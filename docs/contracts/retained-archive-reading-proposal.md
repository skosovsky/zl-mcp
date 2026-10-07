# Retained archive reading: decision proposal

Status: proposed, not implemented or accepted. The existing corpus and import
contracts remain authoritative until this proposal is agreed and executable
schemas are added.

## Problem and evidence

The retained phone package contains 56 authenticated mapped SQLite main files.
All have WAL-mode header bytes. SQLite documents that WAL mode persists across
reopening and that commits can reside only in the separate WAL file. Thus a WAL
marker alone neither proves a broken export nor proves checkpoint completion.
The existing independent regression demonstrates a main image that passes
integrity checks but lacks both a later message and a recall/control transaction.
Running a checkpoint on our copied image cannot recover absent producer WAL.

References: [SQLite WAL](https://www.sqlite.org/wal.html),
[file format header](https://www.sqlite.org/fileformat.html),
[immutable URI](https://www.sqlite.org/uri.html).

Read-only inspection of the installed consumer's format-1 reader shows that it
opens each transferred `.db` using CREATE/READWRITE and queries ChatContent. This
is consumer compatibility evidence; it does not establish how the phone produces
or checkpoints those files. No producer WAL/completeness claim is inferred from
that consumer path, matched aggregate counts or successful immutable reads.

The all-file bounded diagnostic sampled 891 source rows through the requested
cutoff. SQLite scalar validation rejected 128; the remaining 763 have valid
metadata: 460 plain-text projections and 303 unsupported projections. Eight files
have additional unsampled rows. These are sample counts, not complete message
counts; the exact rejection reasons and unsupported content require investigation.
Two records in the requested historical direct conversation now project as plain
text after the native absent-action fallback correction.

## Proposed user-visible behavior

Provide the saved package as a separate, explicitly incomplete source, without
writing its records into the current corpus. Existing corpus browse/search stays
the default. An explicit retained source selection must never silently fall back
to the corpus or a new phone transfer.

Prefer extending the existing conversation/message browse workflow with a flat
optional `source_id` rather than creating an independent family of browse tools.
A narrow read-only source inventory is needed so the client can discover usable
sources and their capture/expiry/coverage metadata without guessing UUIDs.
Executable schemas and examples must be defined before implementation.

Message browse preserves typed conversation identity, explicit RFC3339 interval,
bounded pages, order, excerpts/full-text resource URIs and stable source-bound
cursors. It returns source capture/expiry, main-image bounds, `history_complete=false`,
WAL uncertainty, unsupported/rejected/expired counts and `has_more`. Source and
corpus items never share resource namespaces or checkpoints. A snapshot message
is not eligible as an outgoing reply anchor until independently present in the
current corpus under existing send rules.

Return only verified visible-text projections. Unsupported content remains
visible through fixed kind/reason/count metadata; never render arbitrary
MsgContent for media, execute metadata, fetch attachment URLs or fabricate a
reply, sender or first-incoming fact. Known source/live tombstones and expiry
must suppress applicable records. Unclassified source controls must cause an
explicit unavailable-source result rather than serving an accepted prefix.
Even a source without visible controls has unverified latest-WAL changes.

Retained typed file mappings must resolve the requested conversation exactly.
Sender identity requires a verified mapping; a noise/plain archive ID is not a
session peer ID. Missing sender mappings must be reported explicitly, and cannot
trigger hidden network calls during offline reading. Mapping acquisition, if
needed, must occur through the single account owner and be persisted separately
from immutable archive bytes with explicit provenance.

Authenticate every source/cursor/resource through the existing service; enforce
account binding, current collection policy, source expiry/removal and cancellation
on each read. Retaining the whole account privately does not implicitly enable
cloud access to every group. Original cache ciphertext/digest/expiry remain
immutable. Cache reads create no Events, subscriptions, acknowledgements,
first-incoming facts, sends or history-import checkpoints.

## Acceptance before availability

- Agree separate snapshot reading versus strict corpus-import admission.
- Define executable schemas, source inventory, error codes and source/resource
  ownership rules before implementation.
- AAA tests: typed-ID collision, invalid/revoked ownership, expired/deleted source,
  changed source/filter/cursor, page boundaries, rejected-only pages, unsupported
  metadata, TTL/tombstones, controls, cancellation and restart.
- Prove no hidden phone/network request, corpus writes, Events, ack or sends.
- Reuse the retained source for the requested historical day; verify exact
  conversation/message identity and visible text privately, reporting gaps.
- Update both skills and public tool descriptions only after these capabilities
  exist; verify discovery and actual reading through the connected client.

This proposal does not close the strict import gate or the original live filter/
first-incoming/out-of-catalogue-send criteria. If separate snapshot reading is not
accepted, those read capabilities must not be advertised as implemented.
