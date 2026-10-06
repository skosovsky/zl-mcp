# Format-1 encrypted archive boundary candidate

Status: internal offline assembly; actual phone archive compatibility remains open.

Native decrypt_file (0x4160) writes decrypted chunks back with the original fread
byte count (0x4298–0x42a8), with no PKCS unpadding. GetPaddingLength (0xe324) rounds
to an AES block boundary without adding an extra block to aligned input.
DecryptCBC (0x107f4) copies the supplied IV into its own buffer (0x10880–0x1088c),
so decrypt_file's unchanged zero IV starts each 65536-byte call. It does not shrink
its output. decompress_file later bounds checksum/decompression by the declared
container end, independently of the physical decrypted file end.

ReadFormat1Archive accepts offset-zero ciphertext under an explicit byte budget,
decrypting complete AES blocks only. Read the declared end from bytes 6–9 only
after magic validation; require 18 <= end <= complete-block decrypted byte count. Pass exactly
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

Live diagnosis: scoped CDN authorization enabled a bounded real download, which
failed before selected-file mapping at the container stage. Fixed diagnostics now
distinguish ciphertext alignment, decrypted magic, declared end, checksum/table
and XZ/expanded size. Only length/remainder counts, fixed stages and booleans are
logged. Do not relax alignment or guess framing based on a failure alone; a fresh
archive must identify the actual failed invariant before a format change.

### Partial physical tail

A real authenticated format-1 download was 246660 bytes (remainder 4). Native
`decrypt_file` accepts a short final `fread` and writes its original byte count;
`DecryptCBC` rounds allocation upward and reads full AES blocks. Reproducing its
out-of-bounds partial-block read is unsafe. The safe boundary reader decrypts only
complete blocks at offset zero, then requires the declared container end to fall
entirely inside that decrypted prefix. The full checksum/table/XZ checks remain
mandatory. A physical partial tail is accepted only outside that verified region;
no ciphertext is padded, no offset/cipher is guessed, and no missing container
bytes are fabricated. Counts retain the physical ciphertext size and total tail.
The standalone `DecryptFormat1` contract remains strict block-aligned.

A live run with complete-block prefix handling passed format-1 decryption,
checksum/table/XZ, filename indexing and authenticated identity mapping. It then
failed selection; the previous generic error did not preserve the exact selection
reason. Message parsing/import are still unverified. Selection diagnostics now
separate selected-conversation absence, invalid mapping and cancellation with
file-category counts only, excluding IDs and filenames.
