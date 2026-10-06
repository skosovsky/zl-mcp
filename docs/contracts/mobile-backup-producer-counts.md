# Mobile backup producer count diagnostics

These diagnostics are observations, never archive admission or completeness proof.
The candidate upstream model at
[PR 269, revision 29d01c4](https://github.com/RFS-ADRENO/zca-js/blob/29d01c4732391cf1b68a6630cd715199ef46348a/src/models/SyncEvent.ts)
declares `db_info.backup_db.msg_total` and `msg_thread`, alongside `db_format`.
Their units, filtering and relationship to rows in individual files remain
unverified. `is_full_transfer` is also declared, but our decoder does not retain
it; its name is not evidence of a checkpointed SQLite image.

After successful offer validation and account mapping, emit one
`mobile_backup_producer_counts` log with a `claims` object conforming to
[the executable schema](mobile_backup_producer_counts.output.json).
`state=available` requires both counts to be JSON unsigned integer literals
within uint64; preserve values exactly without float conversion. Missing metadata
is `absent`; a missing count, wrong type, negative/fractional/exponent/overflow
value or invalid metadata is `invalid`. No partial pair is logged. Unknown fields
are ignored; no raw metadata, account, URL, key, filename or message text is logged.
All states carry `verification=unverified`. Invalid optional claims do not alter
an otherwise valid offer. They do not change WAL/control gates or request another
phone sync. This log does not persist a source proof in an import checkpoint.

Counts may eventually identify a mismatch only after their scope is established.
Equal counts cannot establish that IDs, edits, expiry and recall state agree.
SQLite [WAL persistence](https://www.sqlite.org/wal.html) and the
[backup API](https://www.sqlite.org/backup.html) distinguish copying a main file
from exporting a logical snapshot. Running a checkpoint on an already detached
main file cannot recover the absent producer WAL. Existing synthetic regressions
demonstrate both a complete checkpointed 2/2 image and an intact but incomplete
main image. Neither establishes which export method the phone uses.
