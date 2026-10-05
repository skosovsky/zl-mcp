# Mobile BinNet mention scalar candidate

Status: internal offline metadata; no domain identity/offset conversion.

The installed addon calls GetValues for top-level tag 8 at 0x7244 and parses each
nested value at 0x728c, so repeated mentions must be retained in source order.
Nested fields use signed 4-byte big-endian GetIntValue:

| Tag | Native property | Getter / property evidence |
| --- | --- | --- |
| 100 | type | 0x73a4 / 0x73d4 |
| 101 | uid | 0x72f4 / 0x7324 |
| 102 | pos | 0x73f4 / 0x7424 |
| 103 | len | 0x7354 / 0x7384 |

ParseMention uses the same nonempty TLV/context limits as other nested metadata.
Fields are optional with explicit presence; reject duplicate known tags and any
width other than four. Preserve raw signed values without reinterpretation.
Unknown nested occurrences contribute to UnsupportedFields. Formatting/JSON
redact all values; Clear zeros scalar pointees. Failure returns no partial result.

ParseBinNet routes each tag-8 occurrence through this decoder and retains mention
order. Malformed mention data fails the whole BinNet, including a preceding quote.
Top-level byte/field limits bound the number of mentions; no recursion into unknown
fields. Unknown nested count contributes to the envelope coverage count.

No native evidence here establishes valid uid ranges, account-bound ID translation,
type meanings or text offset units (UTF-8 bytes, runes or UTF-16 code units).
Negative/zero values remain raw evidence rather than valid mention offsets/IDs.
Do not slice text or fabricate a resolved mention from these scalars. Real BinNet
fixtures and semantic mapping remain required before corpus import.
