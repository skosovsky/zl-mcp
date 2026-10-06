# Mobile BinNet attachment metadata

Status: internal decode; corpus rendering/import acceptance remains separate.

The native format-1 BinNet dispatcher sends outer tag 6 to
`ParseAttachChatMsg`. Read-only inspection of the installed macOS client's
arm64 function and its switch table establishes the following nested fields.
The inspected build is 26.9.10.2959; module SHA-256 is
`e9b5a7520384fa6db1a1aa0eb0ca5eaa1d71c42bb83f4450eaef4504ce9690d5`.
`ParseAttachChatMsg` begins at 0x5ba8; its switch table starts at 0x28686.
`GetIntValue` at 0xb170 loads/reverses a 32-bit value at 0xb1e4–0xb1ec,
confirming the big-endian scalar encoding. This is consumer evidence for that
installed version, not a public
Zalo API guarantee. No native module is executed or redistributed.

| Tag | Native property | Wire value |
| --- | --- | --- |
| 40 | type | UTF-8 bytes |
| 41 | catId | signed 32-bit integer |
| 42 | id | signed 32-bit integer |
| 43 | extInfo | UTF-8 bytes |
| 44 | childNumber | signed 32-bit integer |
| 45 | action | UTF-8 bytes |
| 46 | params | UTF-8 bytes |
| 47 | title | UTF-8 bytes |
| 48 | href | UTF-8 bytes |
| 49 | thumb | UTF-8 bytes |
| 50 | description | UTF-8 bytes |
| 66 | remains | UTF-8 bytes |
| 67 | zinstantData | UTF-8 bytes |
| 68 | zinstantMsg | UTF-8 bytes |

Use the existing bounded TLV framing and big-endian signed integer encoding.
Preserve absent fields separately from present empty strings and zero integers.
Own returned bytes; reject invalid UTF-8, wrong integer widths and repeated
known fields. Count each unknown field occurrence. Validate the complete nested
block before returning any metadata; cancellation or malformed suffix clears all
owned values. Preserve repeated outer attachments in encounter order rather than
overwrite or merge them. Repetition alone does not establish rendering semantics.

Keep private metadata out of JSON and formatting. `Clear` clears retained byte
and scalar allocations. Do not parse arbitrary params/extInfo as trusted commands,
fetch href/thumb, follow URLs, interpret IDs as peers/message IDs or infer
attachment meaning from the field names. Such actions require separate verified
semantics. The attachment id is a signed 32-bit attachment property, not a Zalo
message identity.

Decode does not establish supported content. The converter supports a separately
verified visible-text projection for exactly one type-0 `rtf` attachment, under
the [message conversion contract](mobile-backup-message-conversion.md).
Other actions, nontext types and repeated attachments remain unsupported.
The native text path recognizes
`action=rtf`, uses attachment title with MsgContent fallback, and passes an
attachment object to normal message processing; reading only MsgContent would
lose data. The supported projection retains title-or-MsgContent and an `rtf`
marker; it does not preserve styling or interpret attachment params.
Quotes, mentions, recall targets, expiry, WAL source eligibility and
atomic history persistence retain their separate contracts. No Events, phone
requests or corpus writes occur in this parser.

## Synthetic acceptance: 2026-10-06

AAA tests cover every known field, signed/zero/asymmetric-endian scalar values,
present-empty versus absent strings, owned bytes, unknown occurrences, repeated
outer attachments, JSON/formatting redaction and clearing. Whole-result failures
cover invalid UTF-8, integer widths, repeated known nested fields, malformed
suffixes, cancellation and an invalid quote after a valid attachment. Integration
regression originally proved that decoded rich-text metadata counted as unsupported
content rather than importing MsgContent as a substitute. The later verified
projection supersedes that gap for one `rtf` attachment: native-framed tests
check a differing visible title, empty/absent fallback, unsupported actions and
repetition, invalid UTF-8, oversized title, no source mutation and the `rtf` marker.
Root and nested-module
race/vet and CGO-free macOS arm64/Linux amd64 builds passed. A five-second bounded
Go fuzz run completed 50,661 inputs without a failure; no model eval was run.
This is offline compatibility evidence, not a new live archive/content acceptance.
