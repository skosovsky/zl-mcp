# SDK-01 — конкретное dependency exception на согласование

Статус: proposal, **не применён**. T09 принят коммитом
`bb3eadc785366588b88bdc83c4d7693c89b96090`. SDK-01/ID01–ID07 ещё не приняты.

## Предлагаемое изменение scope

Сохранить require `github.com/modelcontextprotocol/go-sdk v1.8.0` и добавить только
`replace github.com/modelcontextprotocol/go-sdk => ./third_party/go-sdk`.
В новый `third_party/go-sdk` перенести полный официальный module v1.8.0 (221 файла
в итоговой копии, около 2.6 MiB), сохранив upstream tests/LICENSE, и применить
зафиксированный patch. Существующий `third_party/zcago` и соседние библиотеки не менять.
Никаких fork publication, floating branches, services или model/live вызовов.
Итоговый SDK tree и patch будут зафиксированы в единственном SDK-01 commit после gates.

## Provenance и воспроизводимость

- Official tag v1.8.0: `3f3b699b2b67e1ed033a63d6651671dab53c2d32`.
- Proposed upstream PR #1375 head: `9174a3ffc71cf5c7617a6c64afca0802b855f770`, open/not merged.
- Дополнительные hardening изменения локальны только в изолированной proposal-копии;
  upstream PR/main/fork не редактировались.
- Patch: `sdk-v1.8.0-bounded-id.patch`, SHA256 `8d5f6fbb8728f319ed8aeb0a8fe020a79cd80cf8f7b7099afa51231d837b67b9`.
- Full tree digest: `8a36ff34a6f9c9115a95a662dd203f4e1e51380f8399b508032346d38b6c6800`.
- `sdk-proposal-manifest-v3.json`: hashes всех 221 файлов, точный diff scope.
- `proposal-reconstruction-v3.json`: official clean copy + archived patch дают
  те же hashes каждого файла. go.mod/go.sum самого SDK не изменены.

## Содержание patch

- `internal/json/json.go`
- `internal/jsonrpc2/messages.go`
- `internal/jsonrpc2/sdk01_hardening_test.go`
- `internal/jsonrpc2/wire_test.go`
- `jsonrpc/jsonrpc.go`
- `mcp/id_precision_test.go`
- `mcp/protocol.go`
- `mcp/review_B_cancellation_test.go`
- `mcp/sdk01_framing_test.go`
- `mcp/streamable.go`
- `mcp/transport.go`

Production changes: exact numeric decoding; cancellation RequestID json.Number при
неизменном metadata decoding; backport missing UnmarshalUseNumber с исходными depth/
case-sensitive правилами; lexeme cap 1024 bytes до exact arithmetic; finite integral
float64/int64 guard; фиксированный invalid numeric request error (-32600/id:null,
HTTP400 application/json либо STDIO), без raw ID; итеративное STDIO recovery;
синхронный cancellation lookup вместо goroutine, которая может захватить новую invocation.
Остальные изменения — API docs и regression/adversarial tests. Новый transport/parser
не создаётся: изменены существующие SDK codec/transport paths.

## Числовой и persistence контракт

Signed int64 min/max и соседние значения >2^53 сохраняются точно; decimal/exponent
с целым математическим значением эквивалентны integer. String IDs сохраняют тип/значение
и отличаются от numeric IDs. Fractional/out-of-int64/excessively long numeric IDs
отклоняются без application dispatch. Нулевое число с огромным exponent допустимо
в пределах lexeme cap: вычисление не строит огромный bigint. Уже округлённый
caller-provided float64 восстановить нельзя.

Неприемлемые numeric response IDs остаются codec error; отдельно не заявляем,
что SDK decoder принимает null-ID protocol error responses (существующее ограничение).
Проверка error framing использует raw JSON и не скрывает это поведение через float64.

Domain request UUID, ledger, unknown/terminal states, authority/permissions и native
owners не меняются; потеря ответа/отмена не разрешает resend.

## Licensing и upstream retention

`upstream-LICENSE` — побайтовая копия LICENSE официального v1.8.0; файл описывает
переход MIT→Apache-2.0 и сохранение применимых лицензий прежних contributions.
`upstream-README.md` и весь исходный module сохранены; не заявляем relicensing.
Новые Go tests имеют требуемые upstream copyright/license headers.
Изменённые исходники сохраняют первоначальные headers. Лицензия не удаляется и
не заменяется одной общей маркировкой MIT. `TestCopyrightHeaders` сохраняется.

## Проверки proposal и будущие acceptance gates

Before-fix codec: 16/28 PASS, 12 FAIL на официальном v1.8.0.
Hardened codec: 28/28 PASS; independent HTTP/IO error framing и bounds smoke PASS.
Adversarial cancellation: 20 race runs PASS (neighbor active, old response blocked,
new invocation reuses ID; old completion does not cancel/delete new invocation).
Full nested SDK test/race/vet/build results фиксируются в proposal-checks-v3.json.
Исторические compile/header FAIL logs сохранены и не выдаются за текущий PASS.

После разрешения exception необходимы root wrappers changes и **все ID01–ID07**:
real raw HTTP concurrency, real STDIO subprocess, cancellation/disconnect lifecycle,
bounded invalid inputs, discover/resources/Events/protocol errors, synthetic lost
write response с durable UUID и нулём повторных native dispatch. Затем root tests/race/vet,
CGO-free builds, полные offline eval/parity, повтор affected performance по текущим
согласованным абсолютным latency/object/byte budgets и два task reviewers.
Согласование dependency exception не означает принятия SDK-01.

## Условия удаления и rollback

При выпуске unmodified upstream release с исправлением заменить scoped replace на
immutable official pin только после PASS того же ID01–ID07, cancellation/bounds/error
framing, root/nested/offline/parity/CGO-free/performance набора. Зафиксировать origin,
checksums и removal evidence; не удалять hardening молча ради номера релиза.

До разрешения production dependency не меняется. Сервис не развёрнут, поэтому
runtime rollback/LaunchAgent restart не нужен. После внедрения нельзя считать возврат
к известному defective SDK успешной ID-приёмкой; rollback старого сервиса — отдельное
поручение с явным сохранением known limitation и durable state. Не делать reset/clean,
не удалять пользовательские изменения или исторические evidence.

Model/live gates: **NOT_RUN**.
