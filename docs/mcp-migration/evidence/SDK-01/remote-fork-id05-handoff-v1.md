# SDK-01 — remote fork ID05 blocker и готовый correction

Поручение пользователя выполнено в working tree:
- toolsy обновлён с v0.20.2 до официального **v0.20.3** (public consumer probe PASS).
- `replace github.com/modelcontextprotocol/go-sdk => github.com/skosovsky/go-sdk v1.8.1-0.20261012144231-5ec4c4212548`.
- Root require original module v1.8.0 сохранён; zcago replace не изменён.
- go.sum/probe/dependency matrix синхронизированы. Local SDK vendor не создан.

## Что исправил выбранный pin

Actual public codec: **28/28 PASS** — signed int64 границы и соседние >2^53,
string/numeric distinction, integral decimal/exponent, invalid numeric rejection.
В fork используется bounded decimal parser (numeric lexeme 128 bytes), без big.Rat.
Cancellation lookup синхронный. Это ещё не full ID01/ID04 task acceptance.

## Почему SDK-01 пока нельзя принять

`authorized-fork-framing-v2.json`: **6 FAIL**, cleanup true.
Пять invalid numeric HTTP requests (fractional, int64 overflow/underflow, extreme exponent,
129-byte token) дают HTTP400 **text/plain**, без JSON-RPC error `id:null`.
Actual IOTransport invalid ID 1.5 возвращает parse error и **0 response bytes**.
Это нарушает §4.4 ТЗ: «Для запроса, ID которого невозможно принять, вернуть protocol error
с id:null». Разрешение remote replace не изменяет этот публичный контракт.

Два независимых reports подтвердили blocker:
`authorized-fork-review-A-v1.json`, `authorized-fork-review-B-v2.json`.
Ранний diagnostic timeout/panic из inherited harness сохранён; clean v2 repro не паникует,
останавливает read goroutine и выдаёт durable FAIL. Не заменять этот FAIL на PASS.

Privacy clarification: SDK ensureLogger(nil) использует DiscardHandler, а не global
INFO logger. Root не настраивает SDK Logger. DEBUG logging с явно injected SDK logger
может включать reason/ID, но root leak не доказан; прежняя неверная формулировка B
исправлена, исходный report сохранён отдельно. После SDK adoption обязателен root privacy test.

## Проверенный минимальный correction

`fork-5ec4c421-null-id-framing.patch`, **5406 bytes**.
Base commit `5ec4c42125489a54f8324e2b81396847befcd2c3`.
Patch SHA256 `bee2dda9bf755459f6dc9c64ec4f23650bbb09613ce9ca72fab779db39daeb43`.
Corrected proposal tree `36992e168844dbdac502f10cd7884b841f0496655a845fe384682a1f41a0379a`.
Полные file hashes — `fork-framing-proposal-manifest-v1.json` (225 files).

Три existing production files + один regression test:
- `internal/jsonrpc2/messages.go`
- `mcp/sdk01_framing_test.go`
- `mcp/streamable.go`
- `mcp/transport.go`

Correction классифицирует rejected numeric request в существующем SDK decoder,
кодирует null ID для error response, отправляет HTTP400 application/json / STDIO
JSON-RPC -32600 и продолжает STDIO read loop итеративно. Invalid response остаётся
codec error. Новый parser/transport не создаётся; numeric128 policy, cancellation,
float64 MakeID legacy coercion, модули, лицензии и upstream tests не меняются.

`fork-framing-proposal-repro-v1.json`: те же 6 cases теперь PASS, cleanup true.
`fork-framing-proposal-checks-v1.json`: full SDK test/race/vet/build PASS на exact tree.
Независимые correction readiness reports `fork-framing-proposal-review-A-v1.json`
и `fork-framing-proposal-review-B-v1.json` относятся только к proposal, не к SDK-01 task.

## Необходимое решение

Предпочтительно применить сохранённый correction в fork и дать новый immutable
pseudo-version/commit pin. Агент не редактирует/публикует sibling fork без отдельного
поручения. Local patch поверх выбранного remote pin или vendor также не разрешены
текущим ответом пользователя. До решения root остаётся на точно порученном pin,
SDK-01 gate FAIL; коммит SDK-01 и T10 не создаются.

После совместимого pin: lossless discover/Events wrappers, полный ID01–ID07 с actual
STDIO subprocess/concurrency/cancellation/durable write identity, root/nested checks,
CGO-free builds, full offline/parity, affected performance по согласованным v3 budgets,
два independent final task reviewers 100%/PASS, sole SDK-01 commit.

T01–T09 приняты; sole T09 commit `bb3eadc785366588b88bdc83c4d7693c89b96090`.
Working changes/evidence сохранять; reset/clean не выполнять. Runtime не развёрнут,
LaunchAgent не перезапускался. Model/live gates: **NOT_RUN**.
