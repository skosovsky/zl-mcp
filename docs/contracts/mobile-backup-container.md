# Mobile backup plaintext container candidate

Status: internal offline decoder, not a service history source. This corrects the
previous header-only interpretation. In the installed native decoder, the big-
endian uint32 at [6,10) is the declared complete container end, not the file-table
end. XXH32 seed zero at [10,14) covers **all** bytes [14,declared_end), including
file count/table and compressed payload. Native decompress_file passes
(declared_end-current_file_position) to DecompressProcessV2; the latter bounds
its compressed input by that remaining count and splits output by file sizes.

ReadContainer verifies the whole declared region within the caller's maximum
512 MiB budget before parsing the table. Table bounds remain 64 KiB/1000 files,
128-byte names and strict flat decimal/group names. It returns the remaining
compressed region. ReadPlainArchive additionally requires exact input EOF,
validates/decompresses the XZ stream under the output budget and requires the
output length to equal the sum of all file sizes. Split files in declared order;
never write archive names to filesystem paths. Result buffers are private and
must be cleared after use; JSON/default formatting reveal no contents/names.

Exact plaintext EOF is the supported offline contract. Encrypted padding/trailer
rules are not inferred from AES alignment and remain to be verified separately.
No archive ownership, SQLite schema/validity, ID mapping or message semantics are
established by successful splitting. No files, messages or Events are persisted.

The separately documented [format-1 assembly](mobile-backup-encrypted-archive.md)
now supports the observed declared-end reader boundary after block decryption,
reporting physical tail bytes without guessing/unpadding their content. The exact
plaintext EOF contract of ReadPlainArchive remains unchanged. Actual phone-archive
compatibility and producer tail convention are still unverified.
