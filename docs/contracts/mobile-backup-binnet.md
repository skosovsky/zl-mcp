# Mobile BinNet metadata candidate

Status: internal bounded offline decode, not corpus conversion.

parse_bin_net compares the top-level key with 7 at 0x6f54 and branches to 0x7108;
its nested bytes are parsed by TlvBox::Parse at 0x74e8 before quote reconstruction.
Attachment tag 6 supports repeats; property 5 and reference 11 have separate
native paths and remain unsupported. Mention tag 8 supports repeated nested values
and is covered by mobile-backup-mentions.md, not equated with quote metadata.

ParseBinNet uses the existing nonempty 256-KiB / 1024-field TLV limits. Decode one
optional tag-7 value with ParseQuote. Reject repeated quote tags rather than choose
one. Decode each tag-8 occurrence as a raw mention, retaining encounter order.
Decode each tag-6 occurrence with the [attachment decoder](mobile-backup-attachment.md),
retaining order without merging. A malformed attachment fails the whole result.
Count every other field occurrence as UnsupportedFields; retain repeated
unsupported tags in encounter order as private tag metadata. Include nested
unknown quote, attachment and mention fields in the unsupported count. Never return a successful prefix
when the structural stream or selected nested value fails. Cancellation fails the whole
result. Clear owns and clears quote/attachment/mention data and resets tag metadata. Formatting
and JSON redact all metadata. There is no recursive decoding of unknown values.

This decoder provides evidence for coverage reporting, not proof of complete
message semantics. A zero unsupported count means only that encountered tags
were recognized, not that account identity, quote targets or attachment semantics
were validated. Empty BinNet has no established native compatibility and is
rejected here; callers must separately report missing metadata rather than invent
an empty complete result. Account-bound conversion, actual SQLite/BinNet fixtures,
selected silent import and encrypted archive framing remain required. No corpus
or Events writes occur.

### Empty input evidence

Read-only disassembly of installed client 26.9.10.2959: `TlvBox::Parse` at
0x9fe0 checks length at 0xa020 and branches for nonpositive length to its exception
path at 0xa2d0. `ParseBinNet` separately rejects absent/non-buffer arguments.
Thus an absent/empty BinNet is not evidence of a successfully decoded empty
metadata object. Retain the missing-metadata gap instead of silently accepting
a row. No native parser function or private archive was executed.
