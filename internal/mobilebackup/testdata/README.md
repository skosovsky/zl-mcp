# Synthetic header evidence

`format1-header.hex` contains only an invented plaintext file table: two decimal
names beyond 2^53 and declared sizes. It has no SQLite/message payload, account,
archive key or private material. Its checksum and `checksum-vectors.json` were
generated independently with official xxHash v0.8.3 `XXH32(..., seed=0)` in a
temporary C helper. Input vector byte i is `(i*37+11) modulo 256`.

The official reference header/tool is not vendored or needed to run tests.
These fixtures check parser/checksum behavior; they do not prove decryption,
archive layout compatibility, message recovery or account identity mapping.

`format1-blocks.hex` is an independent synthetic ciphertext generated with
/usr/bin/openssl AES-256-CBC, no padding, zero IV reset for each 65536-byte chunk.
The nonsecret test key text is `0123456789abcdef` repeated four times; AES key
bytes are the first 32 uppercased ASCII characters. Plaintext length is 65552;
byte i is `(i*37+11) modulo 256`, except the prefix is format1-header.hex.
Ciphertext SHA-256 is c3f0242771275a89d781593d7fcb2c4e96034c396c4c91ef27bb749107a7ad3a.
The second chunk verifies IV reset independently of the Go transform. This is
not an encrypted real backup and contains no private messages or real key.

`xz-vectors.json` was generated independently using Python's standard `lzma`
module (liblzma), FORMAT_XZ, one LZMA2 filter with a 1 MiB dictionary. It covers
empty output, repetitive synthetic text, invented binary bytes, and check types
none/CRC32/CRC64/SHA256. Tests assemble concatenated streams from these vectors
and deliberately modify container fields with recomputed CRCs. No real archive,
message, key or account data is present. Valid compression does not authenticate
content; alternative compressed representations can decode to identical bytes.
The multiple-block vector uses XZ Utils 5.8.3 with `--threads=1`,
`--block-size=256` and `--lzma2=dict=1MiB` over 1024 synthetic bytes.

Correction, 2026-10-04: format1-header.hex is a synthetic table-only fragment.
Its length/checksum describe only that fragment, not a real mobile archive; it
is no longer accepted as a complete container. Native evidence establishes that
the real declared length and checksum include compressed bytes after the table.
The OpenSSL block vector also proves only the transform, not archive framing.

`format1-archive-vectors.json` contains complete invented containers: one canonical
decimal filename and nonprivate synthetic text. Python/liblzma creates XZ/CRC64;
official xxHash v0.8.3 XXH32(seed=0) computes the full declared-body checksum in a
temporary C helper; OpenSSL AES-256-CBC encrypts the complete aligned byte stream
with no padding and the same nonsecret ASCII key used by format1-blocks.hex.
Fixtures cover exact alignment, a zero tail and a nonzero opaque tail. They test
observed reader boundaries, not the real producer's padding scheme or account
ownership. Ciphertext digests are recorded in JSON. No native code is executed.

`format1-sqlite-vector.json` is a complete invented encrypted archive with one
8192-byte standalone SQLite database, `902.db`. Python sqlite3 creates two
synthetic rows with the same timestamp; one has TTL=1 ms and the other has no
expiry. Python/liblzma, official XXH32(seed=0), and OpenSSL construct the archive
independently of Go. The AES key is the first 32 **uppercased** ASCII characters
of the public test key above. Recorded hashes verify encrypted and SQLite bytes.
Service tests use it to join the existing session, download, selection, row
mapping and expiry stages; it does not establish compatibility with real phone
archives, cookie scopes, production download hosts or upstream identity mapping.
