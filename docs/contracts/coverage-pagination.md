# Coverage representation and continuation

R02 decision, 2026-10-18. Existing calls retain the full, strict response shape.
`coverage_mode` is optional on both searches, both contexts, conversation detail
and corpus browse: `full` (default) or `compact`. Old inputs produce the same
coverage keys and full `known_gaps`; nothing is silently truncated for old clients.
Archive browse retains its independent archive coverage contract.

A compact corpus coverage object has `known_gaps: []` and a required, explicitly
present `coverage_details` object:

```json
{"mode":"compact","total_known_gaps":800,"gaps_truncated":true,"gaps_cursor":"server-issued opaque cursor","snapshot_at":"2026-10-18T00:00:00Z"}
```

`total_known_gaps` is the exact count in the observed snapshot, not a claim about
all missing Zalo history. `gaps_truncated` is true precisely when this compact
representation omits at least one gap. When there are no gaps it is false and
`gaps_cursor` is null. Other coverage timestamps, retention and
`history_complete:false` retain their existing meaning. Presence of the nested
object is the representation discriminator; full replies omit it entirely.
Clients with old strict output validators must keep requesting full mode until
they load the new schemas. Updated skills request compact mode explicitly.

`zalo_list_coverage_gaps(conversation_type, conversation_id, cursor?, limit?)`
reads only local coverage; limit defaults to 50, range 1–100. The first request
may omit cursor to observe current coverage. For continuation use the exact typed
identity and the opaque cursor returned either by compact coverage or a prior
page. `gaps` is ordered by stable insertion ID ascending (which is not exposed),
not by timestamps: equal or overlapping intervals remain independent records.
`has_more` and nullable `next_cursor` expose continuation; the count and
`snapshot_at` are constant for the snapshot. Every page rechecks collection
policy. No admission bypass or upstream history/read/write request is performed.

Cursors are authenticated with the existing private state key, bound to typed
identity, representation/version, insertion high-water mark and content digest,
and expire after 24 hours. Later appended gaps are excluded. If any selected gap
is modified or removed (including retention/housekeeping), continuation returns
`CURSOR_EXPIRED` rather than silently returning a different snapshot. Start over
without the cursor or obtain new compact coverage. Invalid/tampered/wrong-target
cursors return `INVALID_ARGUMENT`; revoked collection policy returns
`PERMISSION_DENIED`. Normal errors use the existing control envelope. Snapshot
checks also occur after collecting a page, so an update cannot yield a successful
mixed page.

Corpus browse now reduces an oversized page while retaining its original signed
insertion snapshot and filters. The next cursor points after the last returned
record; unseen records are neither skipped nor acknowledged. Current policy and
visibility/retention still apply on each reduced read. If a single item or full
coverage cannot fit, `RESPONSE_TOO_LARGE` recommends compact coverage or the
bounded content tool and explicitly reporting an oversized-metadata limitation.
A context/search with full coverage may still exceed the old budget; callers must
explicitly opt into the new compact representation rather than receiving a
silently changed old response.
