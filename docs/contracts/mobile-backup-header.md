# Mobile backup file-table candidate

Status: superseded in part by [the complete container contract](mobile-backup-container.md).
The former interpretation of [6,10) as a header length and checksum over the
file table alone was incorrect. It is the complete declared container end;
checksum covers table **and compressed payload**. The table-only fixture remains
useful solely for independent table layout and exact filename tests.

The bounded internal table parser reads count/name lengths/names/declared sizes
from the beginning of the already checksum-validated container body. It returns
its exact consumed table offset; compressed bytes follow it. Bounds: table at
most 64 KiB, count 1–1000, names 1–128 bytes, individual sizes at most 256 MiB and
sum at most 512 MiB. Accept only canonical flat decimal .db/group_<decimal>.db
names; reject empty/duplicate/unsafe names and zero sizes. No path is opened.

The XXH32 implementation is independently checked against upstream seed-zero
vectors. Format corruption detection does not authenticate the archive. Complete
XZ checks and file splitting do not establish SQLite validity, account ownership
or plain/noise ID mapping; these remain gates before corpus mutation.
