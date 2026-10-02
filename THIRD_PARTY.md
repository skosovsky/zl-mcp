# Third-party code

The root [MIT license](LICENSE) applies to this project's original code. Dependencies retain their own licenses; the root license does not replace them. Module versions are recorded in `go.mod` and `go.sum`.

## Included zcago source

`third_party/zcago` contains the runtime source of [amrakk/zcago](https://github.com/amrakk/zcago) at commit `d4ff65b460577b2557e70220b68d08ce1f7431b4` (`v0.3.1-0.20260908044439-d4ff65b46057`), with local patches. Upstream CLI and examples are excluded. Its [MIT license and copyright notice](third_party/zcago/LICENSE), Copyright (c) 2025 Amrakk, RFS-ADRENO, are retained.

The local Go module replacement makes these patches part of a checkout build. They fix decoding quoted message IDs and timestamps, preserve authentication failures, and prevent a nil-response panic during login cancellation. See [PATCHES.md](third_party/zcago/PATCHES.md) for implementation details and regression coverage.

Upstream tracking:

- [#2: Decimal-string quote IDs and timestamps](https://github.com/amrakk/zcago/issues/2).
- [#3: WebSocket handshake HTTP status](https://github.com/amrakk/zcago/issues/3).
- [#4: Login response error codes](https://github.com/amrakk/zcago/issues/4).
- [#5: Server-info wire response envelope](https://github.com/amrakk/zcago/issues/5).
- [#6: HTTP 401 during login and server-info](https://github.com/amrakk/zcago/issues/6).
- [#7: Nil-response panic on request failure](https://github.com/amrakk/zcago/issues/7).

Remove a local patch only after a pinned upstream version provides equivalent behavior and its regression tests pass. Updating the upstream pin alone does not remove the local replacement.

This is an unofficial integration using a third-party Zalo client. It is not affiliated with or endorsed by Zalo.
