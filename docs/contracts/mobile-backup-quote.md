# Mobile BinNet quote scalar candidate

Status: internal offline decoder; no corpus import or Events.

The installed arm64 addon parse_bin_net uses nested top-level tag 7 for quote.
GetIntValue (0xb170, load/rev at 0xb1e4) reads big-endian 32 bits;
GetInt64Value (0xb3a8, load/rev at 0xb41c) reads big-endian 64 bits.
The signed reference types and scvtf instructions establish signed native values.
Preserve exact signed bits in Go; never convert message identifiers through float.

| Nested tag | Native property | Width | Getter/property evidence |
| --- | --- | --- | --- |
| 80 | ownerId | 4 | 0x76d8 / 0x7708 |
| 81 | cliMsgId | 8 | 0x762c / 0x7658 |
| 82 | globalMsgId | 8 | 0x7774 / 0x77a0 |
| 83 | cliMsgType | 4 | 0x7810 / 0x7840 |
| 84 | ts | 8 | 0x7560 / 0x758c |
| 85 | ttl | 8 | 0x7728 / 0x7754 |

ParseQuoteScalars accepts the nested value, using the existing TLV limits and
cancellation rules. All six fields are optional and presence is explicit. Require
exact width and reject duplicate known scalar fields rather than choose one.
Unknown/byte fields are structurally checked but not interpreted. Native getters
do not establish safe short-field behavior; this decoder rejects short/long values.
Failure returns no partial scalar result. JSON/formatting redact all scalar data.

These are raw signed values, not validated session IDs, timestamps or TTL policy.
Negative/zero values are retained for later semantic validation, never promoted to
valid identities. ownerId requires verified account-bound translation; mapping
and text/attachment/quote-status semantics remain separate gates. A successful
scalar parse does not imply a valid quote or complete metadata. No native code is
executed and no private database is read.

## UTF-8 byte fields

Native GetBytesValue feeds an explicit-length N-API string creation before these
properties are set:

| Nested tag | Native property | Getter/property evidence |
| --- | --- | --- |
| 86 | msg | 0x7690 / 0x7968 |
| 87 | attach | 0x75d4 / 0x7ab8 |
| 88 | fromD | 0x7868 / 0x7b60 |
| 90 | quoteStatus | 0x77c8 / 0x7a10 |

ParseQuote accepts those four raw UTF-8 fields with explicit presence, owned byte
buffers and exact lengths (including empty strings and embedded NUL). Reject
invalid UTF-8 and duplicate known byte fields, with no partial result. Do not
interpret attach/fromD/quoteStatus as JSON, a URL, identity or execution instruction.
Unknown fields are counted as UnsupportedFields, not declared semantically decoded.
All data remains redacted from formatting/JSON; Clear zeros owned byte buffers and
scalar pointees. Existing structural budget applies to the whole nested stream.
No complete quote claim or domain conversion follows from successful decoding.
