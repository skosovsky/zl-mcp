# Mobile format-1 block transform candidate

Status: offline internal component, not a mobile transfer/import tool. Native
format-1 wrapper requests the decrypted archive key as hexadecimal text and
uppercases it. The AES-256 function expands the first 32 text bytes directly;
these bytes are not hex-decoded. A local IV copy starts at zero for every
65536-byte call. Ciphertext is block-aligned; no PKCS padding is removed.

The candidate accepts only even ASCII hexadecimal key text of 32–256 chars
and an explicit positive ciphertext budget up to 512 MiB. It checks cancellation
while reading and between chunks, reads at most budget+1 bytes, rejects overflow
and misalignment, transforms exact ciphertext starting at offset zero, and
requires decrypted ZDB4.0 magic. No prefix/suffix layout guessing or alternative
key/cipher fallback is allowed. Errors retain neither key nor plaintext.

The next layer must verify the complete header/checksum and compressed stream
before persistence. Magic is not authentication. This component does not remove
unknown trailers, decompress, open files, start a listener or recover messages.
Real archive compatibility and account ownership remain unverified.
