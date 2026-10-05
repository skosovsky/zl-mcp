# Format-1 encrypted archive boundary candidate

Status: internal offline assembly; actual phone archive compatibility remains open.

Native decrypt_file (0x4160) writes decrypted chunks back with the original fread
byte count (0x4298–0x42a8), with no PKCS unpadding. GetPaddingLength (0xe324) rounds
to an AES block boundary without adding an extra block to aligned input.
DecryptCBC (0x107f4) copies the supplied IV into its own buffer (0x10880–0x1088c),
so decrypt_file's unchanged zero IV starts each 65536-byte call. It does not shrink
its output. decompress_file later bounds checksum/decompression by the declared
container end, independently of the physical decrypted file end.

ReadFormat1Archive accepts offset-zero block-aligned ciphertext under an explicit
byte budget, using DecryptFormat1. Read the declared end from bytes 6–9 only after
magic/alignment checks; require 18 <= end <= decrypted byte count. Pass exactly
that region to ReadPlainArchive for full XXH32, file-table, bounded XZ and exact
split-size validation. Report ciphertext/container/trailing byte counts separately.
Do not interpret or require zero/PKCS bytes in the trailing region. Ignored tail
remains inside the caller's ciphertext budget; no alternative offset/key/format is
tried. Decrypted temporary bytes are cleared on every path. Failure returns no
archive or partial counts. Successful result owns opaque file buffers and Clear
zeros them. JSON/formatting expose neither filenames nor file content/counts.

This follows observed reader behavior, not a verified producer padding scheme.
Checksum covers the declared region only and is not cryptographic authentication.
Tail content is not authenticated/validated; real offer correlation, trusted host,
account mapping and SQLite/message semantics remain independent requirements.
No download, native execution, file persistence, corpus import or Events occur.
