# Offline XZ decoding candidate

Status: internal offline component; not a live mobile history source. Native
stub resolution confirms XZ concatenation. Real encrypted payload boundaries,
archive compatibility and account identity remain unverified.

`DecompressXZ` requires positive compressed/output budgets, each at most
512 MiB, and a context. Read at most compressed budget + 1 bytes. Before any
LZMA dictionary allocation, validate all concatenated streams backwards from
their footers: header/footer/index CRC32, matching stream flags, canonical
63-bit integers, exact block locations and headers, and all filter declarations.
Policy allows at most 100 streams, 1000 blocks total, and only a single LZMA2
filter per block with a dictionary no larger than 64 MiB. Check types supported
are none, CRC32, CRC64 and SHA256. Unsupported filters/checks fail explicitly.

The sum of indexed output sizes must fit the output budget. Then decode using
the pinned pure-Go XZ reader, consume to terminal EOF, verify stream integrity
and exact total size, and reject overflow, corruption, truncation or cancellation.
Return no partial output; clear owned input/output buffers on failure. Errors
are one fixed category without input bytes or library details. Library-owned
dictionary buffers are garbage collected, not claimed to be explicitly erased.

No paths, SQLite records, corpus writes or Events are produced. Archive header,
declared file sizes, SQLite schema and account/plain-noise identities require
separate validation before import. Rules are independently implemented from
the public-domain [XZ format specification](https://tukaani.org/xz/xz-file-format.txt).
The reader dependency retains its BSD-3-Clause license.
