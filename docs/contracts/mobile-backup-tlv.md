# Mobile BinNet TLV structure candidate

Status: internal offline structural decoder, not complete message metadata parsing.
Installed arm64 parse_bin_net calls tlv::TlvBox::Parse; the latter reads a 4-byte
big-endian tag and 4-byte big-endian length, followed by value bytes. Native
GetValues supports repeated tags; structural decoding must retain their order
rather than silently overwrite values in a map. No unrelated public TLV library
is treated as proof of Zalo compatibility or copied into the repository.

Accept a nonempty stream at most 256 KiB and at most 1024 fields. Check every
8-byte prefix and complete declared value against the remaining input before
copying. Preserve unsigned tag bits and unknown fields without interpreting them.
Zero-size fields are structurally retained, but their native semantic support is
not established. Nested values are opaque; no recursive semantic guessing occurs.
Check context cancellation before/after parsing and between entries. Failures
return no successful prefix and clear owned value buffers; formatting/JSON reveal
neither tags nor content. Callers clear a successful owned result after use.

Static semantic leads: native top-level tag 6 feeds repeated ParseAttachChatMsg,
tag 7 feeds quote reconstruction, tag 5 feeds property reconstruction, tag 8 feeds
mentions and tag 11 feeds reference. These are research leads, not a verified
semantic contract. Exact nested scalar IDs/types, quote/mention mapping, attachment
formats and real BinNet fixture compatibility remain gates before message import.
A structurally valid TLV stream does not authenticate content or imply that all
message semantics were recovered. No SQLite, corpus, Events or network writes occur.

The confirmed scalar subset of nested quote tag 7 now has its own contract in
[mobile-backup-quote.md](mobile-backup-quote.md). This narrows the remaining scalar
research gate but does not establish complete quote/attachment semantics or real
archive compatibility.
