# Замечания golangci-lint — разбор и предлагаемые решения

Актуализировано после упрощения context-тестов: 2026-10-13. golangci-lint 2.14.0, root toolchain Go 1.27.2, конфиг `.golangci.yml`.

Всего **2529** актуальных замечаний: root — **2314**, `tools/harness` — **215**. Root lint успешно загрузил пакеты; compiler/loader blocker отсутствует. Lint остаётся FAIL из-за перечисленных diagnostics. Несколько правил на одной строке учитываются отдельно.

По сравнению со снимком до восстановления компиляции не воспроизвелись **18** записей (включая агрегированный compile blocker), новых записей — **2**. ID совпавших замечаний сохранены. Исчезнувшие записи вынесены в конец документа и не входят в актуальную сводку.

## Реализация L01–L06

Исходное состояние сохранено перед исправлениями: `/tmp/zl-lint-initial.patch`, `/tmp/zl-lint-initial-status.txt`, `/tmp/zl-lint-initial-sha256.json` (376 файлов). Свежие полные diagnostics без autofix: root 2314, tools 215, всего 2529; оба loader успешны при доступе к Go caches. Первый sandbox-only запуск завершился loader error и не учитывается как приёмка.

Принятые решения: только локальные объяснённые nolint; lint обязателен в check/CI; candidate performance pin Go 1.27.2, historical baseline Go 1.27.1 и числовые бюджеты сохраняются.

| Шаг | Статус | Evidence |
|---|---|---|
| L01 | Architecture PASS: host/all-build 100%, 14 isolated + 6 source controls. Tools tests/race/vet PASS. | `/tmp/zl-lint-L01-checks.log` |
| L02–L06 | OPEN | — |

Lint IDs пока OPEN: L01 закрывает архитектурный blocker, а не diagnostics стилевых правил. Shared performance worker уже имеет cleanup в TestMain; сохраняем его и проверяем build-failure путь, не создаём второй lifetime.

## Исправленная проблема компиляции

Autofix правила `fatcontext` ошибочно заменил присваивание `=` на short declaration `:=`:
- В двух SDK-тестах запись `invocations[v.message] := v.ctx` недопустима: map index не может быть объявляемым именем.
- В `TestTraceCacheBoundAndExpiry` выражение `remembered := ctx` внутри callback создало неиспользуемую локальную переменную вместо сохранения контекста во внешнюю переменную.

SDK-тесты теперь хранят целые `sdkInvocation` и проверяют их `ctx`. Cache-тест заполняет кеш внутри callback, владеющего trace context; внешняя переменная `remembered` удалена. Все три `nolint:fatcontext` удалены. Полный root lint не сообщает fatcontext/typecheck ошибок. Изолированная проверка переписанных конструкций с `--enable-only fatcontext --fix` без исключений: 0 issues.

Компиляция всех root packages/tests проверена командой `go test ./... -run '^$'`: PASS. `go test -race ./internal/mcpserver` и `go test -race ./internal/observability`: PASS.

Дополнительная race-проверка выявила регрессию `errorlint` autofix в `errorOutcome`: замена точного sentinel comparison на `errors.Is` вызывала пользовательский `Is` callback и нарушала no-callback telemetry contract. Исходное сравнение восстановлено с обоснованным локальным `nolint:errorlint`. Строгие passthrough identity assertions в observer tests также восстановлены. Повторный race run — PASS.

## Проверка повторного autofix (2026-10-13)

Компиляция всех root packages/tests, полный `go test ./...` и race checks mcpserver/observability — PASS. Переписанные context-тесты и точные errorlint comparisons сохранились. Повторение прежних compile/no-callback regressions не обнаружено.

`tools/harness` tests — FAIL в `TestConfigDecisionEquivalence`; отдельный действующий architecture gate также FAIL. Автоматические модернизации добавили стандартные импорты вне текущих allowlists:

| Пакет | Новый stdlib import |
|---|---|
| `cmd/zl-mcp-eval-fixture` | `errors` |
| `internal/collector` | `maps` |
| `internal/control` | `errors` |
| `internal/local` | `errors` |
| `internal/mcpserver` | `maps` |
| `internal/mobilebackup` | `slices` |

Это отдельный FAIL архитектурных проверок; он не скрыт успешными compilation/product tests. Исправление должно согласовать точные stdlib allowlists и убрать привязку изменяемого production graph к immutable old config из equivalence test, сохраняя проверки fixture decisions и полного актуального graph. Code/config пока не менялись по этому finding. Полный race для root и model/live при этой проверке не запускались.

## Разбор и порядок исправлений (2026-10-13)

Изучены группы замечаний из сохранённого snapshot и код в местах с риском изменения поведения: конверсии, процессы, cleanup, SQL, fingerprints, redaction, telemetry. Для всех 43 групп ниже указан способ исправления и границы исключений. Это анализ и предложения: новый lint run не выполнялся, 2529 — число в последнем сохранённом lint snapshot, не подтверждённый остаток после последнего пользовательского autofix. Отдельная построчная верификация всех 2529 сообщений не заявляется. ID и исходные diagnostics сохранены; исправлен ошибочно удвоенный префикс tools/harness в путях.

1. Восстановить architecture gate: разрешить ровно шесть описанных выше stdlib edges. В TestConfigDecisionEquivalence оставить old/new config на фиксированных fixtures; текущий полный AST graph проверять только актуальным config. Не расширять разрешения на внутренние слои, не менять исторический config. Проверить mutation mcpserver → storage, hidden imports и thresholds 100%.
2. Закрыть смысловые замечания: errors/cleanup, shadow result/ctx, SQL lifetime, обязательные DTO поля. Сначала сохранить точные wire/persistence bytes и тестовые assertions; повторные effects не удалять как дубликаты.
3. Добавить доказанные локальные исключения для conversions, redaction method sets, optional values, watchdog и protocol compatibility. После каждой группы проверить, что директива реально подавляет только нужное замечание и выдерживает повторный targeted autofix.
4. Выполнить механические rename/doc/format/unused правки. Complexity рефакторить по предметным границам; параметры линтера не ослаблять автоматически. Изменение настроек, предложенное как альтернатива ниже, пока не принято и не применено.
5. Повторить полный lint обоих модулей без ограничений diagnostics, обновить этот файл с прежними ID. Затем root/tools tests/race/vet, architecture, offline и platform builds. Если меняется runtime path — performance с прежними пределами. Model/live остаются NOT_RUN. До этих проверок lint/acceptance не объявлять PASS.

Общий формат исключения: `//nolint:<конкретный линтер> // <проверяемая причина и граница применимости>.` Для gosec дополнительно указывать G115/G202/etc в причине: `nolint:gosec` технически подавляет весь gosec в своём scope, поэтому ставить на минимальную строку, а не на весь parser. Не использовать причины «safe», «false positive» без доказательства. Не добавлять новые тесты ради комментария nolint; использовать имеющиеся boundary/contract tests, а недостающие проверки добавлять для реального риска.

## Адресные решения

### Числовые конверсии G115

Предпочтение пользователя: оставить простую конверсию с локальным nolint, когда безопасность следует из контракта. Общий safe-cast framework здесь не нужен. Ниже разобраны все G115 из snapshot.

| ID | Предлагаемое решение и причина для комментария |
|---|---|
| L1395, L1416, L1418 | `nolint:gosec`: на поддерживаемых Unix targets os.Getuid возвращает UID из того же 32-битного uid_t, что Stat_t.Uid; это не пользовательское число. Сохранить owner check. При добавлении иной платформы пересмотреть предпосылку. |
| L1428, L1433, L1434, L1435 | `nolint:gosec`: wire field — signed int32/int64 в two’s complement; Uint32/Uint64 читает биты, cast намеренно восстанавливает знак. Проверка верхней границы MaxInt здесь сломала бы допустимые отрицательные значения. |
| L1429 | `nolint:gosec`: checksum32 использует арифметику modulo 2^32, усечение длины — часть алгоритма. |
| L1430 | `nolint:gosec`: Fetch проверяет expected ≤ limit ≤ MaxTotalBytes (512 MiB), поэтому int64(expected)+1 не переполняется. readDownload private; причина должна ссылаться на проверку caller, все его callers должны сохранять её. |
| L1431 | `nolint:gosec`: перед cast проверено end ≤ uint64(len(plain)), следовательно end представим как int. |
| L1432 | `nolint:gosec`: length ≤ limit ≤ MaxTotalBytes=512 MiB, помещается даже в 32-битный int; нижняя граница 18 защищает вычитание prefix. |
| L1441 | `nolint:gosec`: len(header) ≤ retainedMetadataLimit=1 MiB до PutUint32. |
| L1456 | `nolint:gosec`: len(metadata) ≤ snapshotMetadataLimit=4096 до PutUint32. |
| L1467 | `nolint:gosec`: indexLen ≤ uint64(end-24), end — неотрицательный int ≥32; cast ограничен длиной входа. |
| L1468, L1469 | `nolint:gosec`: count ≤ maxXZBlocks-blocks, maxXZBlocks=1000; invariant blocks ≤1000 сохраняется после каждого прибавления. |
| L1470, L1471, L1472 | `nolint:gosec`: indexStart=end-12-indexLen ≥12 после проверки indexLen ≤end-24; перевод в uint64 сохраняет значение. |
| L1473 | Предпочтительно сделать локальное доказательство: вычислить padded size в uint64, проверить его ≤ uint64(indexStart-pos) после проверки pos ≤ indexStart, затем cast с nolint. Текущий caller ограничивает весь input 512 MiB, и есть aggregate blockBytes bound, но проверка до cast проще поддерживается и не зависит от знания caller. Не объявлять доказанным runtime overflow на текущих supported paths. |
| L1474, L1475 | `nolint:gosec`: headerLen=(byte+1)*4 лежит в [4,1024], checkBytes выбран из {0,4,8,32}; сумма неотрицательна и мала. Это private helper inspectXZ; сохранить ограничение checkBytes при новых callers. |
| L1476 | Добавить в вычисление within_archive_budget проверку MaxArchiveBytes >0 перед uint64, затем nolint с причиной non-negative bound. Сейчас лимит валидируется при создании request, но читается из persistence и локальной проверки нет. Отрицательный повреждённый лимит не должен стать большим unsigned и дать true. Нормальные ответы не меняются; добавить synthetic invalid-persisted-value case, если такой путь достижим в fixture. |

Примеры комментариев для реализации (ещё не добавлены в Go):

```go
//nolint:gosec // G115: metadata length is checked against snapshotMetadataLimit (4096) above.
binary.BigEndian.PutUint32(plain, uint32(len(metadata)))
//nolint:gosec // G115: decode the signed two's-complement int32 wire value without changing its bits.
v := int32(binary.BigEndian.Uint32(f.Value))
```

### Остальные места, где важен контракт

| ID / место | Предлагаемое решение |
|---|---|
| L1396, L1417 | `nolint:gosec` для Chmod(directory,0700): owner execute нужен для обхода каталога; доступ группы/остальных запрещён. Не ставить 0600 на directory. |
| L1479, L1504–L1507, L1513 | SQL из compile-time fragments и validated enum/order можно оставить с локальным nolint:gosec (G202), перечислив источник fragments. Проверить каждый caller predicate; клиентские значения должны оставаться bound arguments. При появлении raw user fragment исправлять код, не подавлять. |
| L1514 | Cookie копируется с полученной scope metadata для persistence jar. `nolint:gosec` (G124): preserve received cookie attributes; это не выдача cookie сервером. Не выставлять Secure/SameSite/HttpOnly без изменения upstream semantics. |
| L1515, L1527 | Harness запускает выбранный локальный executable argv без shell; допустим точечный nolint:gosec (G702/G204) с этой причиной после проверки trusted CLI/internal callers. Не интерполировать args в shell string. |
| L1516 | root для writeModule — synthetic workspace, принадлежащий harness. Проверить все callers и происхождение относительных File paths; nolint:gosec (G703) только при этом invariant. Если функция начнёт принимать внешние manifests, нужен запрет absolute/.. escape перед записью. |
| L1519–L1525 | Readable synthetic evidence допускает 0644/0755 с локальным nolint:gosec и причиной отсутствия secrets. Каталоги, содержащие executable, должны оставаться traversable. Файлы с окружением/raw output отдельно проверить на privacy; секретные файлы переводить в 0600, не объявлять весь harness безопасным. |
| L1743 | Header+body append намеренный; исключение makezero с причиной формата, а не смена длины prefix. |
| L2139, L2169 | При добавлении json tags сохранить существующие capitalized keys и отсутствие omitempty. Нужны точные bytes/hash для fingerprint и read identity, иначе persistence/idempotency regressions. |
| L2303–L2323 | Для типов с value String/GoString и pointer Clear оставить смешанные receivers с nolint:recvcheck. Redaction должна работать и для значения, и для указателя. У остальных типов исключение не автоматически наследуется. |
| L2485–L2490 | Deferred closure заканчивает span после установки outcome и повторно выбрасывает panic. Локальный nolint:spancheck предпочтительнее второго End или нарушения порядка. |
| L2513 | Два вызова Remove — намеренный тест повторного удаления. Разнести в два if с отдельным сообщением «first removal»/«idempotent removal»; assertion не удалять. |
| L2517, L2518 | Локальный nolint:staticcheck с указанием SA1019 и protocol-required RSA PKCS#1 v1.5, synthetic fixture mirrors protocol. Замена на OAEP несовместима с входящим форматом. Исключение не означает криптографическую безопасность: отдельно проверить доступность decrypt oracle и отсутствие выдачи причин расшифровки недоверенной стороне; не логировать ciphertext/key/plaintext. Формат протокола в lint cleanup не менять. |
| L2538 + L0599–L0601 | Общий worker binary создаётся sync.Once в MkdirTemp без видимого cleanup. Нужен общий владелец TestMain с удалением каталога после suite либо отказ от общего кеша. Замена на t.TempDir первого test оставит последующие tests с удалённым executable. |

## Как дать указания

Заполни «Решение пользователя» под нужным правилом. Для исключений внутри группы укажи ID замечания, например `L0001`. Можно указать: исправить код; оставить конструкцию с обоснованным локальным исключением; изменить правило или его настройки. Пустое решение означает, что индивидуальных указаний пока нет. Ниже под каждым правилом добавлено «Предлагаемое решение Ann»; это план исправления, а не отметка о его выполнении. Разрешение пользователя на обоснованные nolint, особенно безопасные конверсии, учтено.

## Состояние рабочего дерева

- Добавлен `make lint` для root и отдельного Go module `tools/harness`.
- Штатные root autofix ранее применены; при текущем rerun новых autofix не применялось.
- К `tools/harness` autofix не применялся.
- Три context-теста упрощены без fatcontext exceptions; errorlint exceptions, защищающие no-callback и точную identity ошибок, сохранены. Остальные исправления и коммит отложены до решений в этом файле.
- Остальные автоправки требуют semantic review; полный test/race/offline/performance run после них не выполнен.

## Воспроизведение полного списка

```sh
GOWORK=off golangci-lint run --config .golangci.yml --timeout 10m --max-issues-per-linter 0 --max-same-issues 0 --uniq-by-line=false ./...
cd tools/harness
GOWORK=off golangci-lint run --config ../../.golangci.yml --timeout 10m --max-issues-per-linter 0 --max-same-issues 0 --uniq-by-line=false ./...
```

## Сводка

| Правило | Root | tools/harness | Всего |
|---|---:|---:|---:|
| `cyclop` | 73 | 3 | 76 |
| `embeddedstructfieldcheck` | 0 | 1 | 1 |
| `errcheck` | 115 | 0 | 115 |
| `errname` | 5 | 1 | 6 |
| `exhaustive` | 6 | 0 | 6 |
| `exhaustruct_v5` | 303 | 4 | 307 |
| `forbidigo` | 1 | 3 | 4 |
| `funcorder` | 4 | 0 | 4 |
| `funlen` | 62 | 5 | 67 |
| `gochecknoglobals` | 6 | 6 | 12 |
| `gocognit` | 196 | 9 | 205 |
| `goconst` | 413 | 66 | 479 |
| `gocritic` | 9 | 0 | 9 |
| `gocyclo` | 69 | 3 | 72 |
| `godoclint` | 7 | 1 | 8 |
| `golines` | 0 | 7 | 7 |
| `gosec` | 132 | 19 | 151 |
| `govet` | 191 | 5 | 196 |
| `ineffassign` | 1 | 0 | 1 |
| `makezero` | 1 | 0 | 1 |
| `mirror` | 1 | 0 | 1 |
| `mnd` | 346 | 42 | 388 |
| `modernize` | 0 | 5 | 5 |
| `musttag` | 33 | 0 | 33 |
| `nestif` | 36 | 3 | 39 |
| `nilerr` | 13 | 0 | 13 |
| `nilnil` | 4 | 0 | 4 |
| `noctx` | 5 | 1 | 6 |
| `nonamedreturns` | 50 | 1 | 51 |
| `perfsprint` | 0 | 11 | 11 |
| `predeclared` | 4 | 0 | 4 |
| `reassign` | 4 | 0 | 4 |
| `recvcheck` | 21 | 0 | 21 |
| `revive` | 81 | 17 | 98 |
| `sloglint` | 63 | 0 | 63 |
| `spancheck` | 6 | 0 | 6 |
| `sqlclosecheck` | 14 | 0 | 14 |
| `staticcheck` | 14 | 1 | 15 |
| `unparam` | 1 | 0 | 1 |
| `unqueryvet` | 2 | 0 | 2 |
| `unused` | 4 | 0 | 4 |
| `usetesting` | 13 | 1 | 14 |
| `wastedassign` | 5 | 0 | 5 |

## Полный актуальный список

### cyclop (76)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Разбирать вместе с gocyclo/gocognit/funlen/nestif: одно изменение функции закрывает несколько diagnostics. В production выделять самостоятельные этапы с контрактами (CLI parsing, запуск, сбор результата; worker claim/reserve/commit; parser framing/records), сохраняя порядок effects и lifetime. Для линейного protocol switch и цельного AAA integration test допустим локальный nolint с объяснением, почему разделение скроет сценарий. Package-average не лечить добавлением пустых helpers; после рефакторинга оценить отдельную настройку средней сложности для тестовых пакетов.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0001 | `cmd/zl-mcp-eval/main.go:16:1` | calculated cyclomatic complexity for function run is 38, max is 30 |
| L0002 | `cmd/zl-mcp-eval/main_test.go:1:1` | the average complexity for the package main is 15.600000, max is 10.000000 |
| L0003 | `internal/collector/run.go:55:1` | calculated cyclomatic complexity for function runSession is 40, max is 30 |
| L0004 | `internal/config/config.go:47:1` | calculated cyclomatic complexity for function Load is 35, max is 30 |
| L0005 | `internal/domain/history.go:59:1` | calculated cyclomatic complexity for function NewHistoryPageFailure is 46, max is 30 |
| L0006 | `internal/evalfixture/fixture.go:133:1` | calculated cyclomatic complexity for function New is 47, max is 30 |
| L0007 | `internal/evaluation/checkpoints.go:149:1` | calculated cyclomatic complexity for function GradeCheckpoint is 112, max is 30 |
| L0008 | `internal/evaluation/grader.go:19:1` | calculated cyclomatic complexity for function Grade is 71, max is 30 |
| L0009 | `internal/evaluation/process_test.go:1:1` | the average complexity for the package evaluation is 11.161290, max is 10.000000 |
| L0010 | `internal/evaluation/runner.go:135:1` | calculated cyclomatic complexity for function Run is 61, max is 30 |
| L0011 | `internal/evaluation/scenarios.go:22:1` | calculated cyclomatic complexity for function RunCase is 71, max is 30 |
| L0012 | `internal/events/conversation_test.go:20:1` | calculated cyclomatic complexity for function TestConversationScopesSignedDeliveryAndCancellation is 36, max is 30 |
| L0013 | `internal/events/subscriptions.go:132:1` | calculated cyclomatic complexity for function Call is 37, max is 30 |
| L0014 | `internal/historyimport/diagnostics_test.go:1:1` | the average complexity for the package historyimport is 15.909091, max is 10.000000 |
| L0015 | `internal/historyimport/mcp_test.go:27:1` | calculated cyclomatic complexity for function TestMCPHistoryImportDiscoveryStatusAndReads is 34, max is 30 |
| L0016 | `internal/historyimport/mobile_worker.go:101:1` | calculated cyclomatic complexity for function runMobileOperationObserved is 59, max is 30 |
| L0017 | `internal/historyimport/worker.go:68:1` | calculated cyclomatic complexity for function runOperationObserved is 40, max is 30 |
| L0018 | `internal/mcpserver/conversation_events_http_test.go:34:1` | calculated cyclomatic complexity for function testConversationProfileTLS is 47, max is 30 |
| L0019 | `internal/mcpserver/conversations.go:17:1` | calculated cyclomatic complexity for function addConversationResources is 39, max is 30 |
| L0020 | `internal/mcpserver/conversations_test.go:17:1` | calculated cyclomatic complexity for function TestConversationMCPSearchContextCatalogueAndFullText is 43, max is 30 |
| L0021 | `internal/mcpserver/http.go:110:1` | calculated cyclomatic complexity for function ServeHTTP is 41, max is 30 |
| L0022 | `internal/mcpserver/migration_baseline_test.go:56:1` | calculated cyclomatic complexity for function TestMigrationCatalogBaseline is 34, max is 30 |
| L0023 | `internal/mcpserver/pagination_test.go:18:1` | calculated cyclomatic complexity for function TestLargeWirePagesKeepAllResultsAndSearchSnapshot is 32, max is 30 |
| L0024 | `internal/mcpserver/server.go:61:1` | calculated cyclomatic complexity for function New is 56, max is 30 |
| L0025 | `internal/mcpserver/server.go:239:1` | calculated cyclomatic complexity for function call is 41, max is 30 |
| L0026 | `internal/messaging/manager.go:38:1` | calculated cyclomatic complexity for function Call is 32, max is 30 |
| L0027 | `internal/messaging/subprocess_restart_test.go:1:1` | the average complexity for the package messaging is 12.000000, max is 10.000000 |
| L0028 | `internal/mobilebackup/control_scan.go:30:1` | calculated cyclomatic complexity for function ReadOwnDirectRecallSet is 57, max is 30 |
| L0029 | `internal/mobilebackup/history_page.go:17:1` | calculated cyclomatic complexity for function ReadHistoryPage is 42, max is 30 |
| L0030 | `internal/mobilebackup/history_page_test.go:73:1` | calculated cyclomatic complexity for function TestSnapshotReaderConverterJournalResumeTogether is 42, max is 30 |
| L0031 | `internal/mobilebackup/history_worker_test.go:582:1` | calculated cyclomatic complexity for function TestMobileWorkerWholeControlPreludeResumesWithoutRedispatch is 35, max is 30 |
| L0032 | `internal/mobilebackup/message_conversion.go:91:1` | calculated cyclomatic complexity for function convertArchiveCandidates is 62, max is 30 |
| L0033 | `internal/mobilebackup/native_binary_test.go:18:1` | calculated cyclomatic complexity for function TestNativeBinaryTerminalStartupAcceptance is 40, max is 30 |
| L0034 | `internal/mobilebackup/prepared_page.go:44:1` | calculated cyclomatic complexity for function ReadPreparedArchivePage is 31, max is 30 |
| L0035 | `internal/mobilebackup/recall_history_test.go:14:1` | calculated cyclomatic complexity for function TestRecallSnapshotConverterJournalResumeTogether is 38, max is 30 |
| L0036 | `internal/mobilebackup/retained_archive.go:448:1` | calculated cyclomatic complexity for function decode is 39, max is 30 |
| L0037 | `internal/mobilebackup/row_page.go:81:1` | calculated cyclomatic complexity for function PrepareRowPage is 56, max is 30 |
| L0038 | `internal/mobilebackup/snapshot.go:278:1` | calculated cyclomatic complexity for function Save is 35, max is 30 |
| L0039 | `internal/mobilebackup/snapshot_page.go:76:1` | calculated cyclomatic complexity for function ReadSnapshotPage is 41, max is 30 |
| L0040 | `internal/mobilebackup/snapshot_page_test.go:108:1` | calculated cyclomatic complexity for function TestSnapshotTextProjectionIdentityAuthorsAndGaps is 36, max is 30 |
| L0041 | `internal/mobilebackup/snapshot_sqlite_test.go:13:1` | calculated cyclomatic complexity for function TestSnapshotSQLiteZeroIDsPagingAndImportIsolation is 42, max is 30 |
| L0042 | `internal/mobilebackup/snapshot_terminal.go:15:1` | calculated cyclomatic complexity for function CleanupTerminal is 41, max is 30 |
| L0043 | `internal/mobilebackup/sqlite.go:169:1` | calculated cyclomatic complexity for function readSQLitePageMode is 86, max is 30 |
| L0044 | `internal/mobilebackup/xz.go:53:1` | calculated cyclomatic complexity for function inspectXZ is 43, max is 30 |
| L0045 | `internal/service/account_archive_control.go:24:1` | calculated cyclomatic complexity for function accountArchiveControl is 39, max is 30 |
| L0046 | `internal/service/account_archive_test.go:51:1` | calculated cyclomatic complexity for function testAccountArchiveCapture is 69, max is 30 |
| L0047 | `internal/service/archive_catalog.go:43:1` | calculated cyclomatic complexity for function archiveConversations is 33, max is 30 |
| L0048 | `internal/service/archive_messages.go:156:1` | calculated cyclomatic complexity for function archiveMessages is 31, max is 30 |
| L0049 | `internal/service/archive_messages_test.go:20:1` | calculated cyclomatic complexity for function testOfflineArchiveMessages is 31, max is 30 |
| L0050 | `internal/service/cli.go:28:1` | calculated cyclomatic complexity for function RunCLI is 35, max is 30 |
| L0051 | `internal/service/cli_mobile_ledger.go:29:1` | calculated cyclomatic complexity for function mobileLedgerArguments is 32, max is 30 |
| L0052 | `internal/service/conversations_test.go:122:1` | calculated cyclomatic complexity for function testConversationServiceProfile is 41, max is 30 |
| L0053 | `internal/service/events_restart_test.go:123:1` | calculated cyclomatic complexity for function TestServiceEventsSurviveRestartWithStableDeliveryAndBoundary is 49, max is 30 |
| L0054 | `internal/service/history_test.go:82:1` | calculated cyclomatic complexity for function TestHTTPHistoryToolsUseExistingCollectorSession is 32, max is 30 |
| L0055 | `internal/service/mobile_ledger_control.go:72:1` | calculated cyclomatic complexity for function mobileLedgerControl is 38, max is 30 |
| L0056 | `internal/service/mobile_page_test.go:103:1` | calculated cyclomatic complexity for function TestServiceMobilePageUsesCollectorSessionAndExpiresHistoricalRows is 48, max is 30 |
| L0057 | `internal/service/multiprocess_test.go:40:1` | calculated cyclomatic complexity for function TestTwoMCPProcessesReadCollectorWrites is 67, max is 30 |
| L0058 | `internal/service/recall_diagnostic.go:77:1` | calculated cyclomatic complexity for function probeRecall is 51, max is 30 |
| L0059 | `internal/service/run.go:154:1` | calculated cyclomatic complexity for function runConfigured is 57, max is 30 |
| L0060 | `internal/service/run_test.go:49:1` | calculated cyclomatic complexity for function TestServicePreservesMCPWhenAuthenticationIsRequired is 48, max is 30 |
| L0061 | `internal/service/sending_test.go:79:1` | calculated cyclomatic complexity for function TestServiceSendAndReplyUseCollectorSessionAndPersistAcrossRestart is 46, max is 30 |
| L0062 | `internal/storage/browse.go:18:1` | calculated cyclomatic complexity for function Browse is 47, max is 30 |
| L0063 | `internal/storage/deliveries.go:61:1` | calculated cyclomatic complexity for function FanoutProfileEvents is 33, max is 30 |
| L0064 | `internal/storage/events.go:159:1` | calculated cyclomatic complexity for function ActivateSubscription is 33, max is 30 |
| L0065 | `internal/storage/history_operations.go:430:1` | calculated cyclomatic complexity for function commitHistoryRecords is 36, max is 30 |
| L0066 | `internal/storage/history_operations_test.go:57:1` | calculated cyclomatic complexity for function TestHistoryOperationDurableCheckpointAndRequestIdentity is 35, max is 30 |
| L0067 | `internal/storage/mobile_history.go:158:1` | calculated cyclomatic complexity for function CommitMobileHistoryPage is 98, max is 30 |
| L0068 | `internal/storage/mobile_history_test.go:133:1` | calculated cyclomatic complexity for function TestMobileHistoryRestartExactKeysetCountsAndSilentDuplicates is 31, max is 30 |
| L0069 | `internal/toolruntime/runtime.go:144:1` | calculated cyclomatic complexity for function New is 45, max is 30 |
| L0070 | `internal/toolruntime/runtime.go:464:1` | calculated cyclomatic complexity for function dispatch is 32, max is 30 |
| L0071 | `internal/zalo/events.go:48:1` | calculated cyclomatic complexity for function listenConversations is 33, max is 30 |
| L0072 | `internal/zalo/mobile_backup.go:60:1` | calculated cyclomatic complexity for function receiveMobileBackupOffer is 66, max is 30 |
| L0073 | `internal/zalo/preload.go:29:1` | calculated cyclomatic complexity for function readConversationPreload is 52, max is 30 |
| L0074 | `tools/harness/internal/performance/bench.go:75:1` | calculated cyclomatic complexity for function Bench is 42, max is 30 |
| L0075 | `tools/harness/internal/performance/compare.go:8:1` | calculated cyclomatic complexity for function validate is 56, max is 30 |
| L0076 | `tools/harness/internal/performance/compare.go:150:1` | calculated cyclomatic complexity for function Compare is 39, max is 30 |

### embeddedstructfieldcheck (1)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L0077: отделить embedded field пустой строкой. Механическое форматирование, исключение не нужно.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0077 | `tools/harness/internal/architecture/architecture.go:71:2` | there must be an empty line separating embedded fields from regular fields |

### errcheck (115)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Разделить по смыслу: ошибки записи/Commit/Scan/Encode и setup fixture обрабатывать; при rollback/cleanup после основной ошибки допустимо явное игнорирование с причиной (не подменять исходную ошибку). Для Assert/Marshal без возможной ошибки — только доказанный локальный nolint. L0078–L0090: unchecked assertions из map в store adapter заменить типизированной внутренней проекцией либо comma-ok с явной internal error; не превращать несоответствие типов в нулевой успешный ответ. L0112: malformed grader input должен давать явный fail, а не продолжать проверки по zero value. L0125–L0127: fallback logger не должен рекурсивно логировать ошибку собственного Write; допустим локальный nolint с причиной last-resort best effort. Ответные Encode/Write после отправки HTTP headers не повторять при ошибке: завершить handler, сохранить no-retry и privacy. Close после чтения и Close перед persistence commit имеют разные требования; не добавлять общий ignore Close.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0078 | `internal/adapters/store/read.go:85:24` | Error return value is not checked |
| L0079 | `internal/adapters/store/read.go:86:24` | Error return value is not checked |
| L0080 | `internal/adapters/store/read.go:87:24` | Error return value is not checked |
| L0081 | `internal/adapters/store/read.go:96:31` | Error return value is not checked |
| L0082 | `internal/adapters/store/read.go:97:31` | Error return value is not checked |
| L0083 | `internal/adapters/store/read.go:98:31` | Error return value is not checked |
| L0084 | `internal/adapters/store/read.go:106:9` | Error return value is not checked |
| L0085 | `internal/adapters/store/read.go:124:21` | Error return value is not checked |
| L0086 | `internal/adapters/store/read.go:125:21` | Error return value is not checked |
| L0087 | `internal/adapters/store/read.go:126:21` | Error return value is not checked |
| L0088 | `internal/adapters/store/read.go:127:21` | Error return value is not checked |
| L0089 | `internal/adapters/store/read.go:128:21` | Error return value is not checked |
| L0090 | `internal/adapters/store/read.go:129:21` | Error return value is not checked |
| L0091 | `internal/collector/contact_profiles.go:73:32` | Error return value is not checked |
| L0092 | `internal/collector/contact_profiles.go:106:31` | Error return value is not checked |
| L0093 | `internal/collector/contact_profiles.go:107:30` | Error return value is not checked |
| L0094 | `internal/collector/contact_profiles.go:110:7` | Error return value is not checked |
| L0095 | `internal/collector/contacts.go:97:31` | Error return value is not checked |
| L0096 | `internal/collector/contacts.go:99:33` | Error return value is not checked |
| L0097 | `internal/collector/join.go:336:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0098 | `internal/collector/join.go:584:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0099 | `internal/collector/run.go:285:29` | Error return value of &#96;(*encoding/json.Encoder).Encode&#96; is not checked |
| L0100 | `internal/collector/run.go:323:29` | Error return value of &#96;(*encoding/json.Encoder).Encode&#96; is not checked |
| L0101 | `internal/collector/run.go:349:12` | Error return value of &#96;(*encoding/json.Encoder).Encode&#96; is not checked |
| L0102 | `internal/collector/run.go:357:29` | Error return value of &#96;(*encoding/json.Encoder).Encode&#96; is not checked |
| L0103 | `internal/collector/run.go:365:28` | Error return value of &#96;(*encoding/json.Encoder).Encode&#96; is not checked |
| L0104 | `internal/collector/run.go:392:10` | Error return value is not checked |
| L0105 | `internal/collector/run.go:402:33` | Error return value is not checked |
| L0106 | `internal/collector/run.go:416:30` | Error return value is not checked |
| L0107 | `internal/collector/run.go:419:28` | Error return value is not checked |
| L0108 | `internal/collector/run.go:419:57` | Error return value is not checked |
| L0109 | `internal/collector/run.go:421:32` | Error return value is not checked |
| L0110 | `internal/evalfixture/fixture.go:222:40` | Error return value is not checked |
| L0111 | `internal/evalfixture/offline.go:170:31` | Error return value is not checked |
| L0112 | `internal/evaluation/grader.go:69:18` | Error return value of &#96;json.Unmarshal&#96; is not checked |
| L0113 | `internal/evaluation/runner.go:268:17` | Error return value is not checked |
| L0114 | `internal/evaluation/scenarios.go:114:55` | Error return value is not checked |
| L0115 | `internal/evaluation/scenarios.go:173:10` | Error return value is not checked |
| L0116 | `internal/evaluation/scenarios.go:174:11` | Error return value is not checked |
| L0117 | `internal/events/message.go:50:16` | Error return value is not checked |
| L0118 | `internal/events/message.go:51:10` | Error return value is not checked |
| L0119 | `internal/events/message.go:60:82` | Error return value is not checked |
| L0120 | `internal/events/subscriptions.go:172:16` | Error return value is not checked |
| L0121 | `internal/events/subscriptions.go:184:14` | Error return value is not checked |
| L0122 | `internal/events/subscriptions.go:193:22` | Error return value is not checked |
| L0123 | `internal/events/subscriptions.go:197:3` | Error return value is not checked |
| L0124 | `internal/local/files.go:21:28` | Error return value of &#96;unix.Flock&#96; is not checked |
| L0125 | `internal/logging/startup.go:46:17` | Error return value of &#96;fallback.Write&#96; is not checked |
| L0126 | `internal/logging/startup.go:52:17` | Error return value of &#96;fallback.Write&#96; is not checked |
| L0127 | `internal/logging/startup.go:63:18` | Error return value of &#96;fallback.Write&#96; is not checked |
| L0128 | `internal/logging/startup.go:68:18` | Error return value of &#96;unix.Flock&#96; is not checked |
| L0129 | `internal/mcpserver/http.go:73:27` | Error return value of &#96;(*encoding/json.Encoder).Encode&#96; is not checked |
| L0130 | `internal/mcpserver/http.go:284:10` | Error return value of &#96;w.Write&#96; is not checked |
| L0131 | `internal/mcpserver/server.go:436:8` | Error return value is not checked |
| L0132 | `internal/mobilebackup/retained_archive.go:327:28` | Error return value of &#96;s.ledger.root.Remove&#96; is not checked |
| L0133 | `internal/mobilebackup/snapshot.go:368:21` | Error return value of &#96;s.root.Remove&#96; is not checked |
| L0134 | `internal/service/archive_catalog.go:167:10` | Error return value is not checked |
| L0135 | `internal/service/bridge.go:46:15` | Error return value is not checked |
| L0136 | `internal/service/run.go:44:60` | Error return value is not checked |
| L0137 | `internal/storage/browse.go:159:10` | Error return value is not checked |
| L0138 | `internal/storage/contacts.go:19:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0139 | `internal/storage/contacts.go:53:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0140 | `internal/storage/conversation_migration.go:13:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0141 | `internal/storage/conversation_views.go:65:45` | Error return value is not checked |
| L0142 | `internal/storage/conversation_views.go:68:21` | Error return value is not checked |
| L0143 | `internal/storage/conversation_views.go:73:10` | Error return value is not checked |
| L0144 | `internal/storage/conversation_views.go:78:36` | Error return value is not checked |
| L0145 | `internal/storage/conversation_views.go:205:12` | Error return value is not checked |
| L0146 | `internal/storage/conversation_views.go:206:10` | Error return value is not checked |
| L0147 | `internal/storage/deliveries.go:74:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0148 | `internal/storage/deliveries.go:248:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0149 | `internal/storage/deliveries.go:344:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0150 | `internal/storage/diagnostics.go:37:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0151 | `internal/storage/event_journal.go:23:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0152 | `internal/storage/event_journal.go:146:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0153 | `internal/storage/event_journal.go:263:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0154 | `internal/storage/event_journal.go:302:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0155 | `internal/storage/events.go:20:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0156 | `internal/storage/events.go:181:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0157 | `internal/storage/events.go:265:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0158 | `internal/storage/history.go:29:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0159 | `internal/storage/history_expiry.go:18:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0160 | `internal/storage/history_expiry.go:47:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0161 | `internal/storage/history_expiry.go:114:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0162 | `internal/storage/history_operations.go:46:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0163 | `internal/storage/history_operations.go:177:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0164 | `internal/storage/history_operations.go:273:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0165 | `internal/storage/history_operations.go:297:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0166 | `internal/storage/history_operations.go:375:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0167 | `internal/storage/history_operations.go:551:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0168 | `internal/storage/history_operations.go:670:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0169 | `internal/storage/history_operations.go:732:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0170 | `internal/storage/incoming.go:16:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0171 | `internal/storage/mobile_backup.go:42:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0172 | `internal/storage/mobile_backup.go:97:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0173 | `internal/storage/mobile_backup.go:245:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0174 | `internal/storage/mobile_history.go:28:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0175 | `internal/storage/mobile_history.go:170:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0176 | `internal/storage/mobile_history_acquisition.go:17:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0177 | `internal/storage/mobile_history_acquisition.go:41:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0178 | `internal/storage/mobile_history_acquisition.go:156:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0179 | `internal/storage/mobile_history_acquisition.go:195:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0180 | `internal/storage/preload_directory.go:32:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0181 | `internal/storage/sending.go:23:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0182 | `internal/storage/sending.go:80:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0183 | `internal/storage/store.go:180:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0184 | `internal/storage/store.go:240:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0185 | `internal/storage/store.go:390:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0186 | `internal/storage/tombstones.go:15:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0187 | `internal/storage/tombstones.go:42:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0188 | `internal/storage/views.go:142:19` | Error return value of &#96;tx.Rollback&#96; is not checked |
| L0189 | `internal/storage/views.go:379:19` | Error return value is not checked |
| L0190 | `internal/storage/views.go:462:12` | Error return value is not checked |
| L0191 | `internal/storage/views.go:463:33` | Error return value is not checked |
| L0192 | `internal/storage/views.go:488:19` | Error return value of &#96;tx.Rollback&#96; is not checked |

### errname (6)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Переименовать локальные/test ошибки storageFailure/privateFailure/callbackFailure и workerErr в storageError/privateError/callbackError/errWorker, обновить references. Для domain.HistorySourceFailure/HistoryPageFailure сначала проверить всех потребителей: если имена часть согласованного Go-контракта, оставить nolint:errname с причиной; wire reason/state не переименовывать ради стиля.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0193 | `internal/collector/run.go:30:6` | the error type name &#96;storageFailure&#96; should conform to the &#96;xxxError&#96; format |
| L0194 | `internal/domain/history.go:13:6` | the error type name &#96;HistorySourceFailure&#96; should conform to the &#96;XxxError&#96; format |
| L0195 | `internal/domain/history.go:57:6` | the error type name &#96;HistoryPageFailure&#96; should conform to the &#96;XxxError&#96; format |
| L0196 | `internal/observability/observer_test.go:48:6` | the error type name &#96;privateFailure&#96; should conform to the &#96;xxxError&#96; format |
| L0197 | `internal/observability/observer_test.go:351:6` | the error type name &#96;callbackFailure&#96; should conform to the &#96;xxxError&#96; format |
| L0198 | `tools/harness/internal/performance/performance_test.go:201:5` | the sentinel error name &#96;workerErr&#96; should conform to the &#96;errXxx&#96; format |

### exhaustive (6)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Добавить явные перечисленные ветви, сохранив текущий fallback для неизвестных значений. OutcomeUnknown/WorkOther/StateUnknown должны возвращать прежнюю нормализованную метку. Для L0203 явно объединить текущие нетерминальные states в прежний результат. L0204: не добавлять ThreadTypeUser в карту поддерживаемых типов автоматически — если карта намеренно содержит только поддерживаемое подмножество, локальный nolint:exhaustive с описанием этого подмножества.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0199 | `internal/domain/observation.go:16:2` | missing cases in switch of type domain.Outcome: domain.OutcomeUnknown |
| L0200 | `internal/domain/observation.go:44:2` | missing cases in switch of type domain.WorkKind: domain.WorkOther |
| L0201 | `internal/domain/observation.go:82:2` | missing cases in switch of type domain.OperationState: domain.StateUnknown |
| L0202 | `internal/storage/observation.go:14:2` | missing cases in switch of type domain.WorkKind: domain.WorkCollector, domain.WorkOther |
| L0203 | `internal/toolruntime/outcome.go:31:2` | missing cases in switch of type domain.OperationState: domain.StateUnknown, domain.StatePending, domain.StateSending, domain.StateQueued, domain.StateRunning, domain.StatePendingApproval |
| L0204 | `internal/zalo/replay.go:21:7` | missing keys in map of key type model.ThreadType: model.ThreadTypeUser |

### exhaustruct_v5 (307)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Не заполнять сотни DTO литералов нулями. Для sparse response DTO, fixtures и типов options с документированными defaults — узкое исключение по конкретному типу или локальный nolint:exhaustruct_v5: omitted fields intentionally use zero/default values. Для обязательных полей resources/security options проверить реальную инициализацию и дописать только требуемые значения. Не исключать весь internal/domain или весь production; null/missing/empty и SDK MaxLineLength проверять по контракту, а не по требованию заполнить структуру.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0205 | `cmd/zl-mcp-eval-fixture/main.go:27:10` | evalfixture.Options is missing fields Policy, Offline, StateDir, EffectsFile, SendTimeout, DenySend, KeepState, ArchiveVector, Fixtures, SkillDir, Empty, Stopped, JoinTimeout, ApprovalFile, ApprovalTokenFile |
| L0206 | `cmd/zl-mcp-eval-fixture/main.go:73:34` | mcp.StdioTransport is missing field MaxLineLength |
| L0207 | `internal/adapters/operations/read.go:36:19` | domain.Error is missing field Retryable |
| L0208 | `internal/adapters/operations/read.go:39:16` | domain.NextAction is missing fields Tool, Arguments |
| L0209 | `internal/adapters/operations/read.go:48:19` | domain.Error is missing field Retryable |
| L0210 | `internal/adapters/operations/read.go:51:16` | domain.NextAction is missing fields Tool, Arguments |
| L0211 | `internal/adapters/store/read.go:25:11` | domain.Error is missing field Retryable |
| L0212 | `internal/adapters/store/read.go:28:16` | domain.NextAction is missing fields Tool, Arguments |
| L0213 | `internal/adapters/store/read.go:35:9` | storage.Search is missing fields General, ConversationType, ConversationID, GroupID |
| L0214 | `internal/adapters/store/read.go:123:9` | conversations.GroupContext is missing field NextAction |
| L0215 | `internal/app/search/read.go:152:9` | search.GroupResult is missing fields EmptyReason, CoverageSummary |
| L0216 | `internal/app/search/read.go:200:9` | search.ConversationResult is missing fields EmptyReason, CoverageSummary |
| L0217 | `internal/collector/auth_guard.go:22:10` | collector.sessionGuard is missing field authRequired |
| L0218 | `internal/collector/join.go:50:10` | collector.JoinManager is missing field wg |
| L0219 | `internal/collector/join.go:54:10` | domain.Error is missing field Retryable |
| L0220 | `internal/collector/join.go:57:15` | domain.NextAction is missing fields Tool, Arguments |
| L0221 | `internal/collector/join.go:159:7` | collector.preview is missing fields ID, URL, GroupID, Name, Approval, Expires |
| L0222 | `internal/collector/join.go:306:17` | domain.NextAction is missing fields Tool, Arguments |
| L0223 | `internal/collector/join.go:414:19` | domain.NextAction is missing fields Tool, Arguments |
| L0224 | `internal/collector/join.go:468:20` | domain.NextAction is missing fields Tool, Arguments |
| L0225 | `internal/collector/run.go:275:11` | domain.Error is missing field Retryable |
| L0226 | `internal/collector/run.go:278:17` | domain.NextAction is missing fields Tool, Arguments |
| L0227 | `internal/control/client.go:22:14` | net.Dialer is missing fields Timeout, Deadline, LocalAddr, DualStack, FallbackDelay, KeepAlive, KeepAliveConfig, Resolver, Cancel, Control, ControlContext |
| L0228 | `internal/control/client.go:43:16` | domain.NextAction is missing fields Tool, Arguments |
| L0229 | `internal/domain/history.go:19:8` | domain.HistorySourceFailure is missing field APICode |
| L0230 | `internal/domain/mobile_backup_request.go:52:13` | domain.HistoryImportRequest is missing fields Source, PageSize, MaxPages |
| L0231 | `internal/domain/mobile_history.go:86:14` | domain.MobileHistoryPosition is missing fields TimestampMS, RowID |
| L0232 | `internal/domain/mobile_history.go:88:8` | domain.MobileHistoryPage is missing fields ControlPrelude, Snapshot, Records, Recalls, Counts, HasMore, Next |
| L0233 | `internal/domain/model.go:57:10` | domain.Error is missing field Retryable |
| L0234 | `internal/domain/model.go:60:15` | domain.NextAction is missing fields Tool, Arguments |
| L0235 | `internal/domain/model.go:90:10` | domain.Error is missing field Retryable |
| L0236 | `internal/domain/model.go:93:15` | domain.NextAction is missing fields Tool, Arguments |
| L0237 | `internal/evalfixture/evidence.go:28:9` | storage.EventSubscription is missing fields Direction, FirstIncomingOnly, ConversationType, GroupID, Generation, StartSeq, ExpiresAt |
| L0238 | `internal/evalfixture/evidence.go:46:4` | domain.Message is missing fields FirstIncoming, QuoteMetadata, ConversationName, GroupID, SenderName, ReplyTo, AttachmentTypes, TextTruncated, TextResourceURI |
| L0239 | `internal/evalfixture/evidence.go:82:4` | storage.DeliveryOutcome is missing fields RetryAt, Gone |
| L0240 | `internal/evalfixture/evidence.go:113:9` | domain.HistoryPage is missing fields PhaseRequired, LimitedSnapshot, Cursor, IsFiltered, IsFilteredByPhase, IsFilteredByTimeJoin, IsOld, JoinTimestampMillis |
| L0241 | `internal/evalfixture/evidence.go:115:4` | domain.Message is missing fields Direction, FirstIncoming, QuoteMetadata, ConversationName, GroupID, SenderName, ReplyTo, AttachmentTypes, Source, TextTruncated, TextResourceURI |
| L0242 | `internal/evalfixture/fixture.go:92:27` | domain.Group is missing fields MemberCount, CollectionEnabled |
| L0243 | `internal/evalfixture/fixture.go:109:9` | domain.Invite is missing fields Description, ApprovalRequired |
| L0244 | `internal/evalfixture/fixture.go:109:30` | domain.Group is missing fields MemberCount, CollectionEnabled |
| L0245 | `internal/evalfixture/fixture.go:155:8` | evalfixture.Fixture is missing fields Store, Server, Upstream, joins, http, heartbeatDone, offline |
| L0246 | `internal/evalfixture/fixture.go:166:13` | domain.CollectionPolicy is missing field Selected |
| L0247 | `internal/evalfixture/fixture.go:212:16` | evalfixture.Upstream is missing fields mu, joined, calls, effectPath |
| L0248 | `internal/evalfixture/fixture.go:312:6` | mcp.Resource is missing fields Meta, Annotations, Description, Size, Title, Icons |
| L0249 | `internal/evalfixture/fixture.go:314:14` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0250 | `internal/evalfixture/fixture.go:315:41` | mcp.ResourceContents is missing fields Blob, Meta |
| L0251 | `internal/evalfixture/offline.go:71:8` | evalfixture.offlineControl is missing fields library, archive, fixtureSendEnabled, historyCancel, historyDone |
| L0252 | `internal/evalfixture/offline.go:74:13` | evalfixture.offlineSender is missing field mu |
| L0253 | `internal/evalfixture/offline.go:93:5` | domain.Message is missing fields Direction, FirstIncoming, QuoteMetadata, ConversationName, GroupID, SenderName, ReplyTo, AttachmentTypes, TextTruncated, TextResourceURI |
| L0254 | `internal/evalfixture/offline.go:108:4` | domain.Message is missing fields Direction, FirstIncoming, ConversationName, GroupID, SenderName, ReplyTo, AttachmentTypes, TextTruncated, TextResourceURI |
| L0255 | `internal/evalfixture/offline.go:115:21` | domain.QuoteMetadata is missing fields ClientMessageID, MessageType, Timestamp, TTL |
| L0256 | `internal/evalfixture/offline.go:202:25` | domain.MobileIdentityPair is missing field Group |
| L0257 | `internal/evalfixture/offline.go:245:3` | domain.MobileBackupOffer is missing fields PlainAccountID, SessionAccountID, FromSequence |
| L0258 | `internal/evalfixture/read_ports.go:33:15` | toolruntime.Capabilities is missing fields Events, Archives |
| L0259 | `internal/evaluation/benchmark.go:40:11` | evaluation.Measurement is missing fields Calls, Errors, Bytes, P95Seconds, AllocationsPerOp, AllocBytesPerOp, ElapsedSeconds, Cleanup |
| L0260 | `internal/evaluation/benchmark.go:77:3` | evalfixture.Options is missing fields Policy, SendTimeout, DenySend, KeepState, ArchiveVector, SkillDir, Empty, Stopped, JoinTimeout, ApprovalFile, ApprovalTokenFile |
| L0261 | `internal/evaluation/benchmark.go:112:43` | mcp.CallToolParams is missing fields Meta, InputResponses, RequestState |
| L0262 | `internal/evaluation/client.go:25:28` | mcp.Implementation is missing fields Title, Description, WebsiteURL, Icons |
| L0263 | `internal/evaluation/client.go:30:10` | evaluation.client is missing field trace |
| L0264 | `internal/evaluation/client.go:39:37` | mcp.CallToolParams is missing fields Meta, InputResponses, RequestState |
| L0265 | `internal/evaluation/client.go:40:7` | evaluation.Trace is missing fields Result, Error |
| L0266 | `internal/evaluation/client.go:59:41` | mcp.ReadResourceParams is missing fields Meta, InputResponses, RequestState |
| L0267 | `internal/evaluation/client.go:60:7` | evaluation.Trace is missing fields Input, Result, Error |
| L0268 | `internal/evaluation/grader.go:107:12` | evaluation.Trace is missing fields Kind, Name, Input, Result, Error |
| L0269 | `internal/evaluation/grader.go:188:13` | evaly.GraderRevision is missing fields Model, Prompt, Configuration |
| L0270 | `internal/evaluation/grader.go:194:11` | evaly.Grade is missing fields Dispatched, Revision, Metrics, Reasons, EvidenceRefs |
| L0271 | `internal/evaluation/grader.go:196:17` | evaly.Usage is missing field Units |
| L0272 | `internal/evaluation/model.go:31:8` | evaluation.Trace is missing fields Kind, Name, Input, Result, Error |
| L0273 | `internal/evaluation/model.go:38:8` | evaluation.Trace is missing fields Result, Error |
| L0274 | `internal/evaluation/model.go:52:8` | evaluation.Trace is missing fields Input, Result, Error |
| L0275 | `internal/evaluation/model.go:72:8` | evaluation.Output is missing fields Trace, Cleanup, Effects, Complete, Failure, ElapsedSeconds, Bytes |
| L0276 | `internal/evaluation/model.go:81:3` | evalfixture.Options is missing fields Policy, SendTimeout, DenySend, ArchiveVector, SkillDir, Empty, Stopped, JoinTimeout, ApprovalFile, ApprovalTokenFile |
| L0277 | `internal/evaluation/model.go:92:14` | evaluation.modelCapture is missing fields mu, calls, violated, trace |
| L0278 | `internal/evaluation/process.go:48:13` | evaluation.boundedBuffer is missing fields mu, buffer, exceeded |
| L0279 | `internal/evaluation/process.go:49:13` | evaluation.boundedBuffer is missing fields mu, buffer, exceeded |
| L0280 | `internal/evaluation/process.go:55:21` | syscall.SysProcAttr is missing fields Chroot, Credential, Ptrace, Setsid, Setctty, Noctty, Ctty, Foreground, Pgid |
| L0281 | `internal/evaluation/runner.go:136:11` | evaluation.Report is missing fields Reason, Experiment, ExpectedCases, PassedCases, ExpectedAssertions, PassedAssertions, FailedCases, SkippedCases |
| L0282 | `internal/evaluation/runner.go:250:11` | evaly.DatasetDraft is missing fields Cases, ParentRevision |
| L0283 | `internal/evaluation/runner.go:253:37` | evaly.Case is missing fields Revision, Metadata, RequiredEvidence, Generation |
| L0284 | `internal/evaluation/runner.go:262:10` | evaly.RunConfig is missing field Budget |
| L0285 | `internal/evaluation/runner.go:296:16` | evaluation.Output is missing fields Trace, Facts, Cleanup, Effects, Complete, ElapsedSeconds, Bytes |
| L0286 | `internal/evaluation/runner.go:322:15` | evaly.TargetResult is missing field Usage |
| L0287 | `internal/evaluation/runner.go:328:8` | evaly.Event is missing field CorrelationID |
| L0288 | `internal/evaluation/runner.go:336:15` | evaly.TargetResult is missing field Usage |
| L0289 | `internal/evaluation/runner.go:339:63` | evaly.Usage is missing field Units |
| L0290 | `internal/evaluation/runner.go:363:10` | evaly.RunPlan is missing fields StopOnInfrastructure, DispatchUnits, GraderUnits |
| L0291 | `internal/evaluation/runner.go:372:16` | evaly.Provenance is missing fields Model, Prompt, Provider |
| L0292 | `internal/evaluation/scenarios.go:25:13` | evalfixture.Options is missing fields Policy, SendTimeout, DenySend, ArchiveVector, SkillDir, Empty, Stopped, JoinTimeout, ApprovalFile, ApprovalTokenFile |
| L0293 | `internal/evaluation/scenarios.go:199:15` | domain.Message is missing fields Direction, FirstIncoming, QuoteMetadata, ConversationName, GroupID, SenderName, ReplyTo, AttachmentTypes, Source, TextTruncated, TextResourceURI |
| L0294 | `internal/evaluation/scenarios.go:236:12` | domain.CollectionPolicy is missing field All |
| L0295 | `internal/events/message.go:73:10` | events.MessageData is missing fields TextTruncated, TextResourceURI |
| L0296 | `internal/events/message.go:89:3` | events.MessageEvent is missing field Cursor |
| L0297 | `internal/events/subscriptions.go:57:8` | events.SubscriptionManager is missing field mu |
| L0298 | `internal/events/subscriptions.go:256:11` | storage.EventSubscription is missing fields Direction, FirstIncomingOnly, Profile, Scope, ConversationType, Generation, StartSeq, ExpiresAt |
| L0299 | `internal/events/worker.go:44:10` | events.Worker is missing fields Logger, TraceLogger |
| L0300 | `internal/events/worker.go:120:13` | storage.DeliveryOutcome is missing field Gone |
| L0301 | `internal/historyimport/manager.go:18:10` | domain.Error is missing field Retryable |
| L0302 | `internal/historyimport/manager.go:21:15` | domain.NextAction is missing fields Tool, Arguments |
| L0303 | `internal/historyimport/mobile_worker.go:196:27` | domain.MobileBackupOffer is missing fields PlainAccountID, SessionAccountID, URL, KeyText, FileSize, FromSequence |
| L0304 | `internal/historyimport/mobile_worker.go:224:12` | domain.MobileBackupOffer is missing fields PlainAccountID, SessionAccountID, URL, KeyText, FileSize, FromSequence |
| L0305 | `internal/logging/file.go:58:10` | logging.File is missing fields mu, err |
| L0306 | `internal/logging/file.go:59:14` | lumberjack.Logger is missing fields MaxAge, LocalTime, Compress |
| L0307 | `internal/mcpserver/catalog_diagnostics.go:19:4` | mcp.Resource is missing fields Meta, Annotations, Description, Size, Title, Icons |
| L0308 | `internal/mcpserver/catalog_diagnostics.go:54:12` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0309 | `internal/mcpserver/catalog_diagnostics.go:56:6` | mcp.ResourceContents is missing fields Blob, Meta |
| L0310 | `internal/mcpserver/conversations.go:23:4` | mcp.ResourceTemplate is missing fields Meta, Annotations, Description, Title, Icons |
| L0311 | `internal/mcpserver/conversations.go:57:12` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0312 | `internal/mcpserver/conversations.go:59:6` | mcp.ResourceContents is missing fields Blob, Meta |
| L0313 | `internal/mcpserver/conversations.go:69:4` | mcp.Resource is missing fields Meta, Annotations, Description, Size, Title, Icons |
| L0314 | `internal/mcpserver/conversations.go:93:12` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0315 | `internal/mcpserver/conversations.go:95:6` | mcp.ResourceContents is missing fields Blob, Meta |
| L0316 | `internal/mcpserver/conversations.go:101:4` | mcp.ResourceTemplate is missing fields Meta, Annotations, Description, Title, Icons |
| L0317 | `internal/mcpserver/conversations.go:133:5` | conversations.MessageRequest is missing field LegacyGroup |
| L0318 | `internal/mcpserver/conversations.go:144:12` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0319 | `internal/mcpserver/conversations.go:145:39` | mcp.ResourceContents is missing fields Blob, Meta |
| L0320 | `internal/mcpserver/diagnostics.go:20:4` | mcp.Resource is missing fields Meta, Annotations, Description, Size, Title, Icons |
| L0321 | `internal/mcpserver/diagnostics.go:45:12` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0322 | `internal/mcpserver/diagnostics.go:47:6` | mcp.ResourceContents is missing fields Blob, Meta |
| L0323 | `internal/mcpserver/events_protocol.go:34:10` | mcpserver.protocolError is missing field Data |
| L0324 | `internal/mcpserver/http.go:50:5` | mcp.StreamableHTTPOptions is missing fields Logger, EventStore, SessionTimeout, DisableLocalhostProtection, CrossOriginProtection, MaxRequestBodyBytes, OnRequestSummary, StreamKeepAlive |
| L0325 | `internal/mcpserver/http.go:142:26` | mcpserver.protocolError is missing field Data |
| L0326 | `internal/mcpserver/http.go:146:26` | mcpserver.protocolError is missing field Data |
| L0327 | `internal/mcpserver/http.go:162:15` | mcpserver.telemetryResponse is missing field bytes |
| L0328 | `internal/mcpserver/http.go:167:16` | mcpserver.discoveryResponse is missing fields status, written, overflow, body |
| L0329 | `internal/mcpserver/http.go:189:27` | mcpserver.protocolError is missing field Data |
| L0330 | `internal/mcpserver/http.go:197:6` | mcpserver.protocolError is missing field Data |
| L0331 | `internal/mcpserver/http.go:205:34` | mcpserver.protocolError is missing field Data |
| L0332 | `internal/mcpserver/http.go:213:6` | mcpserver.protocolError is missing field Data |
| L0333 | `internal/mcpserver/http.go:224:6` | mcpserver.protocolError is missing field Data |
| L0334 | `internal/mcpserver/http.go:236:6` | mcpserver.protocolError is missing field Data |
| L0335 | `internal/mcpserver/http.go:254:27` | mcpserver.protocolError is missing field Data |
| L0336 | `internal/mcpserver/http.go:258:27` | mcpserver.protocolError is missing field Data |
| L0337 | `internal/mcpserver/http.go:270:15` | mcpserver.jsonResponseCapture is missing fields status, body |
| L0338 | `internal/mcpserver/server.go:82:8` | mcpserver.Service is missing field runtime |
| L0339 | `internal/mcpserver/server.go:95:27` | mcp.Implementation is missing fields Title, Description, WebsiteURL, Icons |
| L0340 | `internal/mcpserver/server.go:128:5` | mcp.Tool is missing fields Meta, Title, Icons |
| L0341 | `internal/mcpserver/server.go:133:19` | mcp.ToolAnnotations is missing field Title |
| L0342 | `internal/mcpserver/server.go:155:4` | mcp.Resource is missing fields Meta, Annotations, Description, Size, Title, Icons |
| L0343 | `internal/mcpserver/server.go:164:12` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0344 | `internal/mcpserver/server.go:166:6` | mcp.ResourceContents is missing fields Blob, Meta |
| L0345 | `internal/mcpserver/server.go:176:4` | mcp.ResourceTemplate is missing fields Meta, Annotations, Description, Title, Icons |
| L0346 | `internal/mcpserver/server.go:219:12` | mcp.ReadResourceResult is missing fields Meta, Cacheable, InputRequests, RequestState |
| L0347 | `internal/mcpserver/server.go:220:39` | mcp.ResourceContents is missing fields Blob, Meta |
| L0348 | `internal/mcpserver/server.go:227:9` | domain.Error is missing field Retryable |
| L0349 | `internal/mcpserver/server.go:230:15` | domain.NextAction is missing fields Tool, Arguments |
| L0350 | `internal/mcpserver/server.go:237:10` | mcp.CallToolResult is missing fields Meta, StructuredContent, InputRequests, RequestState |
| L0351 | `internal/mcpserver/server.go:237:68` | mcp.TextContent is missing fields Meta, Annotations |
| L0352 | `internal/mcpserver/server.go:277:10` | domain.Error is missing fields Code, Message, Retryable, NextAction, Details |
| L0353 | `internal/mcpserver/server.go:301:17` | domain.NextAction is missing fields Tool, Arguments |
| L0354 | `internal/mcpserver/server.go:434:29` | mcp.TextContent is missing fields Meta, Annotations |
| L0355 | `internal/mcpserver/server.go:440:6` | mcp.ResourceLink is missing fields Title, Description, Size, Meta, Annotations, Icons |
| L0356 | `internal/mcpserver/server.go:444:10` | mcp.CallToolResult is missing fields Meta, IsError, InputRequests, RequestState |
| L0357 | `internal/messaging/manager.go:30:10` | domain.Error is missing field Retryable |
| L0358 | `internal/messaging/manager.go:33:15` | domain.NextAction is missing fields Tool, Arguments |
| L0359 | `internal/mobilebackup/account_fetch.go:34:7` | mobilebackup.AccountArchive is missing fields ownerAccount, retainedSourceID, archive, pairs, ciphertextBytes, containerBytes, trailingBytes |
| L0360 | `internal/mobilebackup/account_fetch.go:107:12` | mobilebackup.AccountArchive is missing fields ownerAccount, retainedSourceID |
| L0361 | `internal/mobilebackup/account_fetch.go:114:20` | mobilebackup.PlainArchive is missing field Files |
| L0362 | `internal/mobilebackup/account_inspection.go:71:11` | mobilebackup.AccountFileCoverage is missing fields SourceRows, PeriodRows, InvalidTimestamps, EarliestAt, LatestAt, WALMode, Examined, RejectedMessageIDShapes, RejectedMessageIDContext, RejectedReasons, Rejected, SourceControls, DeferredControls, SampleHasMore, Metadata, DeferredMetadata |
| L0363 | `internal/mobilebackup/account_metadata.go:69:13` | mobilebackup.AccountMetadataDiagnostics is missing fields RowsWithMetadata, MissingMetadata, InvalidMetadata, AttachmentCount, UnsupportedFields, SourceTextPresent, TitlePresent, TitleEqualsText, ShapesTruncated |
| L0364 | `internal/mobilebackup/account_metadata.go:100:44` | mobilebackup.PreparedRow is missing fields SenderID, Direction, ExpiresMS, ExpiryDeclared, QuoteExpired |
| L0365 | `internal/mobilebackup/archive.go:32:16` | mobilebackup.ArchiveFile is missing fields Name, Data |
| L0366 | `internal/mobilebackup/archive_library.go:67:9` | mobilebackup.PreservationReceipt is missing fields DownloadPerformed, ImportPerformed, PublicAccessEnabled |
| L0367 | `internal/mobilebackup/archive_library.go:145:9` | mobilebackup.PreservationReceipt is missing fields DownloadPerformed, ImportPerformed, PublicAccessEnabled |
| L0368 | `internal/mobilebackup/archive_restore.go:63:12` | mobilebackup.RetainedArchiveStore is missing field permanent |
| L0369 | `internal/mobilebackup/archive_restore.go:65:12` | mobilebackup.SnapshotStore is missing fields budget, terminalHints |
| L0370 | `internal/mobilebackup/attachment.go:34:8` | mobilebackup.AttachmentValue is missing fields Present, Bytes |
| L0371 | `internal/mobilebackup/attachment.go:41:7` | mobilebackup.Attachment is missing fields Type, ExtInfo, Action, Params, Title, Href, Thumb, Description, Remains, ZInstantData, ZInstantMessage, CategoryID, ID, ChildNumber, UnsupportedFields |
| L0372 | `internal/mobilebackup/binnet.go:31:7` | mobilebackup.BinNet is missing fields Mentions, Quote, Attachments, UnsupportedTags, UnsupportedFields |
| L0373 | `internal/mobilebackup/control_scan.go:23:8` | mobilebackup.OwnDirectRecallSet is missing fields requestID, fingerprint, account, filename, digest, Recalls |
| L0374 | `internal/mobilebackup/control_scan.go:88:15` | domain.MobileIdentityRequest is missing fields Direct, Groups |
| L0375 | `internal/mobilebackup/download.go:41:10` | mobilebackup.Downloader is missing field session |
| L0376 | `internal/mobilebackup/encrypted_archive.go:21:8` | mobilebackup.Format1Archive is missing fields Archive, CiphertextBytes, ContainerBytes, TrailingBytes |
| L0377 | `internal/mobilebackup/header.go:85:12` | mobilebackup.Header is missing field TotalBytes |
| L0378 | `internal/mobilebackup/history_page.go:59:12` | mobilebackup.SQLiteCursor is missing fields Descending, Snapshot |
| L0379 | `internal/mobilebackup/history_page.go:74:12` | domain.MobileHistorySnapshot is missing field ControlPreludeComplete |
| L0380 | `internal/mobilebackup/mentions.go:24:7` | mobilebackup.Mention is missing fields Type, UID, Position, Length, UnsupportedFields |
| L0381 | `internal/mobilebackup/message_conversion.go:26:19` | domain.ExpiringHistoryRecord is missing fields Message, ExpiresAtMS |
| L0382 | `internal/mobilebackup/message_conversion.go:31:8` | mobilebackup.ConvertedArchivePage is missing fields Records, Recalls, Expired, UnsupportedContent, UnresolvedQuotes, UnresolvedMentions, UnsupportedContentReasons, UnsupportedContentKinds |
| L0383 | `internal/mobilebackup/message_conversion.go:71:12` | mobilebackup.ArchivePageInspection is missing fields OwnRecallCandidates, UnsupportedContentReasons, UnsupportedContentKinds |
| L0384 | `internal/mobilebackup/message_conversion.go:195:14` | domain.Message is missing fields FirstIncoming, ConversationName, GroupID, SenderName, ReplyTo, Source, TextTruncated, TextResourceURI |
| L0385 | `internal/mobilebackup/page_expiry.go:27:11` | mobilebackup.PreparedRow is missing fields Row, SenderID, Direction, Kind, Metadata, ExpiresMS, ExpiryDeclared, QuoteExpired |
| L0386 | `internal/mobilebackup/page_expiry.go:39:14` | mobilebackup.QuoteValue is missing fields Present, Bytes |
| L0387 | `internal/mobilebackup/page_expiry.go:45:11` | mobilebackup.PreparedRow is missing fields Row, SenderID, Direction, Kind, Metadata, ExpiresMS, ExpiryDeclared, QuoteExpired |
| L0388 | `internal/mobilebackup/page_walk.go:44:14` | mobilebackup.ArchiveWalkSummary is missing fields Pages, Examined, Rejected, Candidates, ExpiredMessages, ExpiredQuotes, DeferredControls, UnsupportedTypes, MissingMetadata, InvalidMetadata, UnsupportedMetadataFields, SourceHasMore, StopReason |
| L0389 | `internal/mobilebackup/page_walk.go:58:13` | mobilebackup.ArchiveWalkSummary is missing field StopReason |
| L0390 | `internal/mobilebackup/prepared_page.go:38:14` | mobilebackup.PreparedArchiveCursor is missing fields requestID, fingerprint, account, examined, sqlite |
| L0391 | `internal/mobilebackup/prepared_page.go:40:8` | mobilebackup.PreparedArchivePage is missing fields Coverage, SourceWAL, SourceControls, controlsVerified, requestID, fingerprint, account, Candidates, Examined, Rejected, ExpiredMessages, ExpiredQuotes, SourceHasMore, BudgetExhausted, Next |
| L0392 | `internal/mobilebackup/quote.go:35:7` | mobilebackup.QuoteScalars is missing fields OwnerID, ClientMessageID, GlobalMessageID, MessageType, Timestamp, TTL |
| L0393 | `internal/mobilebackup/quote_fields.go:33:8` | mobilebackup.QuoteValue is missing fields Present, Bytes |
| L0394 | `internal/mobilebackup/quote_fields.go:35:7` | mobilebackup.Quote is missing fields Scalars, Message, Attachment, FromD, Status, UnsupportedFields |
| L0395 | `internal/mobilebackup/retained_archive.go:136:8` | mobilebackup.RetainedArchiveStore is missing field backup |
| L0396 | `internal/mobilebackup/retained_archive.go:138:12` | mobilebackup.SnapshotStore is missing field terminalHints |
| L0397 | `internal/mobilebackup/retained_archive.go:257:10` | mobilebackup.retainedMetadata is missing fields PreservedMS, SourceStoredBytes |
| L0398 | `internal/mobilebackup/retained_archive.go:352:9` | mobilebackup.RetainedArchiveManifest is missing fields HistoryComplete, ImportPerformed |
| L0399 | `internal/mobilebackup/retained_archive.go:500:13` | mobilebackup.AccountArchive is missing fields ownerAccount, retainedSourceID, archive, pairs |
| L0400 | `internal/mobilebackup/row_page.go:37:15` | mobilebackup.PreparedRow is missing fields Row, SenderID, Direction, Kind, Metadata, ExpiresMS, ExpiryDeclared, QuoteExpired |
| L0401 | `internal/mobilebackup/row_page.go:40:7` | mobilebackup.PreparedRowPage is missing fields Rows, OwnRecalls, Examined, DeferredControls, UnsupportedTypes, MissingMetadata, InvalidMetadata, UnsupportedMetadataFields |
| L0402 | `internal/mobilebackup/row_page.go:147:13` | mobilebackup.IdentityRequest is missing fields Direct, Groups |
| L0403 | `internal/mobilebackup/row_page.go:188:4` | mobilebackup.PreparedRow is missing fields SenderID, Direction, QuoteExpired |
| L0404 | `internal/mobilebackup/selected_fetch.go:25:8` | mobilebackup.SelectedArchive is missing fields requestID, ref, requestFingerprint, snapshotCreatedMS, snapshotExpiresMS, File, CiphertextBytes, ContainerBytes, TrailingBytes |
| L0405 | `internal/mobilebackup/selected_fetch.go:76:14` | mobilebackup.SelectedArchive is missing fields snapshotCreatedMS, snapshotExpiresMS |
| L0406 | `internal/mobilebackup/selected_fetch.go:85:33` | mobilebackup.ArchiveFile is missing fields Name, Data |
| L0407 | `internal/mobilebackup/snapshot.go:107:12` | mobilebackup.SnapshotStore is missing field terminalHints |
| L0408 | `internal/mobilebackup/snapshot_page.go:70:8` | mobilebackup.SnapshotPage is missing fields Records, Coverage, WALMode, UnsupportedMetadataFields, UnresolvedQuotes, UnresolvedMentions, Examined, Rejected, Expired, UnresolvedSenders, SourceInformation, SourceNativeExcluded, SourceRecallRows, SuppressedSource, Unsupported, HasMore, Next |
| L0409 | `internal/mobilebackup/snapshot_page.go:189:47` | mobilebackup.PreparedRow is missing fields SenderID, Direction, ExpiresMS, ExpiryDeclared, QuoteExpired |
| L0410 | `internal/mobilebackup/sqlite.go:81:15` | mobilebackup.SQLiteRow is missing fields SenderID, MessageID, ClientID, Text, TimestampMS, TTL, Type, Status, SourceRowID, BinNet |
| L0411 | `internal/mobilebackup/tlv.go:30:17` | mobilebackup.TLVField is missing fields Tag, Value |
| L0412 | `internal/mobilebackup/tlv.go:42:13` | mobilebackup.TLV is missing field Fields |
| L0413 | `internal/mobilebackup/xz.go:38:19` | xz.ReaderConfig is missing field SingleStream |
| L0414 | `internal/netpolicy/http.go:70:13` | net.Dialer is missing fields Deadline, LocalAddr, DualStack, FallbackDelay, KeepAlive, KeepAliveConfig, Resolver, Cancel, Control, ControlContext |
| L0415 | `internal/observability/observer.go:68:8` | observability.Observer is missing fields calls, response, upstream, transitions, queue, age, mu |
| L0416 | `internal/observability/observer.go:294:45` | trace.Link is missing field Attributes |
| L0417 | `internal/observability/provider.go:80:28` | otlptracehttp.RetryConfig is missing fields InitialInterval, MaxInterval, MaxElapsedTime |
| L0418 | `internal/observability/provider.go:90:29` | otlpmetrichttp.RetryConfig is missing fields InitialInterval, MaxInterval, MaxElapsedTime |
| L0419 | `internal/observability/provider.go:124:12` | observability.Owner is missing fields once, shutdownErr |
| L0420 | `internal/service/account_archive_control.go:44:9` | domain.Error is missing field Retryable |
| L0421 | `internal/service/account_archive_control.go:47:16` | domain.NextAction is missing fields Tool, Arguments |
| L0422 | `internal/service/archive_catalog.go:68:12` | service.archiveCatalogueCursor is missing field Next |
| L0423 | `internal/service/archive_messages.go:109:11` | domain.Error is missing fields Retryable, NextAction |
| L0424 | `internal/service/archive_messages.go:115:10` | domain.Error is missing fields Retryable, NextAction |
| L0425 | `internal/service/archive_messages.go:166:16` | domain.Error is missing fields Retryable, NextAction |
| L0426 | `internal/service/archive_messages.go:186:11` | service.archiveReadToken is missing fields Position, Target, Limit |
| L0427 | `internal/service/archive_port.go:19:32` | service.membershipPort is missing fields mu, current, sender, allowSend, recipients, lifecycle, snapshots, archives |
| L0428 | `internal/service/archive_read.go:62:10` | domain.Error is missing fields Retryable, NextAction |
| L0429 | `internal/service/bridge.go:54:27` | mcp.Implementation is missing fields Title, Description, WebsiteURL, Icons |
| L0430 | `internal/service/bridge.go:57:4` | mcp.StreamableClientTransport is missing fields OAuthHandler, MaxEventSize |
| L0431 | `internal/service/bridge.go:69:27` | mcp.Implementation is missing fields Title, Description, WebsiteURL, Icons |
| L0432 | `internal/service/bridge.go:76:34` | mcp.CallToolParams is missing fields Meta, InputResponses, RequestState |
| L0433 | `internal/service/bridge.go:80:37` | mcp.ReadResourceParams is missing fields Meta, InputResponses, RequestState |
| L0434 | `internal/service/bridge.go:94:29` | mcp.StdioTransport is missing field MaxLineLength |
| L0435 | `internal/service/bridge.go:146:14` | service.stdioCountingWriter is missing fields bytes, batch |
| L0436 | `internal/service/bridge.go:147:23` | mcp.IOTransport is missing field MaxLineLength |
| L0437 | `internal/service/bridge.go:151:10` | service.observedStdioConnection is missing fields writeMu, mu, closed |
| L0438 | `internal/service/cli.go:60:59` | slog.HandlerOptions is missing fields AddSource, ReplaceAttr |
| L0439 | `internal/service/mcp_reads.go:34:15` | toolruntime.Capabilities is missing fields Events, Archives |
| L0440 | `internal/service/mobile_backup.go:97:3` | domain.MobileIdentityRequest is missing field Groups |
| L0441 | `internal/service/mobile_backup.go:213:12` | mobilebackup.ArchiveWalkSummary is missing fields Pages, Examined, Rejected, Candidates, ExpiredMessages, ExpiredQuotes, DeferredControls, UnsupportedTypes, MissingMetadata, InvalidMetadata, UnsupportedMetadataFields, SourceHasMore, StopReason |
| L0442 | `internal/service/mobile_history.go:78:45` | domain.MobileIdentityRequest is missing field Groups |
| L0443 | `internal/service/mobile_ledger_control.go:92:10` | domain.Error is missing field Retryable |
| L0444 | `internal/service/mobile_ledger_control.go:95:16` | domain.NextAction is missing fields Tool, Arguments |
| L0445 | `internal/service/recall_diagnostic.go:39:12` | domain.Error is missing field Retryable |
| L0446 | `internal/service/recall_diagnostic.go:39:128` | domain.NextAction is missing fields Tool, Arguments |
| L0447 | `internal/service/recall_diagnostic.go:79:11` | service.recallReceipt is missing fields State, Status |
| L0448 | `internal/service/recall_diagnostic.go:174:9` | service.recallReceipt is missing field Status |
| L0449 | `internal/service/run.go:102:15` | domain.Error is missing field Retryable |
| L0450 | `internal/service/run.go:105:15` | domain.NextAction is missing fields Tool, Arguments |
| L0451 | `internal/service/run.go:183:58` | slog.HandlerOptions is missing fields AddSource, ReplaceAttr |
| L0452 | `internal/service/run.go:273:11` | service.membershipPort is missing fields mu, current, sender |
| L0453 | `internal/service/run.go:549:14` | domain.Error is missing fields Retryable, NextAction, Details |
| L0454 | `internal/storage/browse.go:57:7` | storage.cursor is missing fields Kind, Snapshot, At, Group, ID |
| L0455 | `internal/storage/conversation_views.go:80:27` | domain.NextAction is missing fields Tool, Arguments |
| L0456 | `internal/storage/conversation_views.go:116:7` | storage.<anonymous> is missing fields Kind, ID |
| L0457 | `internal/storage/diagnostics.go:21:7` | storage.DeliveryDiagnostics is missing fields ActiveSubscriptions, JournalPending, PendingPayloadBytes |
| L0458 | `internal/storage/diagnostics.go:33:32` | sql.TxOptions is missing field Isolation |
| L0459 | `internal/storage/events.go:297:10` | domain.Error is missing field Retryable |
| L0460 | `internal/storage/events.go:300:15` | domain.NextAction is missing fields Tool, Arguments |
| L0461 | `internal/storage/history.go:68:12` | storage.HistoryPageCounts is missing fields Inserted, Duplicates |
| L0462 | `internal/storage/history_expiry.go:80:12` | storage.HistoryPageCounts is missing fields Inserted, Duplicates |
| L0463 | `internal/storage/history_operations.go:210:12` | domain.HistoryImportStatus is missing fields MobileCoverage, PagesObserved, RecordsObserved, InsertedCount, DuplicateCount, OutOfIntervalCount, EarliestImportedAt, LatestImportedAt, SourceHasMore, IsFiltered, IsFilteredByPhase, IsFilteredByTimeJoin, IsOld, JoinTimestampMillis, HistoryComplete, StopReason |
| L0464 | `internal/storage/history_operations.go:252:9` | storage.HistoryOperation is missing fields Revision, WorkDuration, ReservedDuration |
| L0465 | `internal/storage/history_operations.go:638:9` | storage.HistoryOperation is missing fields Status, Cursor, Revision, WorkDuration, ReservedDuration |
| L0466 | `internal/storage/mobile_backup.go:152:7` | storage.MobileBackupAttempt is missing fields Revision, PublicKey |
| L0467 | `internal/storage/mobile_control_prelude.go:87:19` | domain.MobileHistoryCounts is missing fields Examined, Rejected, Expired, UnsupportedTypes, UnsupportedContent, MissingMetadata, InvalidMetadata, DeferredControls, OwnRecalls, UnknownMetadataFields, ExpiredQuotes, UnresolvedQuotes, UnresolvedMentions |
| L0468 | `internal/storage/mobile_control_prelude.go:116:30` | domain.MobileHistoryCoverage is missing field MobileHistoryCounts |
| L0469 | `internal/storage/mobile_history.go:307:14` | domain.MobileHistoryCoverage is missing fields MobileHistoryCounts, SourceRecalls, ControlPreludeComplete |
| L0470 | `internal/storage/mobile_history_acquisition.go:53:13` | domain.MobileBackupRequest is missing fields ArchiveScope, RetentionHours |
| L0471 | `internal/storage/read_authority.go:57:11` | domain.Error is missing field Retryable |
| L0472 | `internal/storage/read_authority.go:60:16` | domain.NextAction is missing fields Tool, Arguments |
| L0473 | `internal/storage/store.go:34:12` | domain.CollectionPolicy is missing field All |
| L0474 | `internal/storage/store.go:67:8` | storage.Store is missing fields key, directIngestion, groupIngestion |
| L0475 | `internal/storage/store.go:434:29` | domain.Error is missing field Retryable |
| L0476 | `internal/storage/store.go:437:16` | domain.NextAction is missing fields Tool, Arguments |
| L0477 | `internal/storage/store.go:563:7` | storage.cursor is missing fields Kind, Snapshot, At, Group, ID |
| L0478 | `internal/storage/views.go:64:16` | domain.NextAction is missing fields Tool, Arguments |
| L0479 | `internal/storage/views.go:72:7` | storage.<anonymous> is missing field Last |
| L0480 | `internal/storage/views.go:189:7` | domain.Coverage is missing fields CollectionStartedAt, EarliestStoredAt, LatestStoredAt, HistoryComplete, RetentionDays |
| L0481 | `internal/storage/views.go:396:27` | domain.NextAction is missing fields Tool, Arguments |
| L0482 | `internal/toolruntime/runtime.go:154:8` | toolruntime.Runtime is missing fields operations, view |
| L0483 | `internal/toolruntime/runtime.go:261:6` | toolsy.Chunk is missing fields CallID, ToolName, IsError, EmptyResult, Noop, Effects, Controls, Control, Progress, Envelope |
| L0484 | `internal/toolruntime/runtime.go:280:3` | toolsy.RegistryViewSpec is missing fields Policy, PolicyID |
| L0485 | `internal/toolruntime/runtime.go:308:10` | domain.Error is missing field Retryable |
| L0486 | `internal/toolruntime/runtime.go:311:15` | domain.NextAction is missing fields Tool, Arguments |
| L0487 | `internal/toolruntime/runtime.go:361:13` | toolsy.ToolInput is missing fields CallID, Attachments |
| L0488 | `internal/toolruntime/runtime.go:465:10` | toolruntime.Result is missing fields Acknowledgement, Subscriptions, Events, Directory, ConversationDetail, Browse, ArchiveSources, ArchiveDirectory, ArchiveBrowse, GroupSearch, ConversationSearch, GroupContext, ConversationContext, CollectorStatus, GroupDetail, GroupList, SendStatus, ImportStatus, JoinStatus, Invite |
| L0489 | `internal/zalo/adapter.go:54:38` | zcago.LoginQROption is missing fields UserAgent, Language, QRPath |
| L0490 | `internal/zalo/adapter.go:98:16` | domain.Error is missing field Retryable |
| L0491 | `internal/zalo/adapter.go:101:16` | domain.NextAction is missing fields Tool, Arguments |
| L0492 | `internal/zalo/adapter.go:130:16` | domain.NextAction is missing fields Tool, Arguments |
| L0493 | `internal/zalo/adapter.go:163:10` | zcago.Credentials is missing field Cookie |
| L0494 | `internal/zalo/adapter.go:191:29` | domain.Group is missing field CollectionEnabled |
| L0495 | `internal/zalo/adapter.go:208:9` | domain.Group is missing field CollectionEnabled |
| L0496 | `internal/zalo/adapter.go:221:21` | domain.Group is missing field CollectionEnabled |
| L0497 | `internal/zalo/adapter.go:320:9` | domain.Message is missing fields Direction, FirstIncoming, QuoteMetadata, ConversationName, GroupID, SenderName, Text, ReplyTo, TextTruncated, TextResourceURI |
| L0498 | `internal/zalo/cookies.go:27:10` | zalo.persistentJar is missing field mu |
| L0499 | `internal/zalo/history.go:118:10` | domain.HistoryPage is missing fields PhaseRequired, LimitedSnapshot, JoinTimestampMillis |
| L0500 | `internal/zalo/mobile_backup.go:203:6` | domain.MobileIdentityRequest is missing field Groups |
| L0501 | `internal/zalo/mobile_producer_counts.go:18:7` | zalo.mobileProducerCounts is missing fields Total, Threads |
| L0502 | `internal/zalo/preload_history.go:38:10` | domain.HistoryPage is missing fields PhaseRequired, HasMore, Cursor, IsFiltered, IsFilteredByPhase, IsFilteredByTimeJoin, IsOld, JoinTimestampMillis |
| L0503 | `internal/zalo/replay.go:21:42` | zalo.replayQueue is missing fields pages, messages, stopped |
| L0504 | `internal/zalo/replay.go:23:30` | zalo.replayQueue is missing fields pages, messages, stopped |
| L0505 | `internal/zalo/sending.go:16:13` | api.MessageContent is missing fields Style, Urgency, Quote, Mentions, Attachments, TTL |
| L0506 | `internal/zalo/sending.go:18:20` | api.SendMessageQuote is missing field PropertyExt |
| L0507 | `internal/zalo/sending.go:23:14` | model.Content is missing fields Attachment, DeletedContent, Other |
| L0508 | `tools/harness/internal/architecture/architecture.go:93:21` | syscall.SysProcAttr is missing fields Chroot, Credential, Ptrace, Setsid, Setctty, Noctty, Ctty, Foreground, Pgid |
| L0509 | `tools/harness/internal/architecture/architecture.go:140:7` | architecture.Result is missing field Report |
| L0510 | `tools/harness/internal/architecture/architecture.go:417:22` | architecture.Control is missing field Result |
| L0511 | `tools/harness/internal/performance/process.go:39:21` | syscall.SysProcAttr is missing fields Chroot, Credential, Ptrace, Setsid, Setctty, Noctty, Ctty, Foreground, Pgid |

### forbidigo (4)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L0515–L0517: CLI печатает результат пользователю — локальный nolint:forbidigo с причиной CLI output, не заменять на logger (сломает stdout/JSON). L0518: progress performance harness допустим только в диагностическом потоке; сохранить согласованный формат stdout либо перенести progress в stderr, если stdout занят machine output. Проверять ошибки записи отдельно.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0515 | `cmd/zl-mcp-eval/main.go:186:2` | use of &#96;fmt.Printf&#96; forbidden by pattern &#96;^(fmt\.Print(\|f\|ln)\|print\|println)$&#96; |
| L0516 | `tools/harness/cmd/harness/main.go:52:2` | use of &#96;fmt.Println&#96; forbidden by pattern &#96;^(fmt\.Print(\|f\|ln)\|print\|println)$&#96; |
| L0517 | `tools/harness/cmd/harness/main.go:101:2` | use of &#96;fmt.Println&#96; forbidden by pattern &#96;^(fmt\.Print(\|f\|ln)\|print\|println)$&#96; |
| L0518 | `tools/harness/internal/performance/bench.go:170:4` | use of &#96;fmt.Printf&#96; forbidden by pattern &#96;^(fmt\.Print(\|f\|ln)\|print\|println)$&#96; |

### funcorder (4)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L0519–L0522: перенести constructors перед methods без изменения тел и signatures.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0519 | `internal/mobilebackup/download.go:30:1` | constructor "NewDownloader" for struct "Downloader" should be placed before struct method "Ready" |
| L0520 | `internal/mobilebackup/retained_archive.go:91:1` | constructor "NewRetainedArchiveStore" for struct "RetainedArchiveStore" should be placed before struct method "String" |
| L0521 | `internal/mobilebackup/retained_archive.go:96:1` | constructor "NewArchiveLibraryStore" for struct "RetainedArchiveStore" should be placed before struct method "String" |
| L0522 | `internal/mobilebackup/snapshot.go:71:1` | constructor "NewSnapshotStore" for struct "SnapshotStore" should be placed before struct method "lock" |

### funlen (67)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Применить общий план complexity из cyclop. Не дробить транзакцию/cleanup scope ради числа строк. Разрешить точечный nolint:funlen для цельного сценария или declarative registry с объяснением; длинные orchestration functions разделять по этапам.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0523 | `cmd/zl-mcp-eval/main.go:16:6` | Function 'run' has too many statements (107 > 50) |
| L0524 | `internal/collector/contact_profiles.go:26:6` | Function 'refreshContactProfiles' has too many statements (60 > 50) |
| L0525 | `internal/collector/contacts.go:30:6` | Function 'refreshContacts' has too many statements (53 > 50) |
| L0526 | `internal/collector/join.go:213:23` | Function 'Start' has too many statements (71 > 50) |
| L0527 | `internal/collector/preload_probe.go:13:6` | Function 'probePreload' has too many statements (58 > 50) |
| L0528 | `internal/collector/run.go:55:6` | Function 'runSession' has too many statements (121 > 50) |
| L0529 | `internal/config/config.go:47:6` | Function 'Load' has too many statements (69 > 50) |
| L0530 | `internal/domain/history.go:59:6` | Function 'NewHistoryPageFailure' has too many statements (93 > 50) |
| L0531 | `internal/evalfixture/fixture.go:133:6` | Function 'New' has too many statements (115 > 50) |
| L0532 | `internal/evaluation/benchmark.go:39:6` | Function 'Measure' has too many statements (64 > 50) |
| L0533 | `internal/evaluation/checkpoints.go:149:6` | Function 'GradeCheckpoint' has too many statements (194 > 50) |
| L0534 | `internal/evaluation/grader.go:19:6` | Function 'Grade' has too many statements (113 > 50) |
| L0535 | `internal/evaluation/runner.go:135:6` | Function 'Run' has too many statements (101 > 50) |
| L0536 | `internal/evaluation/scenarios.go:22:6` | Function 'RunCase' has too many statements (217 > 50) |
| L0537 | `internal/events/subscriptions.go:132:31` | Function 'Call' is too long (163 > 100) |
| L0538 | `internal/events/worker.go:94:18` | Function 'deliverClaimed' has too many statements (53 > 50) |
| L0539 | `internal/historyimport/mobile_worker.go:101:6` | Function 'runMobileOperationObserved' has too many statements (61 > 50) |
| L0540 | `internal/historyimport/worker.go:68:6` | Function 'runOperationObserved' has too many statements (79 > 50) |
| L0541 | `internal/mcpserver/conversations.go:17:19` | Function 'addConversationResources' is too long (132 > 100) |
| L0542 | `internal/mcpserver/http.go:110:23` | Function 'ServeHTTP' has too many statements (109 > 50) |
| L0543 | `internal/mcpserver/server.go:61:6` | Function 'New' is too long (163 > 100) |
| L0544 | `internal/mcpserver/server.go:239:19` | Function 'call' has too many statements (99 > 50) |
| L0545 | `internal/messaging/manager.go:38:19` | Function 'Call' has too many statements (57 > 50) |
| L0546 | `internal/mobilebackup/account_metadata.go:68:6` | Function 'inspectAccountMetadata' has too many statements (52 > 50) |
| L0547 | `internal/mobilebackup/attachment.go:44:6` | Function 'ParseAttachment' has too many statements (52 > 50) |
| L0548 | `internal/mobilebackup/control_scan.go:30:6` | Function 'ReadOwnDirectRecallSet' is too long (126 > 100) |
| L0549 | `internal/mobilebackup/history_page.go:17:25` | Function 'ReadHistoryPage' has too many statements (59 > 50) |
| L0550 | `internal/mobilebackup/message_conversion.go:91:6` | Function 'convertArchiveCandidates' has too many statements (63 > 50) |
| L0551 | `internal/mobilebackup/prepared_page.go:44:6` | Function 'ReadPreparedArchivePage' has too many statements (51 > 50) |
| L0552 | `internal/mobilebackup/retained_archive.go:448:32` | Function 'decode' has too many statements (54 > 50) |
| L0553 | `internal/mobilebackup/row_page.go:81:6` | Function 'PrepareRowPage' has too many statements (107 > 50) |
| L0554 | `internal/mobilebackup/snapshot.go:278:25` | Function 'Save' has too many statements (59 > 50) |
| L0555 | `internal/mobilebackup/snapshot_page.go:76:25` | Function 'ReadSnapshotPage' has too many statements (100 > 50) |
| L0556 | `internal/mobilebackup/snapshot_terminal.go:15:25` | Function 'CleanupTerminal' has too many statements (71 > 50) |
| L0557 | `internal/mobilebackup/sqlite.go:169:6` | Function 'readSQLitePageMode' has too many statements (200 > 50) |
| L0558 | `internal/mobilebackup/xz.go:53:6` | Function 'inspectXZ' has too many statements (74 > 50) |
| L0559 | `internal/service/account_archive.go:30:26` | Function 'captureAccountArchiveWithDownloader' has too many statements (55 > 50) |
| L0560 | `internal/service/account_archive_control.go:24:26` | Function 'accountArchiveControl' has too many statements (99 > 50) |
| L0561 | `internal/service/archive_catalog.go:43:26` | Function 'archiveConversations' has too many statements (52 > 50) |
| L0562 | `internal/service/archive_messages.go:156:26` | Function 'archiveMessages' has too many statements (68 > 50) |
| L0563 | `internal/service/cli.go:28:6` | Function 'RunCLI' has too many statements (94 > 50) |
| L0564 | `internal/service/cli_mobile_ledger.go:29:6` | Function 'mobileLedgerArguments' has too many statements (56 > 50) |
| L0565 | `internal/service/mobile_archive_probe.go:42:26` | Function 'probeMobileArchiveComparisonWithDownloader' is too long (184 > 100) |
| L0566 | `internal/service/mobile_history.go:32:26` | Function 'withHistorySession' has too many statements (51 > 50) |
| L0567 | `internal/service/mobile_ledger_control.go:72:26` | Function 'mobileLedgerControl' has too many statements (62 > 50) |
| L0568 | `internal/service/recall_diagnostic.go:77:26` | Function 'probeRecall' has too many statements (88 > 50) |
| L0569 | `internal/service/run.go:154:6` | Function 'runConfigured' has too many statements (145 > 50) |
| L0570 | `internal/service/run.go:501:6` | Function 'collectSession' has too many statements (51 > 50) |
| L0571 | `internal/storage/browse.go:18:17` | Function 'Browse' has too many statements (106 > 50) |
| L0572 | `internal/storage/conversation_views.go:102:17` | Function 'Conversations' has too many statements (69 > 50) |
| L0573 | `internal/storage/deliveries.go:61:17` | Function 'FanoutProfileEvents' has too many statements (89 > 50) |
| L0574 | `internal/storage/event_journal.go:138:17` | Function 'ReadSubscriptionEvents' has too many statements (61 > 50) |
| L0575 | `internal/storage/history_operations.go:430:17` | Function 'commitHistoryRecords' has too many statements (73 > 50) |
| L0576 | `internal/storage/mobile_history.go:158:17` | Function 'CommitMobileHistoryPage' has too many statements (115 > 50) |
| L0577 | `internal/storage/store.go:258:17` | Function 'putMessageTx' is too long (108 > 100) |
| L0578 | `internal/storage/store.go:594:17` | Function 'searchPage' has too many statements (82 > 50) |
| L0579 | `internal/storage/views.go:20:17` | Function 'Groups' has too many statements (59 > 50) |
| L0580 | `internal/storage/views.go:283:17` | Function 'context' has too many statements (67 > 50) |
| L0581 | `internal/toolruntime/runtime.go:144:6` | Function 'New' has too many statements (60 > 50) |
| L0582 | `internal/toolruntime/runtime.go:464:19` | Function 'dispatch' has too many statements (102 > 50) |
| L0583 | `internal/zalo/mobile_backup.go:60:6` | Function 'receiveMobileBackupOffer' is too long (174 > 100) |
| L0584 | `internal/zalo/preload.go:29:6` | Function 'readConversationPreload' has too many statements (73 > 50) |
| L0585 | `tools/harness/internal/architecture/architecture.go:449:6` | Function 'Check' has too many statements (59 > 50) |
| L0586 | `tools/harness/internal/performance/bench.go:75:6` | Function 'Bench' is too long (140 > 100) |
| L0587 | `tools/harness/internal/performance/compare.go:8:6` | Function 'validate' has too many statements (90 > 50) |
| L0588 | `tools/harness/internal/performance/compare.go:150:6` | Function 'Compare' has too many statements (82 > 50) |
| L0589 | `tools/harness/internal/performance/process.go:18:6` | Function 'process' has too many statements (75 > 50) |

### gochecknoglobals (12)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Неизменяемые package-private registries/allowlists (toolAliases, descriptions, blockedPrefixes, toolNames, readNames, cases) оставить с nolint:gochecknoglobals и причиной read-only after initialization, проверив отсутствие writers. Exported Kinds/P95Limits сделать private с accessor/copy, чтобы внешний код не менял бюджет. workerOnce/workerPath/workerErr требуют общего владельца lifetime: TestMain build+cleanup или scoped fixture; sync.Once сам по себе не обеспечивает удаление каталога.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0590 | `docs/contracts/contracts.go:19:5` | toolAliases is a global variable |
| L0591 | `internal/mcpserver/sdk_identity_test.go:25:5` | sdkIdentityCases is a global variable |
| L0592 | `internal/mcpserver/server.go:26:5` | descriptions is a global variable |
| L0593 | `internal/netpolicy/http.go:14:5` | blockedPrefixes is a global variable |
| L0594 | `internal/observability/observer.go:22:5` | toolNames is a global variable |
| L0595 | `internal/toolruntime/runtime.go:24:5` | readNames is a global variable |
| L0596 | `tools/harness/internal/architecture/architecture.go:285:5` | cases is a global variable |
| L0597 | `tools/harness/internal/performance/json.go:19:5` | Kinds is a global variable |
| L0598 | `tools/harness/internal/performance/json.go:24:5` | P95Limits is a global variable |
| L0599 | `tools/harness/internal/performance/performance_test.go:199:5` | workerOnce is a global variable |
| L0600 | `tools/harness/internal/performance/performance_test.go:200:5` | workerPath is a global variable |
| L0601 | `tools/harness/internal/performance/performance_test.go:201:5` | workerErr is a global variable |

### gocognit (205)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Production: guard clauses и функции по предметным этапам; в GradeCheckpoint/Grade разделить graders по группам assertions, сохранив полный набор и counters. Tests: table-driven subtests только для независимых случаев; shared restart/transaction scenario сохранять цельным с локальным nolint:gocognit и причиной. Не выносить assertions в helpers без t.Helper и понятного имени.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0602 | `cmd/zl-mcp-eval/main.go:16:1` | cognitive complexity 58 of func &#96;run&#96; is high (> 20) |
| L0603 | `cmd/zl-mcp-eval/main_test.go:31:1` | cognitive complexity 25 of func &#96;TestCLIExitCodesAndFailedReports&#96; is high (> 20) |
| L0604 | `cmd/zl-mcp-eval/main_test.go:130:1` | cognitive complexity 24 of func &#96;TestOfflineCheckpointCLI&#96; is high (> 20) |
| L0605 | `internal/adapters/operations/read_test.go:53:1` | cognitive complexity 28 of func &#96;TestReadCapabilitiesPreserveNativeProjectionAndCannotDispatchCommands&#96; is high (> 20) |
| L0606 | `internal/adapters/store/catalog_test.go:33:1` | cognitive complexity 28 of func &#96;TestCatalogPortsPreserveNativeNullableDiagnosticsAndIdentity&#96; is high (> 20) |
| L0607 | `internal/adapters/store/conversation_load_test.go:16:1` | cognitive complexity 40 of func &#96;BenchmarkManyConversations&#96; is high (> 20) |
| L0608 | `internal/app/eventaccess/journal_test.go:11:1` | cognitive complexity 28 of func &#96;TestJournalProfileProjectionPreservesNullsAndOpaqueValues&#96; is high (> 20) |
| L0609 | `internal/collector/contact_profiles.go:26:1` | cognitive complexity 40 of func &#96;refreshContactProfiles&#96; is high (> 20) |
| L0610 | `internal/collector/contact_profiles_test.go:36:1` | cognitive complexity 21 of func &#96;TestDirectoryWorkerProfileAuthenticationBoundary&#96; is high (> 20) |
| L0611 | `internal/collector/contact_profiles_test.go:158:1` | cognitive complexity 51 of func &#96;TestProfileRefreshMissingForeignAndBatchLimits&#96; is high (> 20) |
| L0612 | `internal/collector/contacts.go:30:1` | cognitive complexity 33 of func &#96;refreshContacts&#96; is high (> 20) |
| L0613 | `internal/collector/contacts_test.go:20:1` | cognitive complexity 50 of func &#96;TestContactsRefreshBoundsAndPartialRecords&#96; is high (> 20) |
| L0614 | `internal/collector/join.go:213:1` | cognitive complexity 31 of func &#96;(*JoinManager).Start&#96; is high (> 20) |
| L0615 | `internal/collector/join.go:381:1` | cognitive complexity 29 of func &#96;(*JoinManager).execute&#96; is high (> 20) |
| L0616 | `internal/collector/preload_catalog_test.go:22:1` | cognitive complexity 22 of func &#96;TestPreloadRefreshOnlyMergesPermittedMetadataAndKeepsFailureEvidenceSafe&#96; is high (> 20) |
| L0617 | `internal/collector/rpc_test.go:16:1` | cognitive complexity 39 of func &#96;TestControlWorkflowContracts&#96; is high (> 20) |
| L0618 | `internal/collector/run.go:55:1` | cognitive complexity 71 of func &#96;runSession&#96; is high (> 20) |
| L0619 | `internal/collector/run.go:270:1` | cognitive complexity 48 of func &#96;ControlHandler&#96; is high (> 20) |
| L0620 | `internal/collector/run_test.go:164:1` | cognitive complexity 58 of func &#96;TestConnectedListenerResolvesOnlyListenerFailure&#96; is high (> 20) |
| L0621 | `internal/config/config.go:47:1` | cognitive complexity 33 of func &#96;Load&#96; is high (> 20) |
| L0622 | `internal/evalfixture/fixture.go:133:1` | cognitive complexity 78 of func &#96;New&#96; is high (> 20) |
| L0623 | `internal/evalfixture/fixture_test.go:137:1` | cognitive complexity 23 of func &#96;TestSyntheticMCPApprovedJoinKeepsOneMutation&#96; is high (> 20) |
| L0624 | `internal/evaluation/checkpoints.go:149:1` | cognitive complexity 123 of func &#96;GradeCheckpoint&#96; is high (> 20) |
| L0625 | `internal/evaluation/checkpoints.go:415:1` | cognitive complexity 23 of func &#96;result&#96; is high (> 20) |
| L0626 | `internal/evaluation/checkpoints_test.go:105:1` | cognitive complexity 29 of func &#96;TestCheckpointDatasetCoverageAndAllCases&#96; is high (> 20) |
| L0627 | `internal/evaluation/checkpoints_test.go:171:1` | cognitive complexity 34 of func &#96;TestCheckpointUniqueNegativeChecks&#96; is high (> 20) |
| L0628 | `internal/evaluation/grader.go:19:1` | cognitive complexity 78 of func &#96;Grade&#96; is high (> 20) |
| L0629 | `internal/evaluation/process_test.go:62:1` | cognitive complexity 27 of func &#96;TestWatchdogTerminatesDescendantsAfterNormalExitAndTimeout&#96; is high (> 20) |
| L0630 | `internal/evaluation/process_test.go:107:1` | cognitive complexity 28 of func &#96;TestExternalWatchdogAndOutputBounds&#96; is high (> 20) |
| L0631 | `internal/evaluation/runner.go:45:1` | cognitive complexity 25 of func &#96;loadDataset&#96; is high (> 20) |
| L0632 | `internal/evaluation/runner.go:135:1` | cognitive complexity 99 of func &#96;Run&#96; is high (> 20) |
| L0633 | `internal/evaluation/scenarios.go:22:1` | cognitive complexity 93 of func &#96;RunCase&#96; is high (> 20) |
| L0634 | `internal/events/conversation_test.go:20:1` | cognitive complexity 50 of func &#96;TestConversationScopesSignedDeliveryAndCancellation&#96; is high (> 20) |
| L0635 | `internal/events/incoming_test.go:20:1` | cognitive complexity 40 of func &#96;TestFilteredProfileCanonicalIdentitySignedPayloadAndCancel&#96; is high (> 20) |
| L0636 | `internal/events/message_test.go:15:1` | cognitive complexity 22 of func &#96;TestMessageEventPreservesIDsAndProvidesFullTextReference&#96; is high (> 20) |
| L0637 | `internal/events/message_test.go:80:1` | cognitive complexity 28 of func &#96;TestConversationEventUnicodeBoundaryAndStableTypedURI&#96; is high (> 20) |
| L0638 | `internal/events/subscriptions.go:132:1` | cognitive complexity 43 of func &#96;(*SubscriptionManager).Call&#96; is high (> 20) |
| L0639 | `internal/events/webhook_test.go:22:1` | cognitive complexity 28 of func &#96;TestVerificationSignatureAndChallenge&#96; is high (> 20) |
| L0640 | `internal/events/worker.go:94:1` | cognitive complexity 29 of func &#96;(*Worker).deliverClaimed&#96; is high (> 20) |
| L0641 | `internal/events/worker.go:188:1` | cognitive complexity 43 of func &#96;(*Worker).Run&#96; is high (> 20) |
| L0642 | `internal/events/worker_test.go:219:1` | cognitive complexity 40 of func &#96;TestBoundedDiagnosticsExcludePrivateContentInEverySink&#96; is high (> 20) |
| L0643 | `internal/historyimport/manager.go:28:1` | cognitive complexity 22 of func &#96;(*Manager).Call&#96; is high (> 20) |
| L0644 | `internal/historyimport/mcp_test.go:27:1` | cognitive complexity 29 of func &#96;TestMCPHistoryImportDiscoveryStatusAndReads&#96; is high (> 20) |
| L0645 | `internal/historyimport/mobile_worker.go:38:1` | cognitive complexity 40 of func &#96;RunMobile&#96; is high (> 20) |
| L0646 | `internal/historyimport/mobile_worker.go:101:1` | cognitive complexity 116 of func &#96;runMobileOperationObserved&#96; is high (> 20) |
| L0647 | `internal/historyimport/worker.go:68:1` | cognitive complexity 64 of func &#96;runOperationObserved&#96; is high (> 20) |
| L0648 | `internal/logging/file_test.go:12:1` | cognitive complexity 21 of func &#96;TestRotationPreservesSizeArchiveCountAndPrivacy&#96; is high (> 20) |
| L0649 | `internal/mcpserver/budget_error_test.go:18:1` | cognitive complexity 26 of func &#96;TestOversizedRecordAndCoverageReturnActionableErrors&#96; is high (> 20) |
| L0650 | `internal/mcpserver/catalog_ports_test.go:25:1` | cognitive complexity 23 of func &#96;TestCatalogToolsAndStaticResourcesShareSingleNativeAdmission&#96; is high (> 20) |
| L0651 | `internal/mcpserver/conversation_events_http_test.go:34:1` | cognitive complexity 50 of func &#96;testConversationProfileTLS&#96; is high (> 20) |
| L0652 | `internal/mcpserver/conversations.go:17:1` | cognitive complexity 50 of func &#96;(*Service).addConversationResources&#96; is high (> 20) |
| L0653 | `internal/mcpserver/conversations_test.go:17:1` | cognitive complexity 36 of func &#96;TestConversationMCPSearchContextCatalogueAndFullText&#96; is high (> 20) |
| L0654 | `internal/mcpserver/final_audit_test.go:43:1` | cognitive complexity 25 of func &#96;TestFinalAuditIncludesPreflightAndPostExecutionFailures&#96; is high (> 20) |
| L0655 | `internal/mcpserver/http.go:110:1` | cognitive complexity 56 of func &#96;(*HTTPService).ServeHTTP&#96; is high (> 20) |
| L0656 | `internal/mcpserver/http_test.go:77:1` | cognitive complexity 30 of func &#96;TestHTTPEventsLifecycleWithSignedTLSReceiverAndFullText&#96; is high (> 20) |
| L0657 | `internal/mcpserver/http_test.go:291:1` | cognitive complexity 26 of func &#96;TestEventsHTTPTransportGuards&#96; is high (> 20) |
| L0658 | `internal/mcpserver/migration_baseline_test.go:56:1` | cognitive complexity 49 of func &#96;TestMigrationCatalogBaseline&#96; is high (> 20) |
| L0659 | `internal/mcpserver/migration_inputs_test.go:20:1` | cognitive complexity 31 of func &#96;TestMigrationInputCharacterization&#96; is high (> 20) |
| L0660 | `internal/mcpserver/migration_wire_test.go:16:1` | cognitive complexity 22 of func &#96;TestMigrationJSONRPCIdentityBaseline&#96; is high (> 20) |
| L0661 | `internal/mcpserver/pagination_test.go:18:1` | cognitive complexity 48 of func &#96;TestLargeWirePagesKeepAllResultsAndSearchSnapshot&#96; is high (> 20) |
| L0662 | `internal/mcpserver/sdk_identity_test.go:120:1` | cognitive complexity 23 of func &#96;TestSDK01CodecIdentityRequestsAndResponses&#96; is high (> 20) |
| L0663 | `internal/mcpserver/sdk_identity_test.go:152:1` | cognitive complexity 37 of func &#96;TestSDK01HTTPWrappersPreserveEnvelopeIdentity&#96; is high (> 20) |
| L0664 | `internal/mcpserver/sdk_lifecycle_test.go:273:1` | cognitive complexity 27 of func &#96;TestSDK01LostReplyRetainsDurableUUIDAndNeverResends&#96; is high (> 20) |
| L0665 | `internal/mcpserver/sdk_stdio_identity_test.go:47:1` | cognitive complexity 23 of func &#96;sdkStartRawBridge&#96; is high (> 20) |
| L0666 | `internal/mcpserver/server.go:61:1` | cognitive complexity 39 of func &#96;New&#96; is high (> 20) |
| L0667 | `internal/mcpserver/server.go:239:1` | cognitive complexity 65 of func &#96;(*Service).call&#96; is high (> 20) |
| L0668 | `internal/mcpserver/server_test.go:17:1` | cognitive complexity 23 of func &#96;TestMCPDiscoverySearchContextAndResourceAuthorization&#96; is high (> 20) |
| L0669 | `internal/mcpserver/telemetry_test.go:62:1` | cognitive complexity 22 of func &#96;TestHTTPResponseObservationCountsActualSDKWireBytes&#96; is high (> 20) |
| L0670 | `internal/messaging/manager.go:38:1` | cognitive complexity 45 of func &#96;(*Manager).Call&#96; is high (> 20) |
| L0671 | `internal/messaging/subprocess_restart_test.go:29:1` | cognitive complexity 28 of func &#96;TestCommandOwnerCrashHelper&#96; is high (> 20) |
| L0672 | `internal/mobilebackup/account_inspection.go:48:1` | cognitive complexity 41 of func &#96;(AccountArchive).InspectCoverageWithMetadata&#96; is high (> 20) |
| L0673 | `internal/mobilebackup/account_metadata.go:68:1` | cognitive complexity 42 of func &#96;inspectAccountMetadata&#96; is high (> 20) |
| L0674 | `internal/mobilebackup/archive_restore_test.go:103:1` | cognitive complexity 38 of func &#96;TestRestoreBackupRejectsUnsafeInputsWithoutPublishing&#96; is high (> 20) |
| L0675 | `internal/mobilebackup/attachment_test.go:20:1` | cognitive complexity 22 of func &#96;TestAttachmentNativeFieldsPresenceOwnershipAndRedaction&#96; is high (> 20) |
| L0676 | `internal/mobilebackup/binnet.go:33:1` | cognitive complexity 23 of func &#96;ParseBinNet&#96; is high (> 20) |
| L0677 | `internal/mobilebackup/blocks.go:25:1` | cognitive complexity 22 of func &#96;decryptFormat1&#96; is high (> 20) |
| L0678 | `internal/mobilebackup/control_scan.go:30:1` | cognitive complexity 75 of func &#96;ReadOwnDirectRecallSet&#96; is high (> 20) |
| L0679 | `internal/mobilebackup/control_scan_test.go:124:1` | cognitive complexity 26 of func &#96;TestWholeSourceControlScanReturnsNoPrefixOnInvalidSource&#96; is high (> 20) |
| L0680 | `internal/mobilebackup/download_test.go:35:1` | cognitive complexity 23 of func &#96;TestDownloadExactLengthAndFailureBounds&#96; is high (> 20) |
| L0681 | `internal/mobilebackup/history_page.go:17:1` | cognitive complexity 37 of func &#96;(*SnapshotStore).ReadHistoryPage&#96; is high (> 20) |
| L0682 | `internal/mobilebackup/history_page_test.go:73:1` | cognitive complexity 38 of func &#96;TestSnapshotReaderConverterJournalResumeTogether&#96; is high (> 20) |
| L0683 | `internal/mobilebackup/history_page_test.go:205:1` | cognitive complexity 39 of func &#96;TestSnapshotHistoryPageRejectsBindingAndUnsafeSources&#96; is high (> 20) |
| L0684 | `internal/mobilebackup/history_worker_test.go:476:1` | cognitive complexity 76 of func &#96;TestMobileWorkerLossAndCancellationDoNotRedispatchOrImportPrefixes&#96; is high (> 20) |
| L0685 | `internal/mobilebackup/history_worker_test.go:582:1` | cognitive complexity 29 of func &#96;TestMobileWorkerWholeControlPreludeResumesWithoutRedispatch&#96; is high (> 20) |
| L0686 | `internal/mobilebackup/id_diagnostics.go:9:1` | cognitive complexity 22 of func &#96;rejectedMessageIDShape&#96; is high (> 20) |
| L0687 | `internal/mobilebackup/identities.go:76:1` | cognitive complexity 40 of func &#96;DecodeIdentities&#96; is high (> 20) |
| L0688 | `internal/mobilebackup/message_conversion.go:91:1` | cognitive complexity 43 of func &#96;convertArchiveCandidates&#96; is high (> 20) |
| L0689 | `internal/mobilebackup/message_conversion_test.go:68:1` | cognitive complexity 46 of func &#96;TestMobileRichTextProjectionBoundaries&#96; is high (> 20) |
| L0690 | `internal/mobilebackup/message_conversion_test.go:361:1` | cognitive complexity 33 of func &#96;TestMobilePlainAttachmentFallbackKeepsAdmissionGates&#96; is high (> 20) |
| L0691 | `internal/mobilebackup/native_binary_test.go:18:1` | cognitive complexity 77 of func &#96;TestNativeBinaryTerminalStartupAcceptance&#96; is high (> 20) |
| L0692 | `internal/mobilebackup/offer_runner.go:18:1` | cognitive complexity 24 of func &#96;RunPreparedOffer&#96; is high (> 20) |
| L0693 | `internal/mobilebackup/offer_runner_test.go:97:1` | cognitive complexity 33 of func &#96;TestPreparedOfferFailuresCloseEvidenceAndDiscardOffer&#96; is high (> 20) |
| L0694 | `internal/mobilebackup/own_recall_test.go:72:1` | cognitive complexity 44 of func &#96;TestOwnRecallClassificationScopeAndMappingFailures&#96; is high (> 20) |
| L0695 | `internal/mobilebackup/page_walk_test.go:64:1` | cognitive complexity 32 of func &#96;TestArchiveWalkLimitsAndFailure&#96; is high (> 20) |
| L0696 | `internal/mobilebackup/prepared_page.go:44:1` | cognitive complexity 23 of func &#96;ReadPreparedArchivePage&#96; is high (> 20) |
| L0697 | `internal/mobilebackup/quote.go:37:1` | cognitive complexity 25 of func &#96;ParseQuoteScalars&#96; is high (> 20) |
| L0698 | `internal/mobilebackup/recall_history_test.go:14:1` | cognitive complexity 39 of func &#96;TestRecallSnapshotConverterJournalResumeTogether&#96; is high (> 20) |
| L0699 | `internal/mobilebackup/retained_archive.go:448:1` | cognitive complexity 26 of func &#96;(*RetainedArchiveStore).decode&#96; is high (> 20) |
| L0700 | `internal/mobilebackup/retained_archive.go:538:1` | cognitive complexity 31 of func &#96;(*RetainedArchiveStore).inventory&#96; is high (> 20) |
| L0701 | `internal/mobilebackup/retained_archive_test.go:172:1` | cognitive complexity 29 of func &#96;TestRetainedArchiveExpiryRemovalAndSpentIDs&#96; is high (> 20) |
| L0702 | `internal/mobilebackup/retained_archive_test.go:223:1` | cognitive complexity 41 of func &#96;TestRetainedArchiveTamperingKeyLossAndUnsafeArtifacts&#96; is high (> 20) |
| L0703 | `internal/mobilebackup/row_page.go:81:1` | cognitive complexity 72 of func &#96;PrepareRowPage&#96; is high (> 20) |
| L0704 | `internal/mobilebackup/session_download_test.go:22:1` | cognitive complexity 36 of func &#96;TestSessionDownloaderPreservesValidationAndBodyBounds&#96; is high (> 20) |
| L0705 | `internal/mobilebackup/snapshot.go:193:1` | cognitive complexity 24 of func &#96;snapshotKey&#96; is high (> 20) |
| L0706 | `internal/mobilebackup/snapshot.go:278:1` | cognitive complexity 25 of func &#96;(*SnapshotStore).Save&#96; is high (> 20) |
| L0707 | `internal/mobilebackup/snapshot.go:506:1` | cognitive complexity 29 of func &#96;(*SnapshotStore).sweep&#96; is high (> 20) |
| L0708 | `internal/mobilebackup/snapshot_information_test.go:112:1` | cognitive complexity 21 of func &#96;TestSnapshotNativeExclusionAndUnclassifiedControlGuard&#96; is high (> 20) |
| L0709 | `internal/mobilebackup/snapshot_page.go:76:1` | cognitive complexity 68 of func &#96;(AccountArchive).ReadSnapshotPage&#96; is high (> 20) |
| L0710 | `internal/mobilebackup/snapshot_page_test.go:18:1` | cognitive complexity 33 of func &#96;snapshotProjectionFixture&#96; is high (> 20) |
| L0711 | `internal/mobilebackup/snapshot_page_test.go:108:1` | cognitive complexity 23 of func &#96;TestSnapshotTextProjectionIdentityAuthorsAndGaps&#96; is high (> 20) |
| L0712 | `internal/mobilebackup/snapshot_recalls_test.go:15:1` | cognitive complexity 57 of func &#96;TestSnapshotRecallsSuppressWholeSourceTargetsAcrossPagingAndNamespaces&#96; is high (> 20) |
| L0713 | `internal/mobilebackup/snapshot_recalls_test.go:143:1` | cognitive complexity 27 of func &#96;TestSnapshotRecallInvalidWholeSourceReturnsNoPrefix&#96; is high (> 20) |
| L0714 | `internal/mobilebackup/snapshot_sqlite_test.go:13:1` | cognitive complexity 33 of func &#96;TestSnapshotSQLiteZeroIDsPagingAndImportIsolation&#96; is high (> 20) |
| L0715 | `internal/mobilebackup/snapshot_sqlite_test.go:129:1` | cognitive complexity 24 of func &#96;TestSnapshotSQLiteBlocksEveryDeferredControlOutsideWindow&#96; is high (> 20) |
| L0716 | `internal/mobilebackup/snapshot_terminal.go:15:1` | cognitive complexity 57 of func &#96;(*SnapshotStore).CleanupTerminal&#96; is high (> 20) |
| L0717 | `internal/mobilebackup/snapshot_test.go:196:1` | cognitive complexity 34 of func &#96;TestSnapshotCorruptionAndMissingKeyFailWithoutRotation&#96; is high (> 20) |
| L0718 | `internal/mobilebackup/snapshot_test.go:349:1` | cognitive complexity 29 of func &#96;TestSnapshotRejectsTruncatedClaimsAndUnownedArtifacts&#96; is high (> 20) |
| L0719 | `internal/mobilebackup/sqlite.go:169:1` | cognitive complexity 99 of func &#96;readSQLitePageMode&#96; is high (> 20) |
| L0720 | `internal/mobilebackup/sqlite.go:530:1` | cognitive complexity 27 of func &#96;sqliteRowMode&#96; is high (> 20) |
| L0721 | `internal/mobilebackup/sqlite_test.go:180:1` | cognitive complexity 36 of func &#96;TestSQLitePaginationTiesRejectedRowsAndBoundCursor&#96; is high (> 20) |
| L0722 | `internal/mobilebackup/xz.go:53:1` | cognitive complexity 56 of func &#96;inspectXZ&#96; is high (> 20) |
| L0723 | `internal/mobilebackup/xz.go:153:1` | cognitive complexity 22 of func &#96;inspectXZBlock&#96; is high (> 20) |
| L0724 | `internal/observability/native_test.go:48:1` | cognitive complexity 25 of func &#96;TestNativeSendRecordsOneAmbiguousEffectWithoutRetry&#96; is high (> 20) |
| L0725 | `internal/observability/native_test.go:145:1` | cognitive complexity 23 of func &#96;TestNativeHistoryWorkerKeepsLedgerAndRequestLink&#96; is high (> 20) |
| L0726 | `internal/observability/observer_test.go:61:1` | cognitive complexity 26 of func &#96;count&#96; is high (> 20) |
| L0727 | `internal/observability/observer_test.go:178:1` | cognitive complexity 26 of func &#96;TestSignalsAndOpaqueWorkerCorrelation&#96; is high (> 20) |
| L0728 | `internal/observability/provider.go:42:1` | cognitive complexity 24 of func &#96;NewOwned&#96; is high (> 20) |
| L0729 | `internal/service/account_archive.go:30:1` | cognitive complexity 22 of func &#96;(*membershipPort).captureAccountArchiveWithDownloader&#96; is high (> 20) |
| L0730 | `internal/service/account_archive_control.go:24:1` | cognitive complexity 33 of func &#96;(*membershipPort).accountArchiveControl&#96; is high (> 20) |
| L0731 | `internal/service/account_archive_test.go:51:1` | cognitive complexity 65 of func &#96;testAccountArchiveCapture&#96; is high (> 20) |
| L0732 | `internal/service/archive_catalog.go:43:1` | cognitive complexity 34 of func &#96;(*membershipPort).archiveConversations&#96; is high (> 20) |
| L0733 | `internal/service/archive_catalog_test.go:14:1` | cognitive complexity 37 of func &#96;testOfflineArchiveCatalogue&#96; is high (> 20) |
| L0734 | `internal/service/archive_messages.go:156:1` | cognitive complexity 29 of func &#96;(*membershipPort).archiveMessages&#96; is high (> 20) |
| L0735 | `internal/service/archive_messages_test.go:20:1` | cognitive complexity 60 of func &#96;testOfflineArchiveMessages&#96; is high (> 20) |
| L0736 | `internal/service/cli.go:28:1` | cognitive complexity 43 of func &#96;RunCLI&#96; is high (> 20) |
| L0737 | `internal/service/cli_mobile_ledger.go:29:1` | cognitive complexity 41 of func &#96;mobileLedgerArguments&#96; is high (> 20) |
| L0738 | `internal/service/conversations_test.go:122:1` | cognitive complexity 76 of func &#96;testConversationServiceProfile&#96; is high (> 20) |
| L0739 | `internal/service/events_restart_test.go:123:1` | cognitive complexity 59 of func &#96;TestServiceEventsSurviveRestartWithStableDeliveryAndBoundary&#96; is high (> 20) |
| L0740 | `internal/service/history_test.go:82:1` | cognitive complexity 62 of func &#96;TestHTTPHistoryToolsUseExistingCollectorSession&#96; is high (> 20) |
| L0741 | `internal/service/mobile_archive_probe.go:42:1` | cognitive complexity 49 of func &#96;(*membershipPort).probeMobileArchiveComparisonWithDownloader&#96; is high (> 20) |
| L0742 | `internal/service/mobile_archive_probe_test.go:28:1` | cognitive complexity 25 of func &#96;TestMobileArchiveProbeBoundedOneShotWithoutPersistence&#96; is high (> 20) |
| L0743 | `internal/service/mobile_backup.go:53:1` | cognitive complexity 22 of func &#96;(*membershipPort).consumePreparedMobileArchive&#96; is high (> 20) |
| L0744 | `internal/service/mobile_history_test.go:47:1` | cognitive complexity 27 of func &#96;TestMobileHistoryWorkerUsesExistingCollector&#96; is high (> 20) |
| L0745 | `internal/service/mobile_history_test.go:161:1` | cognitive complexity 41 of func &#96;TestMobileHistorySessionLifetime&#96; is high (> 20) |
| L0746 | `internal/service/mobile_ledger_control.go:26:1` | cognitive complexity 24 of func &#96;uniqueLedgerJSON&#96; is high (> 20) |
| L0747 | `internal/service/mobile_ledger_control.go:72:1` | cognitive complexity 37 of func &#96;(*membershipPort).mobileLedgerControl&#96; is high (> 20) |
| L0748 | `internal/service/mobile_page_test.go:103:1` | cognitive complexity 98 of func &#96;TestServiceMobilePageUsesCollectorSessionAndExpiresHistoricalRows&#96; is high (> 20) |
| L0749 | `internal/service/multiprocess_test.go:40:1` | cognitive complexity 96 of func &#96;TestTwoMCPProcessesReadCollectorWrites&#96; is high (> 20) |
| L0750 | `internal/service/preload_probe_test.go:42:1` | cognitive complexity 25 of func &#96;TestPrivatePreloadProbeUsesServiceSessionWithoutPersistingCorpus&#96; is high (> 20) |
| L0751 | `internal/service/recall_diagnostic.go:77:1` | cognitive complexity 44 of func &#96;(*membershipPort).probeRecall&#96; is high (> 20) |
| L0752 | `internal/service/run.go:154:1` | cognitive complexity 70 of func &#96;runConfigured&#96; is high (> 20) |
| L0753 | `internal/service/run.go:501:1` | cognitive complexity 68 of func &#96;collectSession&#96; is high (> 20) |
| L0754 | `internal/service/run_test.go:49:1` | cognitive complexity 106 of func &#96;TestServicePreservesMCPWhenAuthenticationIsRequired&#96; is high (> 20) |
| L0755 | `internal/service/sending_test.go:79:1` | cognitive complexity 43 of func &#96;TestServiceSendAndReplyUseCollectorSessionAndPersistAcrossRestart&#96; is high (> 20) |
| L0756 | `internal/storage/browse.go:18:1` | cognitive complexity 57 of func &#96;(*Store).Browse&#96; is high (> 20) |
| L0757 | `internal/storage/contacts.go:45:1` | cognitive complexity 29 of func &#96;(*Store).PutContacts&#96; is high (> 20) |
| L0758 | `internal/storage/conversation_views.go:102:1` | cognitive complexity 39 of func &#96;(*Store).Conversations&#96; is high (> 20) |
| L0759 | `internal/storage/deliveries.go:61:1` | cognitive complexity 54 of func &#96;(*Store).FanoutProfileEvents&#96; is high (> 20) |
| L0760 | `internal/storage/deliveries.go:240:1` | cognitive complexity 30 of func &#96;(*Store).ClaimDelivery&#96; is high (> 20) |
| L0761 | `internal/storage/deliveries_test.go:18:1` | cognitive complexity 21 of func &#96;TestBroadConversationFanoutCapacityPreservesCollectedCorpus&#96; is high (> 20) |
| L0762 | `internal/storage/delivery_policy_test.go:10:1` | cognitive complexity 37 of func &#96;TestDeliveryStopsAtExpiryDeadlineAttemptsAndAccessRevocation&#96; is high (> 20) |
| L0763 | `internal/storage/direct_migration_failure_test.go:14:1` | cognitive complexity 44 of func &#96;TestDirectMessagingMigrationsRollbackDDLAndPreserveCorpus&#96; is high (> 20) |
| L0764 | `internal/storage/event_journal.go:138:1` | cognitive complexity 29 of func &#96;(*Store).ReadSubscriptionEvents&#96; is high (> 20) |
| L0765 | `internal/storage/events.go:159:1` | cognitive complexity 26 of func &#96;(*Store).ActivateSubscription&#96; is high (> 20) |
| L0766 | `internal/storage/history_expiry_test.go:110:1` | cognitive complexity 26 of func &#96;TestHistoricalExpiryIsInvisibleBeforePhysicalCleanup&#96; is high (> 20) |
| L0767 | `internal/storage/history_operations.go:152:1` | cognitive complexity 25 of func &#96;(*Store).prepareHistoryOperation&#96; is high (> 20) |
| L0768 | `internal/storage/history_operations.go:362:1` | cognitive complexity 23 of func &#96;(*Store).commitHistoryOperationPage&#96; is high (> 20) |
| L0769 | `internal/storage/history_operations.go:430:1` | cognitive complexity 35 of func &#96;(*Store).commitHistoryRecords&#96; is high (> 20) |
| L0770 | `internal/storage/history_operations_test.go:253:1` | cognitive complexity 41 of func &#96;TestHistoryOperationContinuationBoundsAndAuthPause&#96; is high (> 20) |
| L0771 | `internal/storage/history_test.go:89:1` | cognitive complexity 31 of func &#96;TestHistoryPageRollbackAndExistingLivePreservation&#96; is high (> 20) |
| L0772 | `internal/storage/incoming_migration_test.go:12:1` | cognitive complexity 22 of func &#96;TestIncomingMigrationSeedsEvidenceAndPreservesLegacyQueue&#96; is high (> 20) |
| L0773 | `internal/storage/mobile_backup_test.go:41:1` | cognitive complexity 23 of func &#96;TestMobileBackupJournalIdentityCASAndRestart&#96; is high (> 20) |
| L0774 | `internal/storage/mobile_history.go:158:1` | cognitive complexity 94 of func &#96;(*Store).CommitMobileHistoryPage&#96; is high (> 20) |
| L0775 | `internal/storage/mobile_history_acquisition_test.go:98:1` | cognitive complexity 76 of func &#96;TestMobileHistoryAcquisitionRejectsStaleCancelledAndUnreservedDispatch&#96; is high (> 20) |
| L0776 | `internal/storage/mobile_history_acquisition_test.go:309:1` | cognitive complexity 42 of func &#96;TestMobileHistoryAcquisitionScopeAndInputFailuresHaveNoSideEffects&#96; is high (> 20) |
| L0777 | `internal/storage/mobile_history_acquisition_test.go:469:1` | cognitive complexity 67 of func &#96;TestMobileHistoryInactiveAcquisitionCleanupPreservesOwnershipAndEvidence&#96; is high (> 20) |
| L0778 | `internal/storage/mobile_history_acquisition_test.go:552:1` | cognitive complexity 62 of func &#96;TestMobileHistoryTerminalPublicationReleasesAttemptAtomically&#96; is high (> 20) |
| L0779 | `internal/storage/mobile_history_test.go:444:1` | cognitive complexity 40 of func &#96;TestMobileHistoryTerminalSourceCountsAndLimitWithoutContinuation&#96; is high (> 20) |
| L0780 | `internal/storage/mobile_history_test.go:520:1` | cognitive complexity 29 of func &#96;TestMobileHistoryPolicyRevocationAndForeignAccountCannotPersist&#96; is high (> 20) |
| L0781 | `internal/storage/mobile_snapshot_cleanup_test.go:15:1` | cognitive complexity 40 of func &#96;TestMobileSnapshotCleanupOwnershipAndTerminalState&#96; is high (> 20) |
| L0782 | `internal/storage/preload_directory_test.go:13:1` | cognitive complexity 30 of func &#96;TestPreloadDirectoryMergePreservesCorpusPolicyAndKnownMetadata&#96; is high (> 20) |
| L0783 | `internal/storage/store.go:258:1` | cognitive complexity 26 of func &#96;(*Store).putMessageTx&#96; is high (> 20) |
| L0784 | `internal/storage/store.go:532:1` | cognitive complexity 28 of func &#96;(*Store).prepareSearch&#96; is high (> 20) |
| L0785 | `internal/storage/store.go:594:1` | cognitive complexity 33 of func &#96;(*Store).searchPage&#96; is high (> 20) |
| L0786 | `internal/storage/subscription_scope.go:5:1` | cognitive complexity 29 of func &#96;normalizeSubscription&#96; is high (> 20) |
| L0787 | `internal/storage/views.go:20:1` | cognitive complexity 29 of func &#96;(*Store).Groups&#96; is high (> 20) |
| L0788 | `internal/storage/views.go:283:1` | cognitive complexity 45 of func &#96;(*Store).context&#96; is high (> 20) |
| L0789 | `internal/toolruntime/command_scope_test.go:47:1` | cognitive complexity 38 of func &#96;TestCommandsRejectUntrustedScopeBeforeEffectsAndNeverRetryOwnerFailure&#96; is high (> 20) |
| L0790 | `internal/toolruntime/runtime.go:144:1` | cognitive complexity 55 of func &#96;New&#96; is high (> 20) |
| L0791 | `internal/toolruntime/runtime.go:464:1` | cognitive complexity 21 of func &#96;(*Runtime).dispatch&#96; is high (> 20) |
| L0792 | `internal/zalo/events.go:48:1` | cognitive complexity 43 of func &#96;(*Client).listenConversations&#96; is high (> 20) |
| L0793 | `internal/zalo/join_pending_test.go:28:1` | cognitive complexity 25 of func &#96;TestClientJoinRecognizesOnlyExplicitApprovalCode&#96; is high (> 20) |
| L0794 | `internal/zalo/mobile_backup.go:60:1` | cognitive complexity 111 of func &#96;receiveMobileBackupOffer&#96; is high (> 20) |
| L0795 | `internal/zalo/preload.go:29:1` | cognitive complexity 79 of func &#96;readConversationPreload&#96; is high (> 20) |
| L0796 | `internal/zalo/restore_test.go:19:1` | cognitive complexity 31 of func &#96;TestRestoreDistinguishesLoginRejectionFromNetworkFailure&#96; is high (> 20) |
| L0797 | `internal/zalo/restore_test.go:72:1` | cognitive complexity 24 of func &#96;TestRestoreRejectsMissingLegacyAndInsecureSessionBeforeLogin&#96; is high (> 20) |
| L0798 | `tools/harness/internal/architecture/architecture.go:165:1` | cognitive complexity 34 of func &#96;Scan&#96; is high (> 20) |
| L0799 | `tools/harness/internal/architecture/architecture.go:381:1` | cognitive complexity 22 of func &#96;SourceControls&#96; is high (> 20) |
| L0800 | `tools/harness/internal/architecture/architecture.go:449:1` | cognitive complexity 28 of func &#96;Check&#96; is high (> 20) |
| L0801 | `tools/harness/internal/architecture/architecture_test.go:75:1` | cognitive complexity 29 of func &#96;TestConfigDecisionEquivalence&#96; is high (> 20) |
| L0802 | `tools/harness/internal/performance/bench.go:75:1` | cognitive complexity 82 of func &#96;Bench&#96; is high (> 20) |
| L0803 | `tools/harness/internal/performance/build.go:71:1` | cognitive complexity 25 of func &#96;Build&#96; is high (> 20) |
| L0804 | `tools/harness/internal/performance/compare.go:8:1` | cognitive complexity 70 of func &#96;validate&#96; is high (> 20) |
| L0805 | `tools/harness/internal/performance/compare.go:150:1` | cognitive complexity 79 of func &#96;Compare&#96; is high (> 20) |
| L0806 | `tools/harness/internal/performance/process.go:18:1` | cognitive complexity 27 of func &#96;process&#96; is high (> 20) |

### goconst (479)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Выделять константы для действительно общего контракта: operation states, reason codes, schema keys внутри их владельца; сначала использовать существующие domain constants. Одинаковые тексты с разным смыслом не объединять. Для независимых fixture expected values и SQL/JSON ключей, где литерал делает контракт видимым, допустим точечный nolint:goconst. Не создавать общий пакет строковых констант и новые архитектурные зависимости ради lint.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L0807 | `docs/contracts/contracts.go:20:65` | string &#96;history_import_operation&#96; has 5 occurrences, make it a constant |
| L0808 | `internal/app/search/read.go:185:13` | string &#96;no_collected_data&#96; has 3 occurrences, make it a constant |
| L0809 | `internal/collector/contact_profiles.go:34:3` | string &#96;status&#96; has 4 occurrences, make it a constant |
| L0810 | `internal/collector/contact_profiles.go:34:22` | string &#96;running&#96; has 5 occurrences, make it a constant |
| L0811 | `internal/collector/contact_profiles.go:35:3` | string &#96;last_attempt_at&#96; has 3 occurrences, make it a constant |
| L0812 | `internal/collector/contact_profiles.go:36:3` | string &#96;last_success_at&#96; has 3 occurrences, make it a constant |
| L0813 | `internal/collector/contact_profiles.go:40:3` | string &#96;stop_reason&#96; has 3 occurrences, make it a constant |
| L0814 | `internal/collector/contact_profiles.go:61:13` | string &#96;cancelled_or_deadline&#96; has 4 occurrences, make it a constant |
| L0815 | `internal/collector/contact_profiles.go:76:22` | string &#96;partial&#96; has 3 occurrences, make it a constant |
| L0816 | `internal/collector/contact_profiles.go:76:33` | string &#96;upstream_unavailable&#96; has 3 occurrences, make it a constant |
| L0817 | `internal/collector/contact_profiles.go:79:14` | string &#96;unsupported&#96; has 5 occurrences, make it a constant |
| L0818 | `internal/collector/contact_profiles.go:80:15` | string &#96;source_unsupported&#96; has 5 occurrences, make it a constant |
| L0819 | `internal/collector/contact_profiles.go:82:15` | string &#96;auth_required&#96; has 7 occurrences, make it a constant |
| L0820 | `internal/collector/contacts.go:38:3` | string &#96;status&#96; has 4 occurrences, make it a constant |
| L0821 | `internal/collector/contacts.go:38:22` | string &#96;running&#96; has 5 occurrences, make it a constant |
| L0822 | `internal/collector/contacts.go:39:3` | string &#96;last_attempt_at&#96; has 3 occurrences, make it a constant |
| L0823 | `internal/collector/contacts.go:40:3` | string &#96;last_success_at&#96; has 3 occurrences, make it a constant |
| L0824 | `internal/collector/contacts.go:43:3` | string &#96;stop_reason&#96; has 3 occurrences, make it a constant |
| L0825 | `internal/collector/contacts.go:66:14` | string &#96;upstream_unavailable&#96; has 3 occurrences, make it a constant |
| L0826 | `internal/collector/contacts.go:67:13` | string &#96;partial&#96; has 3 occurrences, make it a constant |
| L0827 | `internal/collector/contacts.go:70:13` | string &#96;unsupported&#96; has 5 occurrences, make it a constant |
| L0828 | `internal/collector/contacts.go:71:14` | string &#96;source_unsupported&#96; has 5 occurrences, make it a constant |
| L0829 | `internal/collector/contacts.go:73:14` | string &#96;auth_required&#96; has 7 occurrences, make it a constant |
| L0830 | `internal/collector/contacts.go:75:14` | string &#96;cancelled_or_deadline&#96; has 4 occurrences, make it a constant |
| L0831 | `internal/collector/join.go:132:12` | string &#96;unknown&#96; has 6 occurrences, make it a constant |
| L0832 | `internal/collector/join.go:147:3` | string &#96;group_id&#96; has 3 occurrences, make it a constant |
| L0833 | `internal/collector/join.go:320:3` | string &#96;status&#96; has 4 occurrences, make it a constant |
| L0834 | `internal/collector/join.go:320:25` | string &#96;running&#96; has 5 occurrences, make it a constant |
| L0835 | `internal/collector/join.go:412:4` | string &#96;code&#96; has 7 occurrences, make it a constant |
| L0836 | `internal/collector/join.go:413:4` | string &#96;message&#96; has 7 occurrences, make it a constant |
| L0837 | `internal/collector/join.go:466:16` | string &#96;UPSTREAM_UNAVAILABLE&#96; has 5 occurrences, make it a constant |
| L0838 | `internal/collector/preload_catalog.go:26:3` | string &#96;status&#96; has 4 occurrences, make it a constant |
| L0839 | `internal/collector/preload_catalog.go:26:24` | string &#96;running&#96; has 5 occurrences, make it a constant |
| L0840 | `internal/collector/preload_catalog.go:27:3` | string &#96;last_attempt_at&#96; has 3 occurrences, make it a constant |
| L0841 | `internal/collector/preload_catalog.go:28:3` | string &#96;last_success_at&#96; has 3 occurrences, make it a constant |
| L0842 | `internal/collector/preload_catalog.go:33:3` | string &#96;stop_reason&#96; has 3 occurrences, make it a constant |
| L0843 | `internal/collector/preload_catalog.go:52:20` | string &#96;partial&#96; has 3 occurrences, make it a constant |
| L0844 | `internal/collector/preload_catalog.go:52:31` | string &#96;upstream_unavailable&#96; has 3 occurrences, make it a constant |
| L0845 | `internal/collector/preload_catalog.go:55:20` | string &#96;unsupported&#96; has 5 occurrences, make it a constant |
| L0846 | `internal/collector/preload_catalog.go:55:35` | string &#96;source_unsupported&#96; has 5 occurrences, make it a constant |
| L0847 | `internal/collector/preload_catalog.go:57:13` | string &#96;auth_required&#96; has 7 occurrences, make it a constant |
| L0848 | `internal/collector/preload_catalog.go:59:13` | string &#96;invalid_source_page&#96; has 4 occurrences, make it a constant |
| L0849 | `internal/collector/preload_catalog.go:61:13` | string &#96;cancelled_or_deadline&#96; has 4 occurrences, make it a constant |
| L0850 | `internal/collector/preload_probe.go:15:32` | string &#96;unsupported&#96; has 5 occurrences, make it a constant |
| L0851 | `internal/collector/preload_probe.go:16:32` | string &#96;source_unsupported&#96; has 5 occurrences, make it a constant |
| L0852 | `internal/collector/preload_probe.go:37:29` | string &#96;failed&#96; has 3 occurrences, make it a constant |
| L0853 | `internal/collector/preload_probe.go:38:15` | string &#96;unknown&#96; has 6 occurrences, make it a constant |
| L0854 | `internal/collector/preload_probe.go:46:30` | string &#96;auth_required&#96; has 7 occurrences, make it a constant |
| L0855 | `internal/collector/preload_probe.go:49:15` | string &#96;invalid_source_page&#96; has 4 occurrences, make it a constant |
| L0856 | `internal/collector/run.go:139:19` | string &#96;code&#96; has 7 occurrences, make it a constant |
| L0857 | `internal/collector/run.go:139:27` | string &#96;UPSTREAM_UNAVAILABLE&#96; has 5 occurrences, make it a constant |
| L0858 | `internal/collector/run.go:139:51` | string &#96;message&#96; has 7 occurrences, make it a constant |
| L0859 | `internal/collector/run.go:181:34` | string &#96;auth_required&#96; has 7 occurrences, make it a constant |
| L0860 | `internal/collector/run.go:349:28` | string &#96;group_id&#96; has 3 occurrences, make it a constant |
| L0861 | `internal/domain/history.go:60:12` | string &#96;unknown&#96; has 3 occurrences, make it a constant |
| L0862 | `internal/domain/observation.go:28:10` | string &#96;unknown&#96; has 3 occurrences, make it a constant |
| L0863 | `internal/evalfixture/evidence.go:47:48` | string &#96;direct&#96; has 3 occurrences, make it a constant |
| L0864 | `internal/evalfixture/evidence.go:47:62` | string &#96;peer&#96; has 5 occurrences, make it a constant |
| L0865 | `internal/evalfixture/evidence.go:53:19` | string &#96;live&#96; has 4 occurrences, make it a constant |
| L0866 | `internal/evalfixture/fixture.go:92:44` | string &#96;g-join&#96; has 3 occurrences, make it a constant |
| L0867 | `internal/evalfixture/fixture.go:189:21` | string &#96;live&#96; has 4 occurrences, make it a constant |
| L0868 | `internal/evalfixture/offline.go:90:33` | string &#96;direct&#96; has 3 occurrences, make it a constant |
| L0869 | `internal/evalfixture/offline.go:96:20` | string &#96;peer&#96; has 5 occurrences, make it a constant |
| L0870 | `internal/evalfixture/offline.go:99:20` | string &#96;live&#96; has 4 occurrences, make it a constant |
| L0871 | `internal/evaluation/benchmark.go:60:10` | string &#96;zalo_search_messages&#96; has 4 occurrences, make it a constant |
| L0872 | `internal/evaluation/benchmark.go:61:19` | string &#96;ремонт&#96; has 6 occurrences, make it a constant |
| L0873 | `internal/evaluation/benchmark.go:63:10` | string &#96;zalo_get_message_context&#96; has 3 occurrences, make it a constant |
| L0874 | `internal/evaluation/benchmark.go:64:22` | string &#96;g-1&#96; has 5 occurrences, make it a constant |
| L0875 | `internal/evaluation/checkpoints.go:72:7` | string &#96;service-recommendation&#96; has 4 occurrences, make it a constant |
| L0876 | `internal/evaluation/checkpoints.go:73:3` | string &#96;agreement-history&#96; has 4 occurrences, make it a constant |
| L0877 | `internal/evaluation/checkpoints.go:74:3` | string &#96;duplicate-name&#96; has 3 occurrences, make it a constant |
| L0878 | `internal/evaluation/checkpoints.go:75:3` | string &#96;duplicate-followup&#96; has 3 occurrences, make it a constant |
| L0879 | `internal/evaluation/checkpoints.go:76:3` | string &#96;date-clarification&#96; has 3 occurrences, make it a constant |
| L0880 | `internal/evaluation/checkpoints.go:77:3` | string &#96;unicode&#96; has 3 occurrences, make it a constant |
| L0881 | `internal/evaluation/checkpoints.go:78:3` | string &#96;coverage&#96; has 4 occurrences, make it a constant |
| L0882 | `internal/evaluation/checkpoints.go:79:3` | string &#96;invalid-input&#96; has 3 occurrences, make it a constant |
| L0883 | `internal/evaluation/checkpoints.go:80:3` | string &#96;long-message&#96; has 3 occurrences, make it a constant |
| L0884 | `internal/evaluation/checkpoints.go:81:3` | string &#96;approved-join&#96; has 6 occurrences, make it a constant |
| L0885 | `internal/evaluation/checkpoints.go:82:3` | string &#96;adversarial&#96; has 4 occurrences, make it a constant |
| L0886 | `internal/evaluation/checkpoints.go:83:3` | string &#96;trigger&#96; has 5 occurrences, make it a constant |
| L0887 | `internal/evaluation/checkpoints.go:152:22` | string &#96;FAIL&#96; has 3 occurrences, make it a constant |
| L0888 | `internal/evaluation/checkpoints.go:154:22` | string &#96;NOT_RUN&#96; has 10 occurrences, make it a constant |
| L0889 | `internal/evaluation/checkpoints.go:177:16` | string &#96;tool&#96; has 4 occurrences, make it a constant |
| L0890 | `internal/evaluation/checkpoints.go:177:36` | string &#96;resource&#96; has 5 occurrences, make it a constant |
| L0891 | `internal/evaluation/checkpoints.go:203:4` | string &#96;zalo_search_messages&#96; has 4 occurrences, make it a constant |
| L0892 | `internal/evaluation/checkpoints.go:204:4` | string &#96;zalo_get_message_context&#96; has 3 occurrences, make it a constant |
| L0893 | `internal/evaluation/checkpoints.go:208:4` | string &#96;zalo_join_group&#96; has 3 occurrences, make it a constant |
| L0894 | `internal/evaluation/checkpoints.go:427:20` | string &#96;text&#96; has 3 occurrences, make it a constant |
| L0895 | `internal/evaluation/client.go:40:19` | string &#96;tool&#96; has 4 occurrences, make it a constant |
| L0896 | `internal/evaluation/client.go:60:19` | string &#96;resource&#96; has 5 occurrences, make it a constant |
| L0897 | `internal/evaluation/grader.go:30:16` | string &#96;tool&#96; has 4 occurrences, make it a constant |
| L0898 | `internal/evaluation/grader.go:92:32` | string &#96;send&#96; has 3 occurrences, make it a constant |
| L0899 | `internal/evaluation/grader.go:92:40` | string &#96;join&#96; has 3 occurrences, make it a constant |
| L0900 | `internal/evaluation/grader.go:92:48` | string &#96;history&#96; has 3 occurrences, make it a constant |
| L0901 | `internal/evaluation/grader.go:121:8` | string &#96;same-as&#96; has 3 occurrences, make it a constant |
| L0902 | `internal/evaluation/grader.go:121:19` | string &#96;different-from&#96; has 3 occurrences, make it a constant |
| L0903 | `internal/evaluation/grader.go:166:25` | string &#96;resource&#96; has 5 occurrences, make it a constant |
| L0904 | `internal/evaluation/model.go:38:20` | string &#96;tool&#96; has 4 occurrences, make it a constant |
| L0905 | `internal/evaluation/model.go:52:20` | string &#96;resource&#96; has 5 occurrences, make it a constant |
| L0906 | `internal/evaluation/runner.go:74:9` | string &#96;same-as&#96; has 3 occurrences, make it a constant |
| L0907 | `internal/evaluation/runner.go:75:5` | string &#96;different-from&#96; has 3 occurrences, make it a constant |
| L0908 | `internal/evaluation/runner.go:91:29` | string &#96;send&#96; has 3 occurrences, make it a constant |
| L0909 | `internal/evaluation/runner.go:91:47` | string &#96;join&#96; has 3 occurrences, make it a constant |
| L0910 | `internal/evaluation/runner.go:91:65` | string &#96;history&#96; has 3 occurrences, make it a constant |
| L0911 | `internal/evaluation/runner.go:139:15` | string &#96;NOT_RUN&#96; has 10 occurrences, make it a constant |
| L0912 | `internal/evaluation/runner.go:146:50` | string &#96;model&#96; has 5 occurrences, make it a constant |
| L0913 | `internal/evaluation/runner.go:151:4` | string &#96;profile&#96; has 3 occurrences, make it a constant |
| L0914 | `internal/evaluation/runner.go:152:4` | string &#96;case_budget_seconds&#96; has 3 occurrences, make it a constant |
| L0915 | `internal/evaluation/runner.go:154:4` | string &#96;run_budget_seconds&#96; has 3 occurrences, make it a constant |
| L0916 | `internal/evaluation/runner.go:155:4` | string &#96;model_gate&#96; has 3 occurrences, make it a constant |
| L0917 | `internal/evaluation/runner.go:325:36` | string &#96;operation&#96; has 4 occurrences, make it a constant |
| L0918 | `internal/evaluation/runner.go:325:49` | string &#96;effects&#96; has 4 occurrences, make it a constant |
| L0919 | `internal/evaluation/runner.go:325:60` | string &#96;lifecycle&#96; has 4 occurrences, make it a constant |
| L0920 | `internal/evaluation/runner.go:332:46` | string &#96;case&#96; has 4 occurrences, make it a constant |
| L0921 | `internal/evaluation/runner.go:332:61` | string &#96;complete&#96; has 4 occurrences, make it a constant |
| L0922 | `internal/evaluation/runner.go:439:18` | string &#96;FAIL&#96; has 3 occurrences, make it a constant |
| L0923 | `internal/evaluation/scenarios.go:41:20` | string &#96;archive-dates&#96; has 3 occurrences, make it a constant |
| L0924 | `internal/evaluation/scenarios.go:41:54` | string &#96;archive-empty-page&#96; has 3 occurrences, make it a constant |
| L0925 | `internal/evaluation/scenarios.go:93:65` | string &#96;query&#96; has 6 occurrences, make it a constant |
| L0926 | `internal/evaluation/scenarios.go:93:74` | string &#96;ремонт&#96; has 6 occurrences, make it a constant |
| L0927 | `internal/evaluation/scenarios.go:93:90` | string &#96;limit&#96; has 6 occurrences, make it a constant |
| L0928 | `internal/evaluation/scenarios.go:95:42` | string &#96;peer&#96; has 3 occurrences, make it a constant |
| L0929 | `internal/evaluation/scenarios.go:95:50` | string &#96;text&#96; has 3 occurrences, make it a constant |
| L0930 | `internal/evaluation/scenarios.go:95:64` | string &#96;request_id&#96; has 7 occurrences, make it a constant |
| L0931 | `internal/evaluation/scenarios.go:117:33` | string &#96;direct&#96; has 9 occurrences, make it a constant |
| L0932 | `internal/evaluation/scenarios.go:117:43` | string &#96;group&#96; has 4 occurrences, make it a constant |
| L0933 | `internal/evaluation/scenarios.go:120:20` | string &#96;conversation_type&#96; has 9 occurrences, make it a constant |
| L0934 | `internal/evaluation/scenarios.go:120:47` | string &#96;conversation_id&#96; has 9 occurrences, make it a constant |
| L0935 | `internal/evaluation/scenarios.go:128:31` | string &#96;g-1&#96; has 5 occurrences, make it a constant |
| L0936 | `internal/evaluation/scenarios.go:154:4` | string &#96;since&#96; has 4 occurrences, make it a constant |
| L0937 | `internal/evaluation/scenarios.go:155:4` | string &#96;until&#96; has 3 occurrences, make it a constant |
| L0938 | `internal/evaluation/scenarios.go:274:52` | string &#96;operation_id&#96; has 3 occurrences, make it a constant |
| L0939 | `internal/events/message.go:140:33` | string &#96;name&#96; has 3 occurrences, make it a constant |
| L0940 | `internal/events/subscriptions.go:140:7` | string &#96;events/subscribe&#96; has 3 occurrences, make it a constant |
| L0941 | `internal/events/subscriptions.go:142:7` | string &#96;events/unsubscribe&#96; has 3 occurrences, make it a constant |
| L0942 | `internal/events/subscriptions.go:176:6` | string &#96;name&#96; has 3 occurrences, make it a constant |
| L0943 | `internal/events/worker.go:137:50` | string &#96;failed&#96; has 4 occurrences, make it a constant |
| L0944 | `internal/historyimport/manager.go:30:15` | string &#96;zalo_import_conversation_history&#96; has 4 occurrences, make it a constant |
| L0945 | `internal/historyimport/mobile_worker.go:111:38` | string &#96;running&#96; has 8 occurrences, make it a constant |
| L0946 | `internal/historyimport/mobile_worker.go:293:50` | string &#96;INVALID_ARGUMENT&#96; has 3 occurrences, make it a constant |
| L0947 | `internal/historyimport/mobile_worker.go:342:34` | string &#96;source_unsupported&#96; has 3 occurrences, make it a constant |
| L0948 | `internal/historyimport/mobile_worker.go:344:29` | string &#96;invalid_source_page&#96; has 5 occurrences, make it a constant |
| L0949 | `internal/historyimport/worker.go:100:25` | string &#96;running&#96; has 8 occurrences, make it a constant |
| L0950 | `internal/historyimport/worker.go:177:36` | string &#96;source_unsupported&#96; has 3 occurrences, make it a constant |
| L0951 | `internal/historyimport/worker.go:180:53` | string &#96;INVALID_ARGUMENT&#96; has 3 occurrences, make it a constant |
| L0952 | `internal/historyimport/worker.go:181:14` | string &#96;invalid_source_page&#96; has 5 occurrences, make it a constant |
| L0953 | `internal/mcpserver/catalog_diagnostics.go:22:14` | string &#96;application/json&#96; has 10 occurrences, make it a constant |
| L0954 | `internal/mcpserver/conversations.go:26:17` | string &#96;application/json&#96; has 10 occurrences, make it a constant |
| L0955 | `internal/mcpserver/conversations.go:34:33` | string &#96;zalo&#96; has 3 occurrences, make it a constant |
| L0956 | `internal/mcpserver/conversations.go:104:17` | string &#96;text/plain&#96; has 7 occurrences, make it a constant |
| L0957 | `internal/mcpserver/conversations.go:120:38` | string &#96;messages&#96; has 3 occurrences, make it a constant |
| L0958 | `internal/mcpserver/diagnostics.go:23:14` | string &#96;application/json&#96; has 10 occurrences, make it a constant |
| L0959 | `internal/mcpserver/discovery_logging.go:17:40` | string &#96;tools/list&#96; has 3 occurrences, make it a constant |
| L0960 | `internal/mcpserver/discovery_logging.go:61:7` | string &#96;2026-07-28&#96; has 3 occurrences, make it a constant |
| L0961 | `internal/mcpserver/http.go:89:9` | string &#96;application/json&#96; has 10 occurrences, make it a constant |
| L0962 | `internal/mcpserver/http.go:167:80` | string &#96;tools/list&#96; has 3 occurrences, make it a constant |
| L0963 | `internal/mcpserver/http.go:240:17` | string &#96;2026-07-28&#96; has 3 occurrences, make it a constant |
| L0964 | `internal/mcpserver/ports.go:39:3` | string &#96;messages&#96; has 3 occurrences, make it a constant |
| L0965 | `internal/mcpserver/server.go:30:2` | string &#96;zalo_ack_subscription_events&#96; has 3 occurrences, make it a constant |
| L0966 | `internal/mcpserver/server.go:33:2` | string &#96;zalo_cancel_history_import&#96; has 3 occurrences, make it a constant |
| L0967 | `internal/mcpserver/server.go:155:95` | string &#96;text/plain&#96; has 7 occurrences, make it a constant |
| L0968 | `internal/mcpserver/server.go:190:31` | string &#96;zalo&#96; has 3 occurrences, make it a constant |
| L0969 | `internal/mcpserver/server.go:194:38` | string &#96;messages&#96; has 3 occurrences, make it a constant |
| L0970 | `internal/mcpserver/server.go:228:15` | string &#96;STORAGE_ERROR&#96; has 3 occurrences, make it a constant |
| L0971 | `internal/mcpserver/server.go:246:20` | string &#96;TOOL_ERROR&#96; has 3 occurrences, make it a constant |
| L0972 | `internal/mobilebackup/account_metadata.go:107:17` | string &#96;plain&#96; has 3 occurrences, make it a constant |
| L0973 | `internal/mobilebackup/account_metadata.go:125:10` | string &#96;rtf&#96; has 3 occurrences, make it a constant |
| L0974 | `internal/mobilebackup/account_metadata.go:125:26` | string &#96;text&#96; has 3 occurrences, make it a constant |
| L0975 | `internal/mobilebackup/account_metadata.go:125:43` | string &#96;webchat&#96; has 5 occurrences, make it a constant |
| L0976 | `internal/mobilebackup/identities.go:57:11` | string &#96;fids&#96; has 3 occurrences, make it a constant |
| L0977 | `internal/mobilebackup/identities.go:59:11` | string &#96;gids&#96; has 3 occurrences, make it a constant |
| L0978 | `internal/mobilebackup/message_conversion.go:146:16` | string &#96;incoming&#96; has 3 occurrences, make it a constant |
| L0979 | `internal/mobilebackup/message_conversion.go:148:16` | string &#96;outgoing&#96; has 3 occurrences, make it a constant |
| L0980 | `internal/mobilebackup/message_conversion.go:205:22` | string &#96;webchat&#96; has 5 occurrences, make it a constant |
| L0981 | `internal/mobilebackup/message_conversion.go:211:39` | string &#96;rtf&#96; has 3 occurrences, make it a constant |
| L0982 | `internal/mobilebackup/row_page.go:45:10` | string &#96;webchat&#96; has 5 occurrences, make it a constant |
| L0983 | `internal/mobilebackup/row_page.go:227:19` | string &#96;incoming&#96; has 3 occurrences, make it a constant |
| L0984 | `internal/mobilebackup/row_page.go:229:20` | string &#96;outgoing&#96; has 3 occurrences, make it a constant |
| L0985 | `internal/mobilebackup/snapshot_page.go:44:3` | string &#96;timestamp&#96; has 4 occurrences, make it a constant |
| L0986 | `internal/mobilebackup/snapshot_page.go:45:3` | string &#96;text&#96; has 3 occurrences, make it a constant |
| L0987 | `internal/mobilebackup/snapshot_page.go:215:16` | string &#96;incoming&#96; has 3 occurrences, make it a constant |
| L0988 | `internal/mobilebackup/snapshot_page.go:217:17` | string &#96;outgoing&#96; has 3 occurrences, make it a constant |
| L0989 | `internal/mobilebackup/snapshot_page.go:222:15` | string &#96;plain&#96; has 3 occurrences, make it a constant |
| L0990 | `internal/mobilebackup/sqlite.go:295:3` | string &#96;timestamp&#96; has 4 occurrences, make it a constant |
| L0991 | `internal/mobilebackup/sqlite.go:561:15` | string &#96;text&#96; has 3 occurrences, make it a constant |
| L0992 | `internal/mobilebackup/sqlite.go:583:16` | string &#96;metadata&#96; has 3 occurrences, make it a constant |
| L0993 | `internal/service/account_archive.go:23:48` | string &#96;trans-bin.zaloapp.com&#96; has 3 occurrences, make it a constant |
| L0994 | `internal/service/account_archive.go:67:22` | string &#96;prepared&#96; has 6 occurrences, make it a constant |
| L0995 | `internal/service/account_archive.go:224:3` | string &#96;download_performed&#96; has 3 occurrences, make it a constant |
| L0996 | `internal/service/account_archive.go:225:3` | string &#96;import_performed&#96; has 4 occurrences, make it a constant |
| L0997 | `internal/service/account_archive.go:226:3` | string &#96;history_complete&#96; has 4 occurrences, make it a constant |
| L0998 | `internal/service/account_archive.go:248:24` | string &#96;source_id&#96; has 4 occurrences, make it a constant |
| L0999 | `internal/service/account_archive_control.go:16:19` | string &#96;cli_restore_account_archive&#96; has 4 occurrences, make it a constant |
| L1000 | `internal/service/account_archive_control.go:16:62` | string &#96;cli_preserve_account_archive&#96; has 4 occurrences, make it a constant |
| L1001 | `internal/service/account_archive_control.go:17:13` | string &#96;cli_prepare_account_archive&#96; has 3 occurrences, make it a constant |
| L1002 | `internal/service/account_archive_control.go:18:13` | string &#96;cli_capture_account_archive&#96; has 4 occurrences, make it a constant |
| L1003 | `internal/service/account_archive_control.go:19:13` | string &#96;cli_account_archive_status&#96; has 3 occurrences, make it a constant |
| L1004 | `internal/service/account_archive_control.go:20:13` | string &#96;cli_inspect_account_archive&#96; has 4 occurrences, make it a constant |
| L1005 | `internal/service/account_archive_control.go:21:13` | string &#96;cli_remove_account_archive&#96; has 3 occurrences, make it a constant |
| L1006 | `internal/service/account_archive_control.go:58:50` | string &#96;/rpc&#96; has 3 occurrences, make it a constant |
| L1007 | `internal/service/archive_catalog.go:151:3` | string &#96;has_more&#96; has 3 occurrences, make it a constant |
| L1008 | `internal/service/archive_catalog.go:154:32` | string &#96;retained_archive&#96; has 3 occurrences, make it a constant |
| L1009 | `internal/service/archive_catalog.go:177:26` | string &#96;unknown&#96; has 3 occurrences, make it a constant |
| L1010 | `internal/service/archive_messages.go:283:3` | string &#96;history_complete&#96; has 4 occurrences, make it a constant |
| L1011 | `internal/service/archive_messages.go:284:3` | string &#96;examined&#96; has 3 occurrences, make it a constant |
| L1012 | `internal/service/archive_messages.go:285:3` | string &#96;rejected&#96; has 3 occurrences, make it a constant |
| L1013 | `internal/service/archive_messages.go:301:3` | string &#96;has_more&#96; has 3 occurrences, make it a constant |
| L1014 | `internal/service/archive_read.go:47:43` | string &#96;history_complete&#96; has 4 occurrences, make it a constant |
| L1015 | `internal/service/archive_read.go:53:3` | string &#96;source_id&#96; has 4 occurrences, make it a constant |
| L1016 | `internal/service/bridge.go:190:17` | string &#96;other&#96; has 3 occurrences, make it a constant |
| L1017 | `internal/service/cli.go:64:7` | string &#96;probe-recall&#96; has 3 occurrences, make it a constant |
| L1018 | `internal/service/cli.go:65:3` | string &#96;mobile-backup-prepare&#96; has 3 occurrences, make it a constant |
| L1019 | `internal/service/cli.go:66:3` | string &#96;mobile-backup-status&#96; has 3 occurrences, make it a constant |
| L1020 | `internal/service/cli.go:67:3` | string &#96;mobile-backup-cancel&#96; has 3 occurrences, make it a constant |
| L1021 | `internal/service/cli.go:68:3` | string &#96;mobile-backup-probe-offer&#96; has 4 occurrences, make it a constant |
| L1022 | `internal/service/cli.go:69:3` | string &#96;mobile-backup-probe-archive&#96; has 5 occurrences, make it a constant |
| L1023 | `internal/service/cli.go:70:3` | string &#96;account-archive-prepare&#96; has 4 occurrences, make it a constant |
| L1024 | `internal/service/cli.go:71:3` | string &#96;account-archive-capture&#96; has 4 occurrences, make it a constant |
| L1025 | `internal/service/cli.go:72:3` | string &#96;account-archive-status&#96; has 3 occurrences, make it a constant |
| L1026 | `internal/service/cli.go:73:3` | string &#96;account-archive-preserve&#96; has 4 occurrences, make it a constant |
| L1027 | `internal/service/cli.go:74:3` | string &#96;account-archive-restore&#96; has 4 occurrences, make it a constant |
| L1028 | `internal/service/cli.go:75:3` | string &#96;account-archive-inspect&#96; has 4 occurrences, make it a constant |
| L1029 | `internal/service/cli.go:76:3` | string &#96;account-archive-remove&#96; has 3 occurrences, make it a constant |
| L1030 | `internal/service/cli_mobile_ledger.go:16:17` | string &#96;account-archive-restore&#96; has 4 occurrences, make it a constant |
| L1031 | `internal/service/cli_mobile_ledger.go:16:54` | string &#96;account-archive-preserve&#96; has 4 occurrences, make it a constant |
| L1032 | `internal/service/cli_mobile_ledger.go:17:11` | string &#96;account-archive-inspect&#96; has 4 occurrences, make it a constant |
| L1033 | `internal/service/cli_mobile_ledger.go:18:11` | string &#96;account-archive-remove&#96; has 3 occurrences, make it a constant |
| L1034 | `internal/service/cli_mobile_ledger.go:19:11` | string &#96;account-archive-prepare&#96; has 4 occurrences, make it a constant |
| L1035 | `internal/service/cli_mobile_ledger.go:20:11` | string &#96;account-archive-capture&#96; has 4 occurrences, make it a constant |
| L1036 | `internal/service/cli_mobile_ledger.go:21:11` | string &#96;account-archive-status&#96; has 3 occurrences, make it a constant |
| L1037 | `internal/service/cli_mobile_ledger.go:22:11` | string &#96;probe-recall&#96; has 3 occurrences, make it a constant |
| L1038 | `internal/service/cli_mobile_ledger.go:23:11` | string &#96;mobile-backup-prepare&#96; has 3 occurrences, make it a constant |
| L1039 | `internal/service/cli_mobile_ledger.go:24:11` | string &#96;mobile-backup-status&#96; has 3 occurrences, make it a constant |
| L1040 | `internal/service/cli_mobile_ledger.go:25:11` | string &#96;mobile-backup-cancel&#96; has 3 occurrences, make it a constant |
| L1041 | `internal/service/cli_mobile_ledger.go:26:11` | string &#96;mobile-backup-probe-offer&#96; has 4 occurrences, make it a constant |
| L1042 | `internal/service/cli_mobile_ledger.go:27:11` | string &#96;mobile-backup-probe-archive&#96; has 5 occurrences, make it a constant |
| L1043 | `internal/service/cli_mobile_ledger.go:38:10` | string &#96;cli_probe_recall&#96; has 4 occurrences, make it a constant |
| L1044 | `internal/service/cli_mobile_ledger.go:52:13` | string &#96;cli_prepare_mobile_backup&#96; has 3 occurrences, make it a constant |
| L1045 | `internal/service/cli_mobile_ledger.go:54:13` | string &#96;cli_prepare_account_archive&#96; has 3 occurrences, make it a constant |
| L1046 | `internal/service/cli_mobile_ledger.go:57:13` | string &#96;cli_inspect_account_archive&#96; has 4 occurrences, make it a constant |
| L1047 | `internal/service/cli_mobile_ledger.go:60:13` | string &#96;cli_preserve_account_archive&#96; has 4 occurrences, make it a constant |
| L1048 | `internal/service/cli_mobile_ledger.go:63:13` | string &#96;cli_restore_account_archive&#96; has 4 occurrences, make it a constant |
| L1049 | `internal/service/cli_mobile_ledger.go:70:28` | string &#96;source_id&#96; has 4 occurrences, make it a constant |
| L1050 | `internal/service/cli_mobile_ledger.go:74:10` | string &#96;cli_remove_account_archive&#96; has 3 occurrences, make it a constant |
| L1051 | `internal/service/cli_mobile_ledger.go:79:10` | string &#96;cli_account_archive_status&#96; has 3 occurrences, make it a constant |
| L1052 | `internal/service/cli_mobile_ledger.go:93:13` | string &#96;cli_cancel_prepared_mobile_backup&#96; has 3 occurrences, make it a constant |
| L1053 | `internal/service/cli_mobile_ledger.go:95:13` | string &#96;cli_probe_mobile_backup_offer&#96; has 5 occurrences, make it a constant |
| L1054 | `internal/service/cli_mobile_ledger.go:98:13` | string &#96;cli_probe_mobile_backup_archive&#96; has 6 occurrences, make it a constant |
| L1055 | `internal/service/cli_mobile_ledger.go:101:13` | string &#96;cli_capture_account_archive&#96; has 4 occurrences, make it a constant |
| L1056 | `internal/service/mobile_archive_probe.go:26:48` | string &#96;trans-bin.zaloapp.com&#96; has 3 occurrences, make it a constant |
| L1057 | `internal/service/mobile_archive_probe.go:53:60` | string &#96;prepared&#96; has 6 occurrences, make it a constant |
| L1058 | `internal/service/mobile_archive_probe.go:191:3` | string &#96;download_performed&#96; has 3 occurrences, make it a constant |
| L1059 | `internal/service/mobile_archive_probe.go:192:3` | string &#96;import_performed&#96; has 4 occurrences, make it a constant |
| L1060 | `internal/service/mobile_archive_probe.go:194:3` | string &#96;examined&#96; has 3 occurrences, make it a constant |
| L1061 | `internal/service/mobile_archive_probe.go:195:3` | string &#96;rejected&#96; has 3 occurrences, make it a constant |
| L1062 | `internal/service/mobile_archive_probe.go:248:3` | string &#96;has_more&#96; has 3 occurrences, make it a constant |
| L1063 | `internal/service/mobile_backup.go:93:72` | string &#96;prepared&#96; has 6 occurrences, make it a constant |
| L1064 | `internal/service/mobile_history.go:25:57` | string &#96;trans-bin.zaloapp.com&#96; has 3 occurrences, make it a constant |
| L1065 | `internal/service/mobile_ledger_control.go:18:51` | string &#96;cli_prepare_mobile_backup&#96; has 3 occurrences, make it a constant |
| L1066 | `internal/service/mobile_ledger_control.go:20:13` | string &#96;cli_cancel_prepared_mobile_backup&#96; has 3 occurrences, make it a constant |
| L1067 | `internal/service/mobile_ledger_control.go:21:14` | string &#96;cli_probe_mobile_backup_offer&#96; has 5 occurrences, make it a constant |
| L1068 | `internal/service/mobile_ledger_control.go:21:59` | string &#96;cli_probe_mobile_backup_archive&#96; has 6 occurrences, make it a constant |
| L1069 | `internal/service/mobile_ledger_control.go:106:50` | string &#96;/rpc&#96; has 3 occurrences, make it a constant |
| L1070 | `internal/service/mobile_ledger_control.go:160:59` | string &#96;prepared&#96; has 6 occurrences, make it a constant |
| L1071 | `internal/service/mobile_offer_probe.go:20:60` | string &#96;prepared&#96; has 6 occurrences, make it a constant |
| L1072 | `internal/service/mobile_offer_probe.go:50:3` | string &#96;download_performed&#96; has 3 occurrences, make it a constant |
| L1073 | `internal/service/mobile_offer_probe.go:51:3` | string &#96;import_performed&#96; has 4 occurrences, make it a constant |
| L1074 | `internal/service/recall_diagnostic.go:43:64` | string &#96;/rpc&#96; has 3 occurrences, make it a constant |
| L1075 | `internal/service/recall_diagnostic.go:191:14` | string &#96;unknown&#96; has 3 occurrences, make it a constant |
| L1076 | `internal/service/run.go:118:173` | string &#96;cli_probe_recall&#96; has 4 occurrences, make it a constant |
| L1077 | `internal/service/run.go:461:4` | string &#96;authenticated&#96; has 4 occurrences, make it a constant |
| L1078 | `internal/service/run.go:462:4` | string &#96;collector_state&#96; has 4 occurrences, make it a constant |
| L1079 | `internal/service/run.go:463:4` | string &#96;last_connected_at&#96; has 4 occurrences, make it a constant |
| L1080 | `internal/service/run.go:464:4` | string &#96;last_event_at&#96; has 4 occurrences, make it a constant |
| L1081 | `internal/service/run.go:465:4` | string &#96;last_persisted_at&#96; has 4 occurrences, make it a constant |
| L1082 | `internal/service/run.go:466:4` | string &#96;last_error&#96; has 4 occurrences, make it a constant |
| L1083 | `internal/service/run.go:549:33` | string &#96;STORAGE_ERROR&#96; has 3 occurrences, make it a constant |
| L1084 | `internal/service/run.go:578:7` | string &#96;code&#96; has 3 occurrences, make it a constant |
| L1085 | `internal/service/run.go:579:7` | string &#96;message&#96; has 3 occurrences, make it a constant |
| L1086 | `internal/storage/browse.go:35:11` | string &#96;desc&#96; has 3 occurrences, make it a constant |
| L1087 | `internal/storage/browse.go:120:5` | string &#96;conversation_type&#96; has 4 occurrences, make it a constant |
| L1088 | `internal/storage/browse.go:121:5` | string &#96;conversation_id&#96; has 4 occurrences, make it a constant |
| L1089 | `internal/storage/browse.go:130:26` | string &#96;unknown&#96; has 7 occurrences, make it a constant |
| L1090 | `internal/storage/browse.go:149:21` | string &#96;outgoing&#96; has 4 occurrences, make it a constant |
| L1091 | `internal/storage/browse.go:151:21` | string &#96;incoming&#96; has 6 occurrences, make it a constant |
| L1092 | `internal/storage/browse.go:193:3` | string &#96;has_more&#96; has 4 occurrences, make it a constant |
| L1093 | `internal/storage/browse.go:194:3` | string &#96;next_cursor&#96; has 3 occurrences, make it a constant |
| L1094 | `internal/storage/browse.go:195:3` | string &#96;empty_reason&#96; has 3 occurrences, make it a constant |
| L1095 | `internal/storage/browse.go:196:3` | string &#96;coverage&#96; has 3 occurrences, make it a constant |
| L1096 | `internal/storage/contact_profiles.go:76:4` | string &#96;status&#96; has 3 occurrences, make it a constant |
| L1097 | `internal/storage/contact_profiles.go:76:23` | string &#96;not_attempted&#96; has 3 occurrences, make it a constant |
| L1098 | `internal/storage/contact_profiles.go:77:4` | string &#96;last_attempt_at&#96; has 3 occurrences, make it a constant |
| L1099 | `internal/storage/contact_profiles.go:78:4` | string &#96;last_success_at&#96; has 3 occurrences, make it a constant |
| L1100 | `internal/storage/contact_profiles.go:82:4` | string &#96;stop_reason&#96; has 3 occurrences, make it a constant |
| L1101 | `internal/storage/contacts.go:58:81` | string &#96;unknown&#96; has 7 occurrences, make it a constant |
| L1102 | `internal/storage/contacts.go:61:39` | string &#96;direct&#96; has 10 occurrences, make it a constant |
| L1103 | `internal/storage/contacts.go:139:4` | string &#96;status&#96; has 3 occurrences, make it a constant |
| L1104 | `internal/storage/contacts.go:139:23` | string &#96;not_attempted&#96; has 3 occurrences, make it a constant |
| L1105 | `internal/storage/contacts.go:140:4` | string &#96;last_attempt_at&#96; has 3 occurrences, make it a constant |
| L1106 | `internal/storage/contacts.go:141:4` | string &#96;last_success_at&#96; has 3 occurrences, make it a constant |
| L1107 | `internal/storage/contacts.go:144:4` | string &#96;stop_reason&#96; has 3 occurrences, make it a constant |
| L1108 | `internal/storage/conversation_views.go:97:3` | string &#96;history_complete&#96; has 3 occurrences, make it a constant |
| L1109 | `internal/storage/conversation_views.go:111:55` | string &#96;group&#96; has 9 occurrences, make it a constant |
| L1110 | `internal/storage/conversation_views.go:111:74` | string &#96;direct&#96; has 10 occurrences, make it a constant |
| L1111 | `internal/storage/conversation_views.go:179:6` | string &#96;conversation_type&#96; has 4 occurrences, make it a constant |
| L1112 | `internal/storage/conversation_views.go:180:6` | string &#96;conversation_id&#96; has 4 occurrences, make it a constant |
| L1113 | `internal/storage/conversation_views.go:185:6` | string &#96;updated_at&#96; has 3 occurrences, make it a constant |
| L1114 | `internal/storage/conversation_views.go:226:3` | string &#96;has_more&#96; has 4 occurrences, make it a constant |
| L1115 | `internal/storage/conversation_views.go:227:3` | string &#96;next_cursor&#96; has 3 occurrences, make it a constant |
| L1116 | `internal/storage/conversation_views.go:228:3` | string &#96;catalog_complete&#96; has 4 occurrences, make it a constant |
| L1117 | `internal/storage/conversation_views.go:230:3` | string &#96;empty_reason&#96; has 3 occurrences, make it a constant |
| L1118 | `internal/storage/conversation_views.go:258:3` | string &#96;conversation&#96; has 9 occurrences, make it a constant |
| L1119 | `internal/storage/conversation_views.go:272:3` | string &#96;coverage&#96; has 3 occurrences, make it a constant |
| L1120 | `internal/storage/conversation_views.go:292:10` | string &#96;all&#96; has 6 occurrences, make it a constant |
| L1121 | `internal/storage/conversations.go:13:27` | string &#96;direct&#96; has 10 occurrences, make it a constant |
| L1122 | `internal/storage/conversations.go:13:40` | string &#96;group&#96; has 9 occurrences, make it a constant |
| L1123 | `internal/storage/deliveries.go:166:13` | string &#96;pending&#96; has 7 occurrences, make it a constant |
| L1124 | `internal/storage/deliveries.go:171:21` | string &#96;cancelled&#96; has 5 occurrences, make it a constant |
| L1125 | `internal/storage/deliveries.go:173:21` | string &#96;failed&#96; has 8 occurrences, make it a constant |
| L1126 | `internal/storage/deliveries.go:337:22` | string &#96;delivered&#96; has 3 occurrences, make it a constant |
| L1127 | `internal/storage/diagnostics.go:24:4` | string &#96;pending&#96; has 7 occurrences, make it a constant |
| L1128 | `internal/storage/diagnostics.go:26:4` | string &#96;delivered&#96; has 3 occurrences, make it a constant |
| L1129 | `internal/storage/diagnostics.go:27:4` | string &#96;failed&#96; has 8 occurrences, make it a constant |
| L1130 | `internal/storage/diagnostics.go:28:4` | string &#96;cancelled&#96; has 5 occurrences, make it a constant |
| L1131 | `internal/storage/event_journal.go:55:15` | string &#96;conversation&#96; has 9 occurrences, make it a constant |
| L1132 | `internal/storage/event_journal.go:71:5` | string &#96;conversation_type&#96; has 4 occurrences, make it a constant |
| L1133 | `internal/storage/event_journal.go:72:5` | string &#96;conversation_id&#96; has 4 occurrences, make it a constant |
| L1134 | `internal/storage/event_journal.go:192:15` | string &#96;pending&#96; has 7 occurrences, make it a constant |
| L1135 | `internal/storage/event_journal.go:210:15` | string &#96;delivered&#96; has 3 occurrences, make it a constant |
| L1136 | `internal/storage/event_journal.go:247:3` | string &#96;has_more&#96; has 4 occurrences, make it a constant |
| L1137 | `internal/storage/events.go:170:18` | string &#96;conversation&#96; has 9 occurrences, make it a constant |
| L1138 | `internal/storage/events.go:298:15` | string &#96;PERMISSION_DENIED&#96; has 3 occurrences, make it a constant |
| L1139 | `internal/storage/history.go:54:42` | string &#96;incoming&#96; has 6 occurrences, make it a constant |
| L1140 | `internal/storage/history.go:54:71` | string &#96;outgoing&#96; has 4 occurrences, make it a constant |
| L1141 | `internal/storage/history.go:54:100` | string &#96;unknown&#96; has 7 occurrences, make it a constant |
| L1142 | `internal/storage/history_operations.go:215:25` | string &#96;queued&#96; has 6 occurrences, make it a constant |
| L1143 | `internal/storage/history_operations.go:221:24` | string &#96;conversation_preload&#96; has 3 occurrences, make it a constant |
| L1144 | `internal/storage/history_operations.go:224:37` | string &#96;unsupported&#96; has 5 occurrences, make it a constant |
| L1145 | `internal/storage/history_operations.go:278:83` | string &#96;paused&#96; has 3 occurrences, make it a constant |
| L1146 | `internal/storage/history_operations.go:284:43` | string &#96;running&#96; has 13 occurrences, make it a constant |
| L1147 | `internal/storage/history_operations.go:660:13` | string &#96;failed&#96; has 8 occurrences, make it a constant |
| L1148 | `internal/storage/incoming.go:43:17` | string &#96;unknown&#96; has 7 occurrences, make it a constant |
| L1149 | `internal/storage/incoming.go:45:20` | string &#96;incoming&#96; has 6 occurrences, make it a constant |
| L1150 | `internal/storage/incoming.go:45:49` | string &#96;outgoing&#96; has 4 occurrences, make it a constant |
| L1151 | `internal/storage/ingestion.go:30:35` | string &#96;direct&#96; has 10 occurrences, make it a constant |
| L1152 | `internal/storage/ingestion.go:30:75` | string &#96;group&#96; has 9 occurrences, make it a constant |
| L1153 | `internal/storage/mobile_backup.go:155:16` | string &#96;prepared&#96; has 4 occurrences, make it a constant |
| L1154 | `internal/storage/mobile_backup.go:216:11` | string &#96;failed&#96; has 8 occurrences, make it a constant |
| L1155 | `internal/storage/mobile_backup.go:216:29` | string &#96;cancelled&#96; has 5 occurrences, make it a constant |
| L1156 | `internal/storage/mobile_backup.go:216:50` | string &#96;interrupted&#96; has 3 occurrences, make it a constant |
| L1157 | `internal/storage/mobile_backup.go:217:40` | string &#96;dispatching&#96; has 5 occurrences, make it a constant |
| L1158 | `internal/storage/mobile_backup.go:217:65` | string &#96;waiting_for_confirmation&#96; has 3 occurrences, make it a constant |
| L1159 | `internal/storage/mobile_backup.go:218:12` | string &#96;request_result_unknown&#96; has 3 occurrences, make it a constant |
| L1160 | `internal/storage/mobile_backup.go:219:12` | string &#96;mobile_restoring&#96; has 3 occurrences, make it a constant |
| L1161 | `internal/storage/mobile_backup.go:220:12` | string &#96;waiting_for_backup&#96; has 3 occurrences, make it a constant |
| L1162 | `internal/storage/mobile_history.go:175:51` | string &#96;running&#96; has 13 occurrences, make it a constant |
| L1163 | `internal/storage/mobile_history_acquisition.go:46:51` | string &#96;running&#96; has 13 occurrences, make it a constant |
| L1164 | `internal/storage/mobile_history_acquisition.go:200:81` | string &#96;queued&#96; has 6 occurrences, make it a constant |
| L1165 | `internal/storage/mobile_history_acquisition.go:216:11` | string &#96;interrupted&#96; has 3 occurrences, make it a constant |
| L1166 | `internal/storage/mobile_history_acquisition.go:217:24` | string &#96;cancelled&#96; has 5 occurrences, make it a constant |
| L1167 | `internal/storage/preload_status.go:33:4` | string &#96;status&#96; has 3 occurrences, make it a constant |
| L1168 | `internal/storage/preload_status.go:33:25` | string &#96;not_attempted&#96; has 3 occurrences, make it a constant |
| L1169 | `internal/storage/preload_status.go:34:4` | string &#96;last_attempt_at&#96; has 3 occurrences, make it a constant |
| L1170 | `internal/storage/preload_status.go:35:4` | string &#96;last_success_at&#96; has 3 occurrences, make it a constant |
| L1171 | `internal/storage/preload_status.go:38:4` | string &#96;catalog_complete&#96; has 4 occurrences, make it a constant |
| L1172 | `internal/storage/preload_status.go:40:4` | string &#96;stop_reason&#96; has 3 occurrences, make it a constant |
| L1173 | `internal/storage/read_authority.go:58:16` | string &#96;PERMISSION_DENIED&#96; has 3 occurrences, make it a constant |
| L1174 | `internal/storage/sending.go:135:14` | string &#96;sent&#96; has 4 occurrences, make it a constant |
| L1175 | `internal/storage/sending.go:135:33` | string &#96;failed&#96; has 8 occurrences, make it a constant |
| L1176 | `internal/storage/sending.go:135:54` | string &#96;unknown&#96; has 7 occurrences, make it a constant |
| L1177 | `internal/storage/sending.go:149:8` | string &#96;interrupted&#96; has 3 occurrences, make it a constant |
| L1178 | `internal/storage/store.go:435:16` | string &#96;PERMISSION_DENIED&#96; has 3 occurrences, make it a constant |
| L1179 | `internal/storage/store.go:577:55` | string &#96;group&#96; has 9 occurrences, make it a constant |
| L1180 | `internal/storage/store.go:577:88` | string &#96;direct&#96; has 10 occurrences, make it a constant |
| L1181 | `internal/storage/subscription_scope.go:10:13` | string &#96;conversation&#96; has 9 occurrences, make it a constant |
| L1182 | `internal/storage/subscription_scope.go:27:7` | string &#96;all&#96; has 6 occurrences, make it a constant |
| L1183 | `internal/storage/subscription_scope.go:31:7` | string &#96;direct&#96; has 10 occurrences, make it a constant |
| L1184 | `internal/storage/subscription_scope.go:31:17` | string &#96;group&#96; has 9 occurrences, make it a constant |
| L1185 | `internal/storage/subscription_scope.go:41:44` | string &#96;incoming&#96; has 6 occurrences, make it a constant |
| L1186 | `internal/storage/subscription_scope.go:41:73` | string &#96;outgoing&#96; has 4 occurrences, make it a constant |
| L1187 | `internal/storage/views.go:111:3` | string &#96;has_more&#96; has 4 occurrences, make it a constant |
| L1188 | `internal/storage/views.go:112:3` | string &#96;next_cursor&#96; has 3 occurrences, make it a constant |
| L1189 | `internal/storage/views.go:113:3` | string &#96;empty_reason&#96; has 3 occurrences, make it a constant |
| L1190 | `internal/storage/views.go:130:3` | string &#96;group&#96; has 9 occurrences, make it a constant |
| L1191 | `internal/storage/views.go:133:3` | string &#96;updated_at&#96; has 3 occurrences, make it a constant |
| L1192 | `internal/storage/views.go:262:3` | string &#96;history_complete&#96; has 3 occurrences, make it a constant |
| L1193 | `internal/storage/views.go:364:3` | string &#96;coverage&#96; has 3 occurrences, make it a constant |
| L1194 | `internal/toolruntime/outcome.go:46:12` | string &#96;zalo_send_direct_message&#96; has 4 occurrences, make it a constant |
| L1195 | `internal/toolruntime/outcome.go:46:50` | string &#96;zalo_join_group&#96; has 4 occurrences, make it a constant |
| L1196 | `internal/toolruntime/outcome.go:46:79` | string &#96;zalo_import_conversation_history&#96; has 4 occurrences, make it a constant |
| L1197 | `internal/toolruntime/runtime.go:25:2` | string &#96;zalo_search_messages&#96; has 3 occurrences, make it a constant |
| L1198 | `internal/toolruntime/runtime.go:26:2` | string &#96;zalo_search_conversation_messages&#96; has 3 occurrences, make it a constant |
| L1199 | `internal/toolruntime/runtime.go:27:2` | string &#96;zalo_get_message_context&#96; has 3 occurrences, make it a constant |
| L1200 | `internal/toolruntime/runtime.go:28:2` | string &#96;zalo_get_conversation_message_context&#96; has 3 occurrences, make it a constant |
| L1201 | `internal/toolruntime/runtime.go:419:7` | string &#96;zalo_send_direct_message&#96; has 4 occurrences, make it a constant |
| L1202 | `internal/toolruntime/runtime.go:420:3` | string &#96;zalo_join_group&#96; has 4 occurrences, make it a constant |
| L1203 | `internal/toolruntime/runtime.go:421:3` | string &#96;zalo_import_conversation_history&#96; has 4 occurrences, make it a constant |
| L1204 | `internal/toolruntime/runtime.go:422:3` | string &#96;zalo_cancel_history_import&#96; has 3 occurrences, make it a constant |
| L1205 | `internal/toolruntime/runtime.go:423:3` | string &#96;zalo_ack_subscription_events&#96; has 3 occurrences, make it a constant |
| L1206 | `internal/zalo/adapter.go:291:16` | string &#96;incoming&#96; has 3 occurrences, make it a constant |
| L1207 | `internal/zalo/adapter.go:293:17` | string &#96;outgoing&#96; has 4 occurrences, make it a constant |
| L1208 | `internal/zalo/history.go:178:31` | string &#96;actionId&#96; has 3 occurrences, make it a constant |
| L1209 | `internal/zalo/history.go:178:52` | string &#96;cliMsgId&#96; has 3 occurrences, make it a constant |
| L1210 | `internal/zalo/history.go:178:89` | string &#96;userId&#96; has 3 occurrences, make it a constant |
| L1211 | `internal/zalo/history.go:178:99` | string &#96;realMsgId&#96; has 3 occurrences, make it a constant |
| L1212 | `internal/zalo/history.go:207:19` | string &#96;outgoing&#96; has 4 occurrences, make it a constant |
| L1213 | `internal/zalo/mobile_backup.go:154:24` | string &#96;Web&#96; has 3 occurrences, make it a constant |
| L1214 | `internal/zalo/preload.go:148:20` | string &#96;incoming&#96; has 3 occurrences, make it a constant |
| L1215 | `internal/zalo/preload.go:150:21` | string &#96;outgoing&#96; has 4 occurrences, make it a constant |
| L1216 | `internal/zalo/preload.go:194:31` | string &#96;actionId&#96; has 3 occurrences, make it a constant |
| L1217 | `internal/zalo/preload.go:194:52` | string &#96;cliMsgId&#96; has 3 occurrences, make it a constant |
| L1218 | `internal/zalo/preload.go:194:89` | string &#96;userId&#96; has 3 occurrences, make it a constant |
| L1219 | `internal/zalo/preload.go:194:99` | string &#96;realMsgId&#96; has 3 occurrences, make it a constant |
| L1220 | `tools/harness/internal/architecture/architecture.go:152:54` | string &#96;coverage&#96; has 6 occurrences, make it a constant |
| L1221 | `tools/harness/internal/architecture/architecture.go:286:36` | string &#96;database/sql&#96; has 3 occurrences, make it a constant |
| L1222 | `tools/harness/internal/architecture/architecture.go:286:52` | string &#96;dependency&#96; has 9 occurrences, make it a constant |
| L1223 | `tools/harness/internal/architecture/architecture.go:287:37` | string &#96;github.com/modelcontextprotocol/go-sdk/mcp&#96; has 3 occurrences, make it a constant |
| L1224 | `tools/harness/internal/architecture/architecture.go:294:76` | string &#96;positive&#96; has 6 occurrences, make it a constant |
| L1225 | `tools/harness/internal/architecture/architecture.go:297:27` | string &#96;internal/mobilebackup/archiveport&#96; has 3 occurrences, make it a constant |
| L1226 | `tools/harness/internal/architecture/architecture.go:377:61` | string &#96;PASS&#96; has 4 occurrences, make it a constant |
| L1227 | `tools/harness/internal/architecture/architecture.go:383:109` | string &#96;security&#96; has 3 occurrences, make it a constant |
| L1228 | `tools/harness/internal/performance/bench.go:19:47` | string &#96;status&#96; has 4 occurrences, make it a constant |
| L1229 | `tools/harness/internal/performance/bench.go:19:57` | string &#96;FAIL&#96; has 5 occurrences, make it a constant |
| L1230 | `tools/harness/internal/performance/bench.go:53:22` | string &#96;PASS&#96; has 10 occurrences, make it a constant |
| L1231 | `tools/harness/internal/performance/bench.go:65:45` | string &#96;system&#96; has 3 occurrences, make it a constant |
| L1232 | `tools/harness/internal/performance/bench.go:65:61` | string &#96;release&#96; has 3 occurrences, make it a constant |
| L1233 | `tools/harness/internal/performance/bench.go:65:78` | string &#96;machine&#96; has 4 occurrences, make it a constant |
| L1234 | `tools/harness/internal/performance/bench.go:98:80` | string &#96;model&#96; has 3 occurrences, make it a constant |
| L1235 | `tools/harness/internal/performance/bench.go:98:89` | string &#96;NOT_RUN&#96; has 6 occurrences, make it a constant |
| L1236 | `tools/harness/internal/performance/bench.go:98:100` | string &#96;live&#96; has 3 occurrences, make it a constant |
| L1237 | `tools/harness/internal/performance/bench.go:112:75` | string &#96;candidate&#96; has 6 occurrences, make it a constant |
| L1238 | `tools/harness/internal/performance/bench.go:112:105` | string &#96;./cmd/zl-mcp-eval&#96; has 5 occurrences, make it a constant |
| L1239 | `tools/harness/internal/performance/bench.go:147:267` | string &#96;fixture_sha256&#96; has 3 occurrences, make it a constant |
| L1240 | `tools/harness/internal/performance/bench.go:147:294` | string &#96;runs&#96; has 3 occurrences, make it a constant |
| L1241 | `tools/harness/internal/performance/bench.go:147:337` | string &#96;warmup_calls&#96; has 4 occurrences, make it a constant |
| L1242 | `tools/harness/internal/performance/bench.go:147:357` | string &#96;concurrency_per_kind&#96; has 4 occurrences, make it a constant |
| L1243 | `tools/harness/internal/performance/bench.go:147:384` | string &#96;parallel_kinds&#96; has 3 occurrences, make it a constant |
| L1244 | `tools/harness/internal/performance/bench.go:147:405` | string &#96;admission_enabled&#96; has 6 occurrences, make it a constant |
| L1245 | `tools/harness/internal/performance/bench.go:147:432` | string &#96;exporters_enabled&#96; has 6 occurrences, make it a constant |
| L1246 | `tools/harness/internal/performance/bench.go:147:460` | string &#96;pacing_millis&#96; has 6 occurrences, make it a constant |
| L1247 | `tools/harness/internal/performance/bench.go:194:32` | string &#96;p95_seconds&#96; has 4 occurrences, make it a constant |
| L1248 | `tools/harness/internal/performance/bench.go:194:47` | string &#96;allocations_per_op&#96; has 4 occurrences, make it a constant |
| L1249 | `tools/harness/internal/performance/bench.go:194:69` | string &#96;alloc_bytes_per_op&#96; has 4 occurrences, make it a constant |
| L1250 | `tools/harness/internal/performance/bench.go:204:240` | string &#96;errors&#96; has 4 occurrences, make it a constant |
| L1251 | `tools/harness/internal/performance/build.go:61:32` | string &#96;path&#96; has 3 occurrences, make it a constant |
| L1252 | `tools/harness/internal/performance/build.go:61:46` | string &#96;sha256&#96; has 3 occurrences, make it a constant |
| L1253 | `tools/harness/internal/performance/build.go:83:21` | string &#96;status&#96; has 4 occurrences, make it a constant |
| L1254 | `tools/harness/internal/performance/build.go:83:31` | string &#96;FAIL&#96; has 5 occurrences, make it a constant |
| L1255 | `tools/harness/internal/performance/build.go:83:58` | string &#96;model&#96; has 3 occurrences, make it a constant |
| L1256 | `tools/harness/internal/performance/build.go:83:67` | string &#96;NOT_RUN&#96; has 6 occurrences, make it a constant |
| L1257 | `tools/harness/internal/performance/build.go:83:78` | string &#96;live&#96; has 3 occurrences, make it a constant |
| L1258 | `tools/harness/internal/performance/build.go:106:62` | string &#96;./cmd/zl-mcp-eval&#96; has 5 occurrences, make it a constant |
| L1259 | `tools/harness/internal/performance/build.go:110:28` | string &#96;candidate&#96; has 6 occurrences, make it a constant |
| L1260 | `tools/harness/internal/performance/build.go:135:24` | string &#96;PASS&#96; has 10 occurrences, make it a constant |
| L1261 | `tools/harness/internal/performance/compare.go:20:31` | string &#96;fixture_sha256&#96; has 3 occurrences, make it a constant |
| L1262 | `tools/harness/internal/performance/compare.go:25:31` | string &#96;system&#96; has 3 occurrences, make it a constant |
| L1263 | `tools/harness/internal/performance/compare.go:25:41` | string &#96;release&#96; has 3 occurrences, make it a constant |
| L1264 | `tools/harness/internal/performance/compare.go:25:52` | string &#96;machine&#96; has 4 occurrences, make it a constant |
| L1265 | `tools/harness/internal/performance/compare.go:30:21` | string &#96;runs&#96; has 3 occurrences, make it a constant |
| L1266 | `tools/harness/internal/performance/compare.go:30:32` | string &#96;warmup_calls&#96; has 4 occurrences, make it a constant |
| L1267 | `tools/harness/internal/performance/compare.go:30:52` | string &#96;concurrency_per_kind&#96; has 4 occurrences, make it a constant |
| L1268 | `tools/harness/internal/performance/compare.go:30:79` | string &#96;parallel_kinds&#96; has 3 occurrences, make it a constant |
| L1269 | `tools/harness/internal/performance/compare.go:30:100` | string &#96;admission_enabled&#96; has 6 occurrences, make it a constant |
| L1270 | `tools/harness/internal/performance/compare.go:30:127` | string &#96;exporters_enabled&#96; has 6 occurrences, make it a constant |
| L1271 | `tools/harness/internal/performance/compare.go:30:155` | string &#96;pacing_millis&#96; has 6 occurrences, make it a constant |
| L1272 | `tools/harness/internal/performance/compare.go:60:30` | string &#96;p95_seconds&#96; has 4 occurrences, make it a constant |
| L1273 | `tools/harness/internal/performance/compare.go:60:45` | string &#96;allocations_per_op&#96; has 4 occurrences, make it a constant |
| L1274 | `tools/harness/internal/performance/compare.go:60:67` | string &#96;alloc_bytes_per_op&#96; has 4 occurrences, make it a constant |
| L1275 | `tools/harness/internal/performance/compare.go:65:49` | string &#96;errors&#96; has 4 occurrences, make it a constant |
| L1276 | `tools/harness/internal/performance/compare.go:100:24` | string &#96;PASS&#96; has 10 occurrences, make it a constant |
| L1277 | `tools/harness/internal/performance/compare.go:108:13` | string &#96;./cmd/zl-mcp-eval&#96; has 5 occurrences, make it a constant |
| L1278 | `tools/harness/internal/performance/compare.go:118:13` | string &#96;candidate&#96; has 6 occurrences, make it a constant |
| L1279 | `tools/harness/internal/performance/compare.go:145:16` | string &#96;status&#96; has 4 occurrences, make it a constant |
| L1280 | `tools/harness/internal/performance/compare.go:145:26` | string &#96;FAIL&#96; has 5 occurrences, make it a constant |
| L1281 | `tools/harness/internal/performance/compare.go:145:260` | string &#96;model&#96; has 3 occurrences, make it a constant |
| L1282 | `tools/harness/internal/performance/compare.go:145:269` | string &#96;NOT_RUN&#96; has 6 occurrences, make it a constant |
| L1283 | `tools/harness/internal/performance/compare.go:145:280` | string &#96;live&#96; has 3 occurrences, make it a constant |
| L1284 | `tools/harness/internal/performance/compare.go:273:34` | string &#96;path&#96; has 3 occurrences, make it a constant |
| L1285 | `tools/harness/internal/performance/compare.go:273:52` | string &#96;sha256&#96; has 3 occurrences, make it a constant |

### gocritic (9)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L1286/L1287/L1290–L1294: заменить if-else цепи на condition switch с тем же порядком и short-circuit. L1288: (*counter)++ допустимо. L1289 объединить else-if только сохранив два Remove — это проверка идемпотентности, см. L2513.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1286 | `internal/evaluation/grader.go:108:3` | ifElseChain: rewrite if-else to switch statement |
| L1287 | `internal/mobilebackup/account_metadata.go:102:3` | ifElseChain: rewrite if-else to switch statement |
| L1288 | `internal/mobilebackup/control_scan_test.go:76:4` | assignOp: replace &#96;*counter = *counter + 1&#96; with &#96;*counter++&#96; |
| L1289 | `internal/mobilebackup/retained_archive_test.go:197:11` | elseif: can replace 'else {if cond {}}' with 'else if cond {}' |
| L1290 | `internal/storage/event_journal.go:202:3` | ifElseChain: rewrite if-else to switch statement |
| L1291 | `internal/storage/history_operations.go:219:2` | ifElseChain: rewrite if-else to switch statement |
| L1292 | `internal/storage/history_operations.go:583:3` | ifElseChain: rewrite if-else to switch statement |
| L1293 | `internal/storage/mobile_history.go:236:2` | ifElseChain: rewrite if-else to switch statement |
| L1294 | `internal/storage/views.go:434:2` | ifElseChain: rewrite if-else to switch statement |

### gocyclo (72)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Закрывать теми же изменениями, что cyclop, без второго независимого рефакторинга. Для оправданного исключения перечислить только реально сработавшие complexity linters в одной директиве и дать предметную причину.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1295 | `cmd/zl-mcp-eval/main.go:16:1` | cyclomatic complexity 38 of func &#96;run&#96; is high (> 30) |
| L1296 | `internal/collector/run.go:55:1` | cyclomatic complexity 40 of func &#96;runSession&#96; is high (> 30) |
| L1297 | `internal/config/config.go:47:1` | cyclomatic complexity 34 of func &#96;Load&#96; is high (> 30) |
| L1298 | `internal/domain/history.go:59:1` | cyclomatic complexity 46 of func &#96;NewHistoryPageFailure&#96; is high (> 30) |
| L1299 | `internal/evalfixture/fixture.go:133:1` | cyclomatic complexity 47 of func &#96;New&#96; is high (> 30) |
| L1300 | `internal/evaluation/checkpoints.go:149:1` | cyclomatic complexity 111 of func &#96;GradeCheckpoint&#96; is high (> 30) |
| L1301 | `internal/evaluation/grader.go:19:1` | cyclomatic complexity 71 of func &#96;Grade&#96; is high (> 30) |
| L1302 | `internal/evaluation/runner.go:135:1` | cyclomatic complexity 61 of func &#96;Run&#96; is high (> 30) |
| L1303 | `internal/evaluation/scenarios.go:22:1` | cyclomatic complexity 70 of func &#96;RunCase&#96; is high (> 30) |
| L1304 | `internal/events/conversation_test.go:20:1` | cyclomatic complexity 36 of func &#96;TestConversationScopesSignedDeliveryAndCancellation&#96; is high (> 30) |
| L1305 | `internal/events/subscriptions.go:132:1` | cyclomatic complexity 36 of func &#96;(*SubscriptionManager).Call&#96; is high (> 30) |
| L1306 | `internal/historyimport/mcp_test.go:27:1` | cyclomatic complexity 34 of func &#96;TestMCPHistoryImportDiscoveryStatusAndReads&#96; is high (> 30) |
| L1307 | `internal/historyimport/mobile_worker.go:101:1` | cyclomatic complexity 59 of func &#96;runMobileOperationObserved&#96; is high (> 30) |
| L1308 | `internal/historyimport/worker.go:68:1` | cyclomatic complexity 40 of func &#96;runOperationObserved&#96; is high (> 30) |
| L1309 | `internal/mcpserver/conversation_events_http_test.go:34:1` | cyclomatic complexity 46 of func &#96;testConversationProfileTLS&#96; is high (> 30) |
| L1310 | `internal/mcpserver/conversations.go:17:1` | cyclomatic complexity 39 of func &#96;(*Service).addConversationResources&#96; is high (> 30) |
| L1311 | `internal/mcpserver/conversations_test.go:17:1` | cyclomatic complexity 43 of func &#96;TestConversationMCPSearchContextCatalogueAndFullText&#96; is high (> 30) |
| L1312 | `internal/mcpserver/http.go:110:1` | cyclomatic complexity 41 of func &#96;(*HTTPService).ServeHTTP&#96; is high (> 30) |
| L1313 | `internal/mcpserver/migration_baseline_test.go:56:1` | cyclomatic complexity 34 of func &#96;TestMigrationCatalogBaseline&#96; is high (> 30) |
| L1314 | `internal/mcpserver/pagination_test.go:18:1` | cyclomatic complexity 32 of func &#96;TestLargeWirePagesKeepAllResultsAndSearchSnapshot&#96; is high (> 30) |
| L1315 | `internal/mcpserver/server.go:61:1` | cyclomatic complexity 56 of func &#96;New&#96; is high (> 30) |
| L1316 | `internal/mcpserver/server.go:239:1` | cyclomatic complexity 39 of func &#96;(*Service).call&#96; is high (> 30) |
| L1317 | `internal/messaging/manager.go:38:1` | cyclomatic complexity 32 of func &#96;(*Manager).Call&#96; is high (> 30) |
| L1318 | `internal/mobilebackup/control_scan.go:30:1` | cyclomatic complexity 57 of func &#96;ReadOwnDirectRecallSet&#96; is high (> 30) |
| L1319 | `internal/mobilebackup/history_page.go:17:1` | cyclomatic complexity 42 of func &#96;(*SnapshotStore).ReadHistoryPage&#96; is high (> 30) |
| L1320 | `internal/mobilebackup/history_page_test.go:73:1` | cyclomatic complexity 42 of func &#96;TestSnapshotReaderConverterJournalResumeTogether&#96; is high (> 30) |
| L1321 | `internal/mobilebackup/history_worker_test.go:582:1` | cyclomatic complexity 35 of func &#96;TestMobileWorkerWholeControlPreludeResumesWithoutRedispatch&#96; is high (> 30) |
| L1322 | `internal/mobilebackup/message_conversion.go:91:1` | cyclomatic complexity 62 of func &#96;convertArchiveCandidates&#96; is high (> 30) |
| L1323 | `internal/mobilebackup/native_binary_test.go:18:1` | cyclomatic complexity 40 of func &#96;TestNativeBinaryTerminalStartupAcceptance&#96; is high (> 30) |
| L1324 | `internal/mobilebackup/prepared_page.go:44:1` | cyclomatic complexity 31 of func &#96;ReadPreparedArchivePage&#96; is high (> 30) |
| L1325 | `internal/mobilebackup/recall_history_test.go:14:1` | cyclomatic complexity 38 of func &#96;TestRecallSnapshotConverterJournalResumeTogether&#96; is high (> 30) |
| L1326 | `internal/mobilebackup/retained_archive.go:448:1` | cyclomatic complexity 39 of func &#96;(*RetainedArchiveStore).decode&#96; is high (> 30) |
| L1327 | `internal/mobilebackup/row_page.go:81:1` | cyclomatic complexity 56 of func &#96;PrepareRowPage&#96; is high (> 30) |
| L1328 | `internal/mobilebackup/snapshot.go:278:1` | cyclomatic complexity 35 of func &#96;(*SnapshotStore).Save&#96; is high (> 30) |
| L1329 | `internal/mobilebackup/snapshot_page.go:76:1` | cyclomatic complexity 41 of func &#96;(AccountArchive).ReadSnapshotPage&#96; is high (> 30) |
| L1330 | `internal/mobilebackup/snapshot_page_test.go:108:1` | cyclomatic complexity 36 of func &#96;TestSnapshotTextProjectionIdentityAuthorsAndGaps&#96; is high (> 30) |
| L1331 | `internal/mobilebackup/snapshot_sqlite_test.go:13:1` | cyclomatic complexity 42 of func &#96;TestSnapshotSQLiteZeroIDsPagingAndImportIsolation&#96; is high (> 30) |
| L1332 | `internal/mobilebackup/snapshot_terminal.go:15:1` | cyclomatic complexity 41 of func &#96;(*SnapshotStore).CleanupTerminal&#96; is high (> 30) |
| L1333 | `internal/mobilebackup/sqlite.go:169:1` | cyclomatic complexity 86 of func &#96;readSQLitePageMode&#96; is high (> 30) |
| L1334 | `internal/mobilebackup/xz.go:53:1` | cyclomatic complexity 42 of func &#96;inspectXZ&#96; is high (> 30) |
| L1335 | `internal/service/account_archive_control.go:24:1` | cyclomatic complexity 39 of func &#96;(*membershipPort).accountArchiveControl&#96; is high (> 30) |
| L1336 | `internal/service/account_archive_test.go:51:1` | cyclomatic complexity 69 of func &#96;testAccountArchiveCapture&#96; is high (> 30) |
| L1337 | `internal/service/archive_catalog.go:43:1` | cyclomatic complexity 33 of func &#96;(*membershipPort).archiveConversations&#96; is high (> 30) |
| L1338 | `internal/service/archive_messages.go:156:1` | cyclomatic complexity 31 of func &#96;(*membershipPort).archiveMessages&#96; is high (> 30) |
| L1339 | `internal/service/archive_messages_test.go:20:1` | cyclomatic complexity 31 of func &#96;testOfflineArchiveMessages&#96; is high (> 30) |
| L1340 | `internal/service/cli.go:28:1` | cyclomatic complexity 34 of func &#96;RunCLI&#96; is high (> 30) |
| L1341 | `internal/service/cli_mobile_ledger.go:29:1` | cyclomatic complexity 32 of func &#96;mobileLedgerArguments&#96; is high (> 30) |
| L1342 | `internal/service/conversations_test.go:122:1` | cyclomatic complexity 41 of func &#96;testConversationServiceProfile&#96; is high (> 30) |
| L1343 | `internal/service/events_restart_test.go:123:1` | cyclomatic complexity 49 of func &#96;TestServiceEventsSurviveRestartWithStableDeliveryAndBoundary&#96; is high (> 30) |
| L1344 | `internal/service/history_test.go:82:1` | cyclomatic complexity 32 of func &#96;TestHTTPHistoryToolsUseExistingCollectorSession&#96; is high (> 30) |
| L1345 | `internal/service/mobile_ledger_control.go:72:1` | cyclomatic complexity 38 of func &#96;(*membershipPort).mobileLedgerControl&#96; is high (> 30) |
| L1346 | `internal/service/mobile_page_test.go:103:1` | cyclomatic complexity 48 of func &#96;TestServiceMobilePageUsesCollectorSessionAndExpiresHistoricalRows&#96; is high (> 30) |
| L1347 | `internal/service/multiprocess_test.go:40:1` | cyclomatic complexity 67 of func &#96;TestTwoMCPProcessesReadCollectorWrites&#96; is high (> 30) |
| L1348 | `internal/service/recall_diagnostic.go:77:1` | cyclomatic complexity 51 of func &#96;(*membershipPort).probeRecall&#96; is high (> 30) |
| L1349 | `internal/service/run.go:154:1` | cyclomatic complexity 57 of func &#96;runConfigured&#96; is high (> 30) |
| L1350 | `internal/service/run_test.go:49:1` | cyclomatic complexity 48 of func &#96;TestServicePreservesMCPWhenAuthenticationIsRequired&#96; is high (> 30) |
| L1351 | `internal/service/sending_test.go:79:1` | cyclomatic complexity 46 of func &#96;TestServiceSendAndReplyUseCollectorSessionAndPersistAcrossRestart&#96; is high (> 30) |
| L1352 | `internal/storage/browse.go:18:1` | cyclomatic complexity 47 of func &#96;(*Store).Browse&#96; is high (> 30) |
| L1353 | `internal/storage/deliveries.go:61:1` | cyclomatic complexity 33 of func &#96;(*Store).FanoutProfileEvents&#96; is high (> 30) |
| L1354 | `internal/storage/events.go:159:1` | cyclomatic complexity 33 of func &#96;(*Store).ActivateSubscription&#96; is high (> 30) |
| L1355 | `internal/storage/history_operations.go:430:1` | cyclomatic complexity 35 of func &#96;(*Store).commitHistoryRecords&#96; is high (> 30) |
| L1356 | `internal/storage/history_operations_test.go:57:1` | cyclomatic complexity 35 of func &#96;TestHistoryOperationDurableCheckpointAndRequestIdentity&#96; is high (> 30) |
| L1357 | `internal/storage/mobile_history.go:158:1` | cyclomatic complexity 98 of func &#96;(*Store).CommitMobileHistoryPage&#96; is high (> 30) |
| L1358 | `internal/storage/mobile_history_test.go:133:1` | cyclomatic complexity 31 of func &#96;TestMobileHistoryRestartExactKeysetCountsAndSilentDuplicates&#96; is high (> 30) |
| L1359 | `internal/toolruntime/runtime.go:144:1` | cyclomatic complexity 45 of func &#96;New&#96; is high (> 30) |
| L1360 | `internal/toolruntime/runtime.go:464:1` | cyclomatic complexity 31 of func &#96;(*Runtime).dispatch&#96; is high (> 30) |
| L1361 | `internal/zalo/events.go:48:1` | cyclomatic complexity 33 of func &#96;(*Client).listenConversations&#96; is high (> 30) |
| L1362 | `internal/zalo/mobile_backup.go:60:1` | cyclomatic complexity 62 of func &#96;receiveMobileBackupOffer&#96; is high (> 30) |
| L1363 | `internal/zalo/preload.go:29:1` | cyclomatic complexity 51 of func &#96;readConversationPreload&#96; is high (> 30) |
| L1364 | `tools/harness/internal/performance/bench.go:75:1` | cyclomatic complexity 42 of func &#96;Bench&#96; is high (> 30) |
| L1365 | `tools/harness/internal/performance/compare.go:8:1` | cyclomatic complexity 56 of func &#96;validate&#96; is high (> 30) |
| L1366 | `tools/harness/internal/performance/compare.go:150:1` | cyclomatic complexity 39 of func &#96;Compare&#96; is high (> 30) |

### godoclint (8)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L1367–L1373: начать комментарий с имени объявленного символа, сохранить смысл контракта. L1374: заменить exec.Cmd.Wait на [exec.Cmd.Wait]. Без nolint.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1367 | `internal/app/eventaccess/journal.go:8:1` | godoc should start with symbol name ("Subscription") |
| L1368 | `internal/app/eventaccess/journal.go:43:1` | godoc should start with symbol name ("MarshalJSON") |
| L1369 | `internal/app/history/command.go:10:1` | godoc should start with symbol name ("StartRequest") |
| L1370 | `internal/evaluation/scenarios.go:357:1` | godoc should start with symbol name ("DefaultChildOptions") |
| L1371 | `internal/mobilebackup/archive_tokens.go:9:1` | godoc should start with symbol name ("SealArchiveToken") |
| L1372 | `internal/mobilebackup/sqlite.go:36:1` | godoc should start with symbol name ("SQLiteCoverage") |
| L1373 | `internal/storage/event_journal.go:136:1` | godoc should start with symbol name ("ReadSubscriptionEvents") |
| L1374 | `tools/harness/internal/performance/process.go:16:1` | text "exec.Cmd.Wait" should be replaced with "[exec.Cmd.Wait]" to link to stdlib method |

### golines (7)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Отформатировать перечисленные tools/harness файлы штатным formatter; затем проверить diff. Не менять SQL/JSON fixtures и shell literal contents ради ширины строки.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1376 | `tools/harness/cmd/harness/main.go:52:1` | File is not properly formatted |
| L1377 | `tools/harness/internal/architecture/architecture.go:163:1` | File is not properly formatted |
| L1378 | `tools/harness/internal/architecture/architecture_test.go:195:1` | File is not properly formatted |
| L1379 | `tools/harness/internal/performance/bench.go:32:1` | File is not properly formatted |
| L1380 | `tools/harness/internal/performance/build.go:67:1` | File is not properly formatted |
| L1381 | `tools/harness/internal/performance/compare.go:30:1` | File is not properly formatted |
| L1382 | `tools/harness/internal/performance/performance_test.go:61:1` | File is not properly formatted |

### gosec (151)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Разбор G115 и специальных security findings приведён выше в разделе «Адресные решения». G104 обрабатывать вместе с errcheck; не подавлять весь gosec на функции. G202 допускает локальный nolint только для фиксированных SQL fragments и placeholders для всех пользовательских значений. G702/G703/G204 проверять по происхождению binary/path/args. Права synthetic evidence и реальные credentials не объединять одним исключением.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1383 | `internal/collector/join.go:484:4` | G104: Errors unhandled |
| L1384 | `internal/collector/join.go:489:4` | G104: Errors unhandled |
| L1385 | `internal/collector/join.go:500:2` | G104: Errors unhandled |
| L1386 | `internal/collector/join.go:550:4` | G104: Errors unhandled |
| L1387 | `internal/collector/join.go:555:4` | G104: Errors unhandled |
| L1388 | `internal/collector/join.go:570:4` | G104: Errors unhandled |
| L1389 | `internal/collector/join.go:576:2` | G104: Errors unhandled |
| L1390 | `internal/collector/run.go:285:4` | G104: Errors unhandled |
| L1391 | `internal/collector/run.go:323:4` | G104: Errors unhandled |
| L1392 | `internal/collector/run.go:348:5` | G104: Errors unhandled |
| L1393 | `internal/collector/run.go:357:4` | G104: Errors unhandled |
| L1394 | `internal/collector/run.go:365:3` | G104: Errors unhandled |
| L1395 | `internal/config/config.go:226:30` | G115: integer overflow conversion int -> uint32 |
| L1396 | `internal/config/config.go:229:9` | G302: Expect file permissions to be 0600 or less |
| L1397 | `internal/evalfixture/fixture.go:158:4` | G104: Errors unhandled |
| L1398 | `internal/evalfixture/fixture.go:284:3` | G104: Errors unhandled |
| L1399 | `internal/evalfixture/offline.go:85:4` | G104: Errors unhandled |
| L1400 | `internal/evalfixture/offline.go:102:5` | G104: Errors unhandled |
| L1401 | `internal/evalfixture/offline.go:118:4` | G104: Errors unhandled |
| L1402 | `internal/evalfixture/offline.go:123:3` | G104: Errors unhandled |
| L1403 | `internal/evalfixture/offline.go:127:3` | G104: Errors unhandled |
| L1404 | `internal/evaluation/benchmark.go:89:3` | G104: Errors unhandled |
| L1405 | `internal/evaluation/client.go:27:3` | G104: Errors unhandled |
| L1406 | `internal/evaluation/client.go:32:28` | G104: Errors unhandled |
| L1407 | `internal/evaluation/client.go:32:47` | G104: Errors unhandled |
| L1408 | `internal/evaluation/grader.go:69:4` | G104: Errors unhandled |
| L1409 | `internal/evaluation/model.go:99:3` | G104: Errors unhandled |
| L1410 | `internal/evaluation/runner.go:280:7` | G104: Errors unhandled |
| L1411 | `internal/evaluation/scenarios.go:50:3` | G104: Errors unhandled |
| L1412 | `internal/events/worker.go:132:3` | G104: Errors unhandled |
| L1413 | `internal/local/files.go:18:3` | G104: Errors unhandled |
| L1414 | `internal/local/files.go:21:18` | G104: Errors unhandled |
| L1415 | `internal/local/files.go:21:57` | G104: Errors unhandled |
| L1416 | `internal/logging/file.go:37:48` | G115: integer overflow conversion int -> uint32 |
| L1417 | `internal/logging/file.go:40:12` | G302: Expect file permissions to be 0600 or less |
| L1418 | `internal/logging/file.go:45:92` | G115: integer overflow conversion int -> uint32 |
| L1419 | `internal/logging/startup.go:46:3` | G104: Errors unhandled |
| L1420 | `internal/logging/startup.go:52:3` | G104: Errors unhandled |
| L1421 | `internal/logging/startup.go:63:4` | G104: Errors unhandled |
| L1422 | `internal/mcpserver/http.go:73:2` | G104: Errors unhandled |
| L1423 | `internal/mcpserver/http.go:284:3` | G104: Errors unhandled |
| L1424 | `internal/mobilebackup/archive_library.go:28:2` | G104: Errors unhandled |
| L1425 | `internal/mobilebackup/archive_restore.go:30:4` | G104: Errors unhandled |
| L1426 | `internal/mobilebackup/archive_restore.go:42:3` | G104: Errors unhandled |
| L1427 | `internal/mobilebackup/archive_restore.go:46:2` | G104: Errors unhandled |
| L1428 | `internal/mobilebackup/attachment.go:98:14` | G115: integer overflow conversion uint32 -> int32 |
| L1429 | `internal/mobilebackup/checksum.go:32:13` | G115: integer overflow conversion int -> uint32 |
| L1430 | `internal/mobilebackup/download.go:138:80` | G115: integer overflow conversion uint64 -> int64 |
| L1431 | `internal/mobilebackup/encrypted_archive.go:44:64` | G115: integer overflow conversion uint64 -> int |
| L1432 | `internal/mobilebackup/header.go:56:26` | G115: integer overflow conversion uint64 -> int |
| L1433 | `internal/mobilebackup/mentions.go:58:13` | G115: integer overflow conversion uint32 -> int32 |
| L1434 | `internal/mobilebackup/quote.go:68:14` | G115: integer overflow conversion uint32 -> int32 |
| L1435 | `internal/mobilebackup/quote.go:76:13` | G115: integer overflow conversion uint64 -> int64 |
| L1436 | `internal/mobilebackup/retained_archive.go:117:3` | G104: Errors unhandled |
| L1437 | `internal/mobilebackup/retained_archive.go:123:3` | G104: Errors unhandled |
| L1438 | `internal/mobilebackup/retained_archive.go:128:3` | G104: Errors unhandled |
| L1439 | `internal/mobilebackup/retained_archive.go:133:3` | G104: Errors unhandled |
| L1440 | `internal/mobilebackup/retained_archive.go:149:3` | G104: Errors unhandled |
| L1441 | `internal/mobilebackup/retained_archive.go:302:42` | G115: integer overflow conversion int -> uint32 |
| L1442 | `internal/mobilebackup/retained_archive.go:463:3` | G104: Errors unhandled |
| L1443 | `internal/mobilebackup/retained_archive.go:467:2` | G104: Errors unhandled |
| L1444 | `internal/mobilebackup/retained_archive.go:544:2` | G104: Errors unhandled |
| L1445 | `internal/mobilebackup/snapshot.go:88:3` | G104: Errors unhandled |
| L1446 | `internal/mobilebackup/snapshot.go:94:3` | G104: Errors unhandled |
| L1447 | `internal/mobilebackup/snapshot.go:99:3` | G104: Errors unhandled |
| L1448 | `internal/mobilebackup/snapshot.go:104:3` | G104: Errors unhandled |
| L1449 | `internal/mobilebackup/snapshot.go:117:3` | G104: Errors unhandled |
| L1450 | `internal/mobilebackup/snapshot.go:137:3` | G104: Errors unhandled |
| L1451 | `internal/mobilebackup/snapshot.go:141:2` | G104: Errors unhandled |
| L1452 | `internal/mobilebackup/snapshot.go:178:3` | G104: Errors unhandled |
| L1453 | `internal/mobilebackup/snapshot.go:202:3` | G104: Errors unhandled |
| L1454 | `internal/mobilebackup/snapshot.go:239:3` | G104: Errors unhandled |
| L1455 | `internal/mobilebackup/snapshot.go:243:2` | G104: Errors unhandled |
| L1456 | `internal/mobilebackup/snapshot.go:343:42` | G115: integer overflow conversion int -> uint32 |
| L1457 | `internal/mobilebackup/snapshot.go:468:3` | G104: Errors unhandled |
| L1458 | `internal/mobilebackup/snapshot.go:472:2` | G104: Errors unhandled |
| L1459 | `internal/mobilebackup/snapshot.go:512:2` | G104: Errors unhandled |
| L1460 | `internal/mobilebackup/snapshot_terminal.go:31:2` | G104: Errors unhandled |
| L1461 | `internal/mobilebackup/sqlite.go:305:4` | G104: Errors unhandled |
| L1462 | `internal/mobilebackup/sqlite.go:310:4` | G104: Errors unhandled |
| L1463 | `internal/mobilebackup/sqlite.go:317:4` | G104: Errors unhandled |
| L1464 | `internal/mobilebackup/sqlite.go:322:2` | G104: Errors unhandled |
| L1465 | `internal/mobilebackup/sqlite.go:361:4` | G104: Errors unhandled |
| L1466 | `internal/mobilebackup/sqlite.go:367:2` | G104: Errors unhandled |
| L1467 | `internal/mobilebackup/xz.go:93:31` | G115: integer overflow conversion uint64 -> int |
| L1468 | `internal/mobilebackup/xz.go:103:16` | G115: integer overflow conversion uint64 -> int |
| L1469 | `internal/mobilebackup/xz.go:104:37` | G115: integer overflow conversion uint64 -> int |
| L1470 | `internal/mobilebackup/xz.go:108:33` | G115: integer overflow conversion int -> uint64 |
| L1471 | `internal/mobilebackup/xz.go:117:26` | G115: integer overflow conversion int -> uint64 |
| L1472 | `internal/mobilebackup/xz.go:122:63` | G115: integer overflow conversion int -> uint64 |
| L1473 | `internal/mobilebackup/xz.go:136:15` | G115: integer overflow conversion uint64 -> int |
| L1474 | `internal/mobilebackup/xz.go:158:28` | G115: integer overflow conversion int -> uint64 |
| L1475 | `internal/mobilebackup/xz.go:166:40` | G115: integer overflow conversion int -> uint64 |
| L1476 | `internal/service/mobile_offer_probe.go:49:52` | G115: integer overflow conversion int64 -> uint64 |
| L1477 | `internal/service/recall_diagnostic.go:152:2` | G104: Errors unhandled |
| L1478 | `internal/service/recall_diagnostic.go:183:2` | G104: Errors unhandled |
| L1479 | `internal/storage/browse.go:92:3` | G202: SQL string concatenation |
| L1480 | `internal/storage/browse.go:107:4` | G104: Errors unhandled |
| L1481 | `internal/storage/browse.go:137:2` | G104: Errors unhandled |
| L1482 | `internal/storage/conversation_views.go:150:4` | G104: Errors unhandled |
| L1483 | `internal/storage/conversation_views.go:160:4` | G104: Errors unhandled |
| L1484 | `internal/storage/conversation_views.go:196:2` | G104: Errors unhandled |
| L1485 | `internal/storage/deliveries.go:95:4` | G104: Errors unhandled |
| L1486 | `internal/storage/deliveries.go:101:3` | G104: Errors unhandled |
| L1487 | `internal/storage/deliveries.go:104:2` | G104: Errors unhandled |
| L1488 | `internal/storage/deliveries.go:150:5` | G104: Errors unhandled |
| L1489 | `internal/storage/deliveries.go:156:4` | G104: Errors unhandled |
| L1490 | `internal/storage/deliveries.go:159:3` | G104: Errors unhandled |
| L1491 | `internal/storage/event_journal.go:189:4` | G104: Errors unhandled |
| L1492 | `internal/storage/event_journal.go:223:5` | G104: Errors unhandled |
| L1493 | `internal/storage/event_journal.go:237:2` | G104: Errors unhandled |
| L1494 | `internal/storage/events.go:47:4` | G104: Errors unhandled |
| L1495 | `internal/storage/events.go:53:3` | G104: Errors unhandled |
| L1496 | `internal/storage/events.go:56:2` | G104: Errors unhandled |
| L1497 | `internal/storage/history_operations.go:568:4` | G104: Errors unhandled |
| L1498 | `internal/storage/history_operations.go:574:2` | G104: Errors unhandled |
| L1499 | `internal/storage/store.go:46:2` | G104: Errors unhandled |
| L1500 | `internal/storage/store.go:61:4` | G104: Errors unhandled |
| L1501 | `internal/storage/store.go:74:3` | G104: Errors unhandled |
| L1502 | `internal/storage/store.go:82:4` | G104: Errors unhandled |
| L1503 | `internal/storage/store.go:92:3` | G104: Errors unhandled |
| L1504 | `internal/storage/store.go:402:3` | G202: SQL string concatenation |
| L1505 | `internal/storage/store.go:407:34` | G202: SQL string concatenation |
| L1506 | `internal/storage/store.go:410:34` | G202: SQL string concatenation |
| L1507 | `internal/storage/store.go:657:3` | G202: SQL string concatenation |
| L1508 | `internal/storage/store.go:680:4` | G104: Errors unhandled |
| L1509 | `internal/storage/store.go:686:4` | G104: Errors unhandled |
| L1510 | `internal/storage/store.go:699:2` | G104: Errors unhandled |
| L1511 | `internal/storage/views.go:40:4` | G104: Errors unhandled |
| L1512 | `internal/storage/views.go:53:2` | G104: Errors unhandled |
| L1513 | `internal/storage/views.go:306:4` | G202: SQL string concatenation |
| L1514 | `internal/zalo/cookies.go:38:3` | G124: http.Cookie missing or has insecure Secure, HttpOnly, or SameSite attribute |
| L1515 | `tools/harness/internal/architecture/architecture.go:92:28` | G702: Command injection via taint analysis |
| L1516 | `tools/harness/internal/architecture/architecture.go:233:24` | G703: Path traversal via taint analysis |
| L1517 | `tools/harness/internal/architecture/architecture.go:406:3` | G104: Errors unhandled |
| L1518 | `tools/harness/internal/architecture/architecture.go:549:3` | G104: Errors unhandled |
| L1519 | `tools/harness/internal/performance/bench.go:32:17` | G306: Expect WriteFile permissions to be 0600 or less |
| L1520 | `tools/harness/internal/performance/bench.go:35:17` | G306: Expect WriteFile permissions to be 0600 or less |
| L1521 | `tools/harness/internal/performance/bench.go:95:9` | G301: Expect directory permissions to be 0750 or less |
| L1522 | `tools/harness/internal/performance/build.go:80:9` | G301: Expect directory permissions to be 0750 or less |
| L1523 | `tools/harness/internal/performance/build.go:109:13` | G306: Expect WriteFile permissions to be 0600 or less |
| L1524 | `tools/harness/internal/performance/json.go:57:9` | G301: Expect directory permissions to be 0750 or less |
| L1525 | `tools/harness/internal/performance/json.go:60:9` | G306: Expect WriteFile permissions to be 0600 or less |
| L1526 | `tools/harness/internal/performance/process.go:29:3` | G104: Errors unhandled |
| L1527 | `tools/harness/internal/performance/process.go:33:9` | G204: Subprocess launched with a potential tainted input or cmd arguments |
| L1528 | `tools/harness/internal/performance/process.go:41:3` | G104: Errors unhandled |
| L1529 | `tools/harness/internal/performance/process.go:42:3` | G104: Errors unhandled |
| L1530 | `tools/harness/internal/performance/process.go:45:2` | G104: Errors unhandled |
| L1531 | `tools/harness/internal/performance/process.go:46:2` | G104: Errors unhandled |
| L1532 | `tools/harness/internal/performance/process.go:98:3` | G104: Errors unhandled |
| L1533 | `tools/harness/internal/performance/process.go:99:3` | G104: Errors unhandled |

### govet (196)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** В snapshot замечания shadow; не заменять повсеместно := на =: это меняет scope и может повторить недавнюю регрессию. При реально перекрытом result/err/ctx использовать осмысленные имена (queryErr, buildCtx) либо сократить scope внешней переменной. Для обычного if err := operation(); err != nil с независимым внутренним err допустим локальный nolint:govet с причиной; поскольку он подавляет весь govet на строке, предпочитать переименование при одинаковой читаемости. Остальные vet diagnostics, если появятся на rerun, разбирать отдельно.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1534 | `cmd/zl-mcp-eval/main_test.go:50:5` | shadow: declaration of "err" shadows declaration at line 40 |
| L1535 | `cmd/zl-mcp-eval/main_test.go:68:5` | shadow: declaration of "err" shadows declaration at line 40 |
| L1536 | `cmd/zl-mcp-eval/main_test.go:163:12` | shadow: declaration of "err" shadows declaration at line 139 |
| L1537 | `docs/contracts/archive_metadata_diagnostics_test.go:17:5` | shadow: declaration of "err" shadows declaration at line 11 |
| L1538 | `docs/contracts/history_import_test.go:58:4` | shadow: declaration of "err" shadows declaration at line 10 |
| L1539 | `internal/collector/conversations_test.go:45:6` | shadow: declaration of "err" shadows declaration at line 36 |
| L1540 | `internal/collector/conversations_test.go:49:7` | shadow: declaration of "err" shadows declaration at line 36 |
| L1541 | `internal/collector/conversations_test.go:62:6` | shadow: declaration of "err" shadows declaration at line 36 |
| L1542 | `internal/collector/conversations_test.go:68:6` | shadow: declaration of "err" shadows declaration at line 36 |
| L1543 | `internal/collector/join.go:115:6` | shadow: declaration of "e" shadows declaration at line 82 |
| L1544 | `internal/collector/mobile_backup_test.go:55:7` | shadow: declaration of "err" shadows declaration at line 49 |
| L1545 | `internal/collector/preload_catalog.go:45:6` | shadow: declaration of "err" shadows declaration at line 21 |
| L1546 | `internal/collector/run.go:118:6` | shadow: declaration of "cancel" shadows declaration at line 63 |
| L1547 | `internal/collector/run.go:120:6` | shadow: declaration of "e" shadows declaration at line 62 |
| L1548 | `internal/collector/run.go:124:11` | shadow: declaration of "e" shadows declaration at line 62 |
| L1549 | `internal/collector/run.go:343:8` | shadow: declaration of "e" shadows declaration at line 295 |
| L1550 | `internal/collector/run.go:352:11` | shadow: declaration of "e" shadows declaration at line 295 |
| L1551 | `internal/events/conversation_test.go:39:6` | shadow: declaration of "err" shadows declaration at line 26 |
| L1552 | `internal/events/conversation_test.go:65:11` | shadow: declaration of "err" shadows declaration at line 26 |
| L1553 | `internal/events/conversation_test.go:127:9` | shadow: declaration of "err" shadows declaration at line 26 |
| L1554 | `internal/events/conversation_test.go:168:11` | shadow: declaration of "err" shadows declaration at line 26 |
| L1555 | `internal/events/incoming_test.go:41:6` | shadow: declaration of "err" shadows declaration at line 23 |
| L1556 | `internal/events/incoming_test.go:87:6` | shadow: declaration of "err" shadows declaration at line 23 |
| L1557 | `internal/events/incoming_test.go:108:9` | shadow: declaration of "err" shadows declaration at line 23 |
| L1558 | `internal/events/incoming_test.go:148:8` | shadow: declaration of "err" shadows declaration at line 23 |
| L1559 | `internal/events/message_test.go:35:6` | shadow: declaration of "err" shadows declaration at line 30 |
| L1560 | `internal/events/message_test.go:112:7` | shadow: declaration of "err" shadows declaration at line 97 |
| L1561 | `internal/events/subscriptions.go:243:5` | shadow: declaration of "err" shadows declaration at line 232 |
| L1562 | `internal/events/subscriptions_test.go:32:6` | shadow: declaration of "err" shadows declaration at line 20 |
| L1563 | `internal/events/subscriptions_test.go:41:9` | shadow: declaration of "err" shadows declaration at line 20 |
| L1564 | `internal/events/worker.go:88:7` | shadow: declaration of "err" shadows declaration at line 82 |
| L1565 | `internal/events/worker.go:241:11` | shadow: declaration of "err" shadows declaration at line 238 |
| L1566 | `internal/events/worker_test.go:50:5` | shadow: declaration of "err" shadows declaration at line 25 |
| L1567 | `internal/events/worker_test.go:84:6` | shadow: declaration of "err" shadows declaration at line 79 |
| L1568 | `internal/historyimport/mobile_worker.go:177:12` | shadow: declaration of "e" shadows declaration at line 175 |
| L1569 | `internal/historyimport/mobile_worker.go:214:8` | shadow: declaration of "err" shadows declaration at line 107 |
| L1570 | `internal/historyimport/mobile_worker.go:247:17` | shadow: declaration of "e" shadows declaration at line 240 |
| L1571 | `internal/historyimport/worker.go:84:6` | shadow: declaration of "err" shadows declaration at line 74 |
| L1572 | `internal/logging/file.go:40:5` | shadow: declaration of "err" shadows declaration at line 32 |
| L1573 | `internal/logging/file.go:43:11` | shadow: declaration of "err" shadows declaration at line 32 |
| L1574 | `internal/logging/file_test.go:25:9` | shadow: declaration of "err" shadows declaration at line 17 |
| L1575 | `internal/logging/file_test.go:30:5` | shadow: declaration of "err" shadows declaration at line 17 |
| L1576 | `internal/logging/file_test.go:50:9` | shadow: declaration of "err" shadows declaration at line 17 |
| L1577 | `internal/mcpserver/budget_error_test.go:60:11` | shadow: declaration of "err" shadows declaration at line 23 |
| L1578 | `internal/mcpserver/catalog_ports_test.go:75:11` | shadow: declaration of "err" shadows declaration at line 29 |
| L1579 | `internal/mcpserver/catalog_ports_test.go:84:11` | shadow: declaration of "err" shadows declaration at line 29 |
| L1580 | `internal/mcpserver/conversation_events_http_test.go:52:9` | shadow: declaration of "err" shadows declaration at line 38 |
| L1581 | `internal/mcpserver/conversations.go:29:13` | shadow: declaration of "err" shadows declaration at line 18 |
| L1582 | `internal/mcpserver/diagnostics_test.go:40:5` | shadow: declaration of "err" shadows declaration at line 18 |
| L1583 | `internal/mcpserver/http_test.go:91:9` | shadow: declaration of "err" shadows declaration at line 82 |
| L1584 | `internal/mcpserver/http_test.go:139:5` | shadow: declaration of "err" shadows declaration at line 82 |
| L1585 | `internal/mcpserver/http_test.go:164:6` | shadow: declaration of "err" shadows declaration at line 82 |
| L1586 | `internal/mcpserver/http_test.go:178:8` | shadow: declaration of "err" shadows declaration at line 82 |
| L1587 | `internal/mcpserver/http_test.go:181:8` | shadow: declaration of "err" shadows declaration at line 82 |
| L1588 | `internal/mcpserver/http_test.go:215:5` | shadow: declaration of "err" shadows declaration at line 82 |
| L1589 | `internal/mcpserver/http_test.go:228:8` | shadow: declaration of "err" shadows declaration at line 82 |
| L1590 | `internal/mcpserver/migration_baseline_test.go:32:6` | shadow: declaration of "err" shadows declaration at line 25 |
| L1591 | `internal/mcpserver/migration_baseline_test.go:100:5` | shadow: declaration of "err" shadows declaration at line 60 |
| L1592 | `internal/mcpserver/migration_baseline_test.go:105:5` | shadow: declaration of "err" shadows declaration at line 60 |
| L1593 | `internal/mcpserver/migration_inputs_test.go:117:5` | shadow: declaration of "err" shadows declaration at line 112 |
| L1594 | `internal/mcpserver/migration_inputs_test.go:120:5` | shadow: declaration of "err" shadows declaration at line 112 |
| L1595 | `internal/mcpserver/migration_inputs_test.go:130:8` | shadow: declaration of "err" shadows declaration at line 112 |
| L1596 | `internal/mcpserver/runtime_invocation_test.go:136:11` | shadow: declaration of "err" shadows declaration at line 55 |
| L1597 | `internal/mcpserver/sdk_identity_test.go:110:5` | shadow: declaration of "err" shadows declaration at line 105 |
| L1598 | `internal/mcpserver/stdio_test.go:46:5` | shadow: declaration of "err" shadows declaration at line 35 |
| L1599 | `internal/mcpserver/stdio_test.go:85:8` | shadow: declaration of "err" shadows declaration at line 35 |
| L1600 | `internal/messaging/manager.go:48:7` | shadow: declaration of "err" shadows declaration at line 39 |
| L1601 | `internal/messaging/subprocess_restart_test.go:58:12` | shadow: declaration of "err" shadows declaration at line 36 |
| L1602 | `internal/messaging/subprocess_restart_test.go:188:13` | shadow: declaration of "err" shadows declaration at line 182 |
| L1603 | `internal/mobilebackup/archive_library_test.go:56:11` | shadow: declaration of "err" shadows declaration at line 23 |
| L1604 | `internal/mobilebackup/archive_restore_test.go:37:9` | shadow: declaration of "err" shadows declaration at line 19 |
| L1605 | `internal/mobilebackup/archive_restore_test.go:90:10` | shadow: declaration of "err" shadows declaration at line 57 |
| L1606 | `internal/mobilebackup/native_binary_test.go:82:9` | shadow: declaration of "err" shadows declaration at line 30 |
| L1607 | `internal/mobilebackup/native_binary_test.go:129:10` | shadow: declaration of "err" shadows declaration at line 109 |
| L1608 | `internal/mobilebackup/retained_archive_test.go:265:12` | shadow: declaration of "err" shadows declaration at line 230 |
| L1609 | `internal/mobilebackup/row_page.go:176:13` | shadow: declaration of "e" shadows declaration at line 91 |
| L1610 | `internal/mobilebackup/snapshot_sqlite_test.go:86:11` | shadow: declaration of "err" shadows declaration at line 47 |
| L1611 | `internal/mobilebackup/snapshot_sqlite_test.go:106:12` | shadow: declaration of "err" shadows declaration at line 47 |
| L1612 | `internal/mobilebackup/snapshot_terminal.go:73:12` | shadow: declaration of "e" shadows declaration at line 60 |
| L1613 | `internal/mobilebackup/snapshot_terminal_test.go:131:5` | shadow: declaration of "err" shadows declaration at line 126 |
| L1614 | `internal/mobilebackup/sqlite.go:382:16` | shadow: declaration of "err" shadows declaration at line 177 |
| L1615 | `internal/mobilebackup/sqlite.go:392:13` | shadow: declaration of "err" shadows declaration at line 177 |
| L1616 | `internal/mobilebackup/sqlite_test.go:586:9` | shadow: declaration of "err" shadows declaration at line 574 |
| L1617 | `internal/observability/native_test.go:273:11` | shadow: declaration of "err" shadows declaration at line 262 |
| L1622 | `internal/service/account_archive_test.go:131:9` | shadow: declaration of "err" shadows declaration at line 59 |
| L1623 | `internal/service/account_archive_test.go:210:11` | shadow: declaration of "e" shadows declaration at line 69 |
| L1624 | `internal/service/archive_catalog_test.go:45:11` | shadow: declaration of "err" shadows declaration at line 18 |
| L1625 | `internal/service/archive_messages.go:203:10` | shadow: declaration of "err" shadows declaration at line 173 |
| L1626 | `internal/service/archive_messages_test.go:51:11` | shadow: declaration of "err" shadows declaration at line 33 |
| L1627 | `internal/service/events_restart_test.go:138:5` | shadow: declaration of "err" shadows declaration at line 126 |
| L1628 | `internal/service/events_restart_test.go:151:9` | shadow: declaration of "err" shadows declaration at line 126 |
| L1629 | `internal/service/events_restart_test.go:225:8` | shadow: declaration of "err" shadows declaration at line 126 |
| L1630 | `internal/service/events_restart_test.go:236:8` | shadow: declaration of "err" shadows declaration at line 126 |
| L1631 | `internal/service/events_restart_test.go:265:8` | shadow: declaration of "err" shadows declaration at line 126 |
| L1632 | `internal/service/mobile_archive_probe.go:123:18` | shadow: declaration of "e" shadows declaration at line 89 |
| L1633 | `internal/service/mobile_history_test.go:118:8` | shadow: declaration of "err" shadows declaration at line 49 |
| L1634 | `internal/service/mobile_ledger_control.go:46:9` | shadow: declaration of "err" shadows declaration at line 33 |
| L1635 | `internal/service/mobile_ledger_control_test.go:246:8` | shadow: declaration of "e" shadows declaration at line 212 |
| L1636 | `internal/service/mobile_page_test.go:150:8` | shadow: declaration of "e" shadows declaration at line 108 |
| L1637 | `internal/service/mobile_page_test.go:180:14` | shadow: declaration of "e" shadows declaration at line 108 |
| L1638 | `internal/service/multiprocess_test.go:60:5` | shadow: declaration of "err" shadows declaration at line 42 |
| L1639 | `internal/service/multiprocess_test.go:66:13` | shadow: declaration of "err" shadows declaration at line 42 |
| L1640 | `internal/service/multiprocess_test.go:82:8` | shadow: declaration of "err" shadows declaration at line 42 |
| L1641 | `internal/service/multiprocess_test.go:96:5` | shadow: declaration of "err" shadows declaration at line 42 |
| L1642 | `internal/service/multiprocess_test.go:120:13` | shadow: declaration of "err" shadows declaration at line 42 |
| L1643 | `internal/service/multiprocess_test.go:164:12` | shadow: declaration of "err" shadows declaration at line 42 |
| L1644 | `internal/service/multiprocess_test.go:173:11` | shadow: declaration of "err" shadows declaration at line 42 |
| L1645 | `internal/service/multiprocess_test.go:197:7` | shadow: declaration of "err" shadows declaration at line 42 |
| L1646 | `internal/service/multiprocess_test.go:221:11` | shadow: declaration of "err" shadows declaration at line 42 |
| L1647 | `internal/service/multiprocess_test.go:253:6` | shadow: declaration of "err" shadows declaration at line 42 |
| L1648 | `internal/service/multiprocess_test.go:267:7` | shadow: declaration of "err" shadows declaration at line 42 |
| L1649 | `internal/service/preload_probe_test.go:52:5` | shadow: declaration of "err" shadows declaration at line 44 |
| L1650 | `internal/service/preload_probe_test.go:66:8` | shadow: declaration of "err" shadows declaration at line 44 |
| L1651 | `internal/service/preload_probe_test.go:93:6` | shadow: declaration of "err" shadows declaration at line 44 |
| L1652 | `internal/service/recall_diagnostic.go:113:11` | shadow: declaration of "e" shadows declaration at line 80 |
| L1653 | `internal/service/recall_diagnostic.go:117:6` | shadow: declaration of "e" shadows declaration at line 113 |
| L1654 | `internal/service/recall_diagnostic.go:162:6` | shadow: declaration of "e" shadows declaration at line 80 |
| L1655 | `internal/service/run.go:186:7` | shadow: declaration of "err" shadows declaration at line 162 |
| L1656 | `internal/service/run.go:227:6` | shadow: declaration of "err" shadows declaration at line 162 |
| L1657 | `internal/service/run_test.go:69:7` | shadow: declaration of "err" shadows declaration at line 53 |
| L1658 | `internal/service/run_test.go:109:9` | shadow: declaration of "err" shadows declaration at line 53 |
| L1659 | `internal/service/run_test.go:139:15` | shadow: declaration of "err" shadows declaration at line 53 |
| L1660 | `internal/service/run_test.go:162:15` | shadow: declaration of "err" shadows declaration at line 53 |
| L1661 | `internal/service/run_test.go:202:7` | shadow: declaration of "err" shadows declaration at line 53 |
| L1662 | `internal/service/run_test.go:219:9` | shadow: declaration of "err" shadows declaration at line 53 |
| L1663 | `internal/service/run_test.go:297:5` | shadow: declaration of "err" shadows declaration at line 291 |
| L1664 | `internal/service/run_test.go:303:8` | shadow: declaration of "err" shadows declaration at line 291 |
| L1665 | `internal/service/run_test.go:321:7` | shadow: declaration of "err" shadows declaration at line 291 |
| L1666 | `internal/service/run_test.go:327:8` | shadow: declaration of "err" shadows declaration at line 291 |
| L1667 | `internal/service/run_test.go:351:7` | shadow: declaration of "err" shadows declaration at line 291 |
| L1668 | `internal/service/sending_test.go:112:9` | shadow: declaration of "err" shadows declaration at line 81 |
| L1669 | `internal/service/sending_test.go:123:8` | shadow: declaration of "err" shadows declaration at line 81 |
| L1670 | `internal/storage/archive_visibility_test.go:39:11` | shadow: declaration of "err" shadows declaration at line 16 |
| L1671 | `internal/storage/budget_test.go:54:7` | shadow: declaration of "err" shadows declaration at line 48 |
| L1672 | `internal/storage/contacts.go:76:9` | shadow: declaration of "err" shadows declaration at line 49 |
| L1673 | `internal/storage/conversation_migration_test.go:117:5` | shadow: declaration of "err" shadows declaration at line 111 |
| L1674 | `internal/storage/conversation_views.go:158:29` | shadow: declaration of "err" shadows declaration at line 125 |
| L1675 | `internal/storage/conversations_test.go:24:3` | shadow: declaration of "err" shadows declaration at line 15 |
| L1676 | `internal/storage/deliveries.go:125:7` | shadow: declaration of "err" shadows declaration at line 115 |
| L1677 | `internal/storage/deliveries.go:148:8` | shadow: declaration of "target" shadows declaration at line 145 |
| L1678 | `internal/storage/deliveries.go:283:7` | shadow: declaration of "err" shadows declaration at line 280 |
| L1679 | `internal/storage/deliveries.go:292:10` | shadow: declaration of "err" shadows declaration at line 280 |
| L1680 | `internal/storage/deliveries_test.go:28:13` | shadow: declaration of "err" shadows declaration at line 21 |
| L1681 | `internal/storage/deliveries_test.go:49:7` | shadow: declaration of "err" shadows declaration at line 21 |
| L1682 | `internal/storage/deliveries_test.go:109:13` | shadow: declaration of "err" shadows declaration at line 97 |
| L1683 | `internal/storage/deliveries_test.go:118:6` | shadow: declaration of "err" shadows declaration at line 97 |
| L1684 | `internal/storage/deliveries_test.go:133:8` | shadow: declaration of "err" shadows declaration at line 97 |
| L1685 | `internal/storage/deliveries_test.go:142:5` | shadow: declaration of "err" shadows declaration at line 97 |
| L1686 | `internal/storage/deliveries_test.go:158:5` | shadow: declaration of "err" shadows declaration at line 97 |
| L1687 | `internal/storage/deliveries_test.go:218:5` | shadow: declaration of "err" shadows declaration at line 212 |
| L1688 | `internal/storage/deliveries_test.go:225:8` | shadow: declaration of "err" shadows declaration at line 212 |
| L1689 | `internal/storage/deliveries_test.go:259:8` | shadow: declaration of "err" shadows declaration at line 250 |
| L1690 | `internal/storage/deliveries_test.go:267:5` | shadow: declaration of "err" shadows declaration at line 250 |
| L1691 | `internal/storage/delivery_policy_test.go:27:11` | shadow: declaration of "err" shadows declaration at line 16 |
| L1692 | `internal/storage/delivery_policy_test.go:33:10` | shadow: declaration of "err" shadows declaration at line 16 |
| L1693 | `internal/storage/delivery_policy_test.go:45:11` | shadow: declaration of "err" shadows declaration at line 16 |
| L1694 | `internal/storage/delivery_policy_test.go:50:8` | shadow: declaration of "err" shadows declaration at line 16 |
| L1695 | `internal/storage/delivery_policy_test.go:118:11` | shadow: declaration of "err" shadows declaration at line 107 |
| L1696 | `internal/storage/delivery_policy_test.go:124:5` | shadow: declaration of "err" shadows declaration at line 107 |
| L1697 | `internal/storage/delivery_policy_test.go:127:5` | shadow: declaration of "err" shadows declaration at line 107 |
| L1698 | `internal/storage/diagnostics.go:51:9` | shadow: declaration of "err" shadows declaration at line 33 |
| L1699 | `internal/storage/diagnostics_test.go:38:8` | shadow: declaration of "err" shadows declaration at line 28 |
| L1700 | `internal/storage/events.go:206:12` | shadow: declaration of "err" shadows declaration at line 177 |
| L1701 | `internal/storage/events_test.go:46:8` | shadow: declaration of "err" shadows declaration at line 41 |
| L1702 | `internal/storage/events_test.go:88:5` | shadow: declaration of "err" shadows declaration at line 59 |
| L1703 | `internal/storage/events_test.go:97:5` | shadow: declaration of "err" shadows declaration at line 59 |
| L1704 | `internal/storage/events_test.go:111:5` | shadow: declaration of "err" shadows declaration at line 59 |
| L1705 | `internal/storage/history_expiry_test.go:35:7` | shadow: declaration of "e" shadows declaration at line 16 |
| L1706 | `internal/storage/history_expiry_test.go:138:17` | shadow: declaration of "e" shadows declaration at line 113 |
| L1707 | `internal/storage/history_expiry_test.go:141:8` | shadow: declaration of "e" shadows declaration at line 113 |
| L1708 | `internal/storage/history_operations_test.go:403:9` | shadow: declaration of "err" shadows declaration at line 388 |
| L1709 | `internal/storage/history_operations_test.go:416:6` | shadow: declaration of "err" shadows declaration at line 388 |
| L1710 | `internal/storage/history_operations_test.go:462:5` | shadow: declaration of "err" shadows declaration at line 438 |
| L1711 | `internal/storage/incoming_migration_test.go:38:6` | shadow: declaration of "err" shadows declaration at line 16 |
| L1712 | `internal/storage/incoming_test.go:36:7` | shadow: declaration of "err" shadows declaration at line 17 |
| L1713 | `internal/storage/incoming_test.go:104:6` | shadow: declaration of "err" shadows declaration at line 91 |
| L1714 | `internal/storage/incoming_test.go:114:6` | shadow: declaration of "err" shadows declaration at line 91 |
| L1715 | `internal/storage/mobile_backup.go:125:13` | shadow: declaration of "err" shadows declaration at line 114 |
| L1716 | `internal/storage/preload_directory_test.go:46:5` | shadow: declaration of "err" shadows declaration at line 33 |
| L1717 | `internal/storage/preload_directory_test.go:53:5` | shadow: declaration of "err" shadows declaration at line 33 |
| L1718 | `internal/storage/preload_directory_test.go:62:6` | shadow: declaration of "err" shadows declaration at line 33 |
| L1719 | `internal/storage/preload_directory_test.go:70:5` | shadow: declaration of "err" shadows declaration at line 33 |
| L1720 | `internal/storage/preload_directory_test.go:92:6` | shadow: declaration of "err" shadows declaration at line 87 |
| L1721 | `internal/storage/store.go:303:8` | shadow: declaration of "err" shadows declaration at line 276 |
| L1722 | `internal/storage/views.go:304:9` | shadow: declaration of "e" shadows declaration at line 299 |
| L1723 | `internal/storage/views.go:347:6` | shadow: declaration of "e" shadows declaration at line 299 |
| L1724 | `internal/toolruntime/catalog_reads_test.go:42:11` | shadow: declaration of "err" shadows declaration at line 35 |
| L1725 | `internal/toolruntime/runtime.go:248:11` | shadow: declaration of "err" shadows declaration at line 222 |
| L1726 | `internal/zalo/mobile_backup.go:104:6` | shadow: declaration of "err" shadows declaration at line 80 |
| L1727 | `internal/zalo/mobile_backup.go:251:8` | shadow: declaration of "err" shadows declaration at line 242 |
| L1728 | `internal/zalo/restore_test.go:38:7` | shadow: declaration of "err" shadows declaration at line 34 |
| L1735 | `tools/harness/internal/performance/bench.go:101:15` | shadow: declaration of "e" shadows declaration at line 78 |
| L1736 | `tools/harness/internal/performance/bench.go:121:8` | shadow: declaration of "e" shadows declaration at line 101 |
| L1737 | `tools/harness/internal/performance/build.go:47:9` | shadow: declaration of "e" shadows declaration at line 30 |
| L1738 | `tools/harness/internal/performance/build.go:85:3` | shadow: declaration of "ctx" shadows declaration at line 71 |
| L1739 | `tools/harness/internal/performance/build.go:87:9` | shadow: declaration of "e" shadows declaration at line 72 |

### ineffassign (1)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L1740 (= L2508/L2541): заменить err = <-done в cancellation ветке на <-done; ожидание cmd.Wait обязательно, значение намеренно заменяется ctx.Err()/ErrOutputBudget. Не удалять receive.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1740 | `internal/evaluation/process.go:70:3` | ineffectual assignment to err |

### makezero (1)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L1743: prefix уже содержит 14-байтовый заголовок. Оставить append(prefix, body.Bytes()...) с локальным nolint:makezero — existing length is serialized header, body must follow it. Замена на make([]byte, 0, ...) потеряет заголовок.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1743 | `internal/mobilebackup/header_test.go:79:9` | append to slice &#96;prefix&#96; with non-zero initialized length |

### mirror (1)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L1744: использовать bytes.Contains для []byte вместо временного string, сохранив тот же needle и assertion.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1744 | `internal/mobilebackup/attachment_test.go:76:45` | avoid allocations with strings.Contains |

### mnd (388)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Timeouts, protocol limits, binary widths и budget numbers оформить предметными constants рядом с владельцем; использовать уже существующие constants. В boundary tests literals являются самим входом/ожиданием — локальный nolint:mnd либо узкое test исключение после согласования. Не создавать const three=3; не объединять разные лимиты только из-за совпадения числа. Performance thresholds сохранять буквально и без пересчёта baseline.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L1745 | `cmd/zl-mcp-eval/main.go:84:60` | Magic number: 20, in <argument> detected |
| L1746 | `cmd/zl-mcp-eval/main.go:103:17` | Magic number: 60, in <operation> detected |
| L1747 | `cmd/zl-mcp-eval/main.go:105:17` | Magic number: 180, in <operation> detected |
| L1748 | `internal/app/conversations/catalog.go:131:46` | Magic number: 5, in <argument> detected |
| L1749 | `internal/app/history/command.go:29:42` | Magic number: 35, in <argument> detected |
| L1750 | `internal/app/history/command.go:34:42` | Magic number: 35, in <argument> detected |
| L1751 | `internal/app/history/read.go:22:42` | Magic number: 35, in <argument> detected |
| L1752 | `internal/app/membership/command.go:19:42` | Magic number: 6, in <argument> detected |
| L1753 | `internal/app/membership/read.go:52:42` | Magic number: 6, in <argument> detected |
| L1754 | `internal/app/membership/read.go:58:42` | Magic number: 6, in <argument> detected |
| L1755 | `internal/app/messaging/command.go:16:42` | Magic number: 35, in <argument> detected |
| L1756 | `internal/app/messaging/read.go:22:42` | Magic number: 35, in <argument> detected |
| L1757 | `internal/app/shared/telemetry.go:72:43` | Magic number: 100, in <argument> detected |
| L1758 | `internal/collector/contact_profiles.go:27:45` | Magic number: 30, in <argument> detected |
| L1759 | `internal/collector/contact_profiles.go:67:65` | Magic number: 100, in <argument> detected |
| L1760 | `internal/collector/contacts.go:31:45` | Magic number: 30, in <argument> detected |
| L1761 | `internal/collector/contacts.go:64:48` | Magic number: 200, in <argument> detected |
| L1762 | `internal/collector/contacts.go:79:21` | Magic number: 200, in <condition> detected |
| L1763 | `internal/collector/contacts.go:102:21` | Magic number: 200, in <condition> detected |
| L1764 | `internal/collector/contacts.go:113:27` | Magic number: 5, in <argument> detected |
| L1765 | `internal/collector/join.go:113:35` | Magic number: 10, in <argument> detected |
| L1766 | `internal/collector/join.go:185:20` | Magic number: 32, in <argument> detected |
| L1767 | `internal/collector/join.go:307:50` | Magic number: 60000, in <assign> detected |
| L1768 | `internal/collector/join.go:325:25` | Magic number: 1000, in <assign> detected |
| L1769 | `internal/collector/join.go:382:45` | Magic number: 45, in <argument> detected |
| L1770 | `internal/collector/join.go:397:68` | Magic number: 5, in <argument> detected |
| L1771 | `internal/collector/join.go:443:48` | Magic number: 15, in <argument> detected |
| L1772 | `internal/collector/preload_catalog.go:19:45` | Magic number: 30, in <argument> detected |
| L1773 | `internal/collector/preload_catalog.go:74:27` | Magic number: 5, in <argument> detected |
| L1774 | `internal/collector/preload_probe.go:33:45` | Magic number: 30, in <argument> detected |
| L1775 | `internal/collector/run.go:118:44` | Magic number: 30, in <argument> detected |
| L1776 | `internal/collector/run.go:155:23` | Magic number: 10, in <argument> detected |
| L1777 | `internal/collector/run.go:157:35` | Magic number: 5, in <argument> detected |
| L1778 | `internal/collector/run.go:177:63` | Magic number: 5, in <argument> detected |
| L1779 | `internal/collector/run.go:264:24` | Magic number: 40, in <argument> detected |
| L1780 | `internal/collector/run.go:295:53` | Magic number: 16, in <argument> detected |
| L1781 | `internal/collector/run.go:295:57` | Magic number: 10, in <argument> detected |
| L1782 | `internal/collector/run.go:393:45` | Magic number: 5, in <argument> detected |
| L1783 | `internal/control/client.go:48:53` | Magic number: 20, in <argument> detected |
| L1784 | `internal/domain/mobile_backup_request.go:39:24` | Magic number: 512, in <operation> detected |
| L1785 | `internal/domain/mobile_backup_request.go:39:31` | Magic number: 20, in <operation> detected |
| L1786 | `internal/domain/mobile_backup_request.go:58:23` | Magic number: 64, in <operation> detected |
| L1787 | `internal/domain/mobile_backup_request.go:58:29` | Magic number: 20, in <operation> detected |
| L1788 | `internal/evalfixture/fixture.go:170:96` | Magic number: 90, in <argument> detected |
| L1789 | `internal/evalfixture/fixture.go:172:87` | Magic number: 90, in <argument> detected |
| L1790 | `internal/evalfixture/fixture.go:287:87` | Magic number: 5, in <assign> detected |
| L1791 | `internal/evalfixture/fixture.go:325:28` | Magic number: 10, in <argument> detected |
| L1792 | `internal/evalfixture/fixture.go:346:60` | Magic number: 5, in <argument> detected |
| L1793 | `internal/evalfixture/offline.go:76:87` | Magic number: 8, in <argument> detected |
| L1794 | `internal/evalfixture/offline.go:76:90` | Magic number: 20, in <argument> detected |
| L1795 | `internal/evalfixture/offline.go:247:49` | Magic number: 4, in <argument> detected |
| L1796 | `internal/evalfixture/offline.go:250:3` | Magic number: 8, in <argument> detected |
| L1797 | `internal/evalfixture/offline.go:250:6` | Magic number: 20, in <argument> detected |
| L1798 | `internal/evalfixture/offline.go:257:96` | Magic number: 8, in <argument> detected |
| L1799 | `internal/evalfixture/offline.go:257:99` | Magic number: 20, in <argument> detected |
| L1800 | `internal/evaluation/benchmark.go:42:17` | Magic number: 50, in <assign> detected |
| L1801 | `internal/evaluation/benchmark.go:47:17` | Magic number: 110, in <assign> detected |
| L1802 | `internal/evaluation/benchmark.go:51:13` | Magic number: 1000, in <condition> detected |
| L1803 | `internal/evaluation/benchmark.go:107:21` | Magic number: 110, in <argument> detected |
| L1804 | `internal/evaluation/benchmark.go:129:40` | Magic number: 99, in <operation> detected |
| L1805 | `internal/evaluation/client.go:37:20` | Magic number: 110, in <argument> detected |
| L1806 | `internal/evaluation/model.go:41:24` | Magic number: 30, in <operation> detected |
| L1807 | `internal/evaluation/model.go:104:22` | Magic number: 5, in <assign> detected |
| L1808 | `internal/evaluation/model.go:136:3` | Magic number: 4, in <argument> detected |
| L1809 | `internal/evaluation/model.go:136:6` | Magic number: 20, in <argument> detected |
| L1810 | `internal/evaluation/runner.go:113:45` | Magic number: 64, in <operation> detected |
| L1811 | `internal/evaluation/runner.go:113:49` | Magic number: 10, in <operation> detected |
| L1812 | `internal/evaluation/runner.go:152:27` | Magic number: 180, in <assign> detected |
| L1813 | `internal/evaluation/runner.go:153:27` | Magic number: 30, in <assign> detected |
| L1814 | `internal/evaluation/runner.go:168:27` | Magic number: 180, in <assign> detected |
| L1815 | `internal/evaluation/runner.go:169:27` | Magic number: 30, in <assign> detected |
| L1816 | `internal/evaluation/runner.go:197:50` | Magic number: 6, in <operation> detected |
| L1817 | `internal/evaluation/runner.go:217:32` | Magic number: 60, in <assign> detected |
| L1818 | `internal/evaluation/runner.go:218:32` | Magic number: 4, in <assign> detected |
| L1819 | `internal/evaluation/runner.go:218:37` | Magic number: 20, in <assign> detected |
| L1820 | `internal/evaluation/runner.go:219:32` | Magic number: 4, in <assign> detected |
| L1821 | `internal/evaluation/runner.go:219:37` | Magic number: 20, in <assign> detected |
| L1822 | `internal/evaluation/runner.go:248:16` | Magic number: 60, in <operation> detected |
| L1823 | `internal/evaluation/runner.go:295:63` | Magic number: 4, in <argument> detected |
| L1824 | `internal/evaluation/runner.go:295:66` | Magic number: 20, in <argument> detected |
| L1825 | `internal/evaluation/runner.go:354:20` | Magic number: 3, in <assign> detected |
| L1826 | `internal/evaluation/runner.go:355:20` | Magic number: 8192, in <assign> detected |
| L1827 | `internal/evaluation/runner.go:368:22` | Magic number: 5, in <assign> detected |
| L1828 | `internal/evaluation/scenarios.go:93:99` | Magic number: 10, in <assign> detected |
| L1829 | `internal/evaluation/scenarios.go:338:71` | Magic number: 20, in <assign> detected |
| L1830 | `internal/events/subscriptions.go:242:51` | Magic number: 10, in <argument> detected |
| L1831 | `internal/events/subscriptions.go:249:27` | Magic number: 512, in <condition> detected |
| L1832 | `internal/events/webhook.go:51:68` | Magic number: 10, in <argument> detected |
| L1833 | `internal/events/webhook.go:70:17` | Magic number: 262144, in <condition> detected |
| L1834 | `internal/events/webhook.go:102:33` | Magic number: 32, in <argument> detected |
| L1835 | `internal/events/webhook.go:116:56` | Magic number: 65537, in <argument> detected |
| L1836 | `internal/events/worker.go:63:51` | Magic number: 400001, in <argument> detected |
| L1837 | `internal/events/worker.go:63:61` | Magic number: 200000, in <operation> detected |
| L1838 | `internal/events/worker.go:64:49` | Magic number: 1000000, in <argument> detected |
| L1839 | `internal/events/worker.go:111:49` | Magic number: 10, in <argument> detected |
| L1840 | `internal/events/worker.go:136:16` | Magic number: 410, in <case> detected |
| L1841 | `internal/events/worker.go:138:16` | Magic number: 413, in <case> detected |
| L1842 | `internal/events/worker.go:162:77` | Magic number: 5, in <argument> detected |
| L1843 | `internal/events/worker.go:194:27` | Magic number: 5, in <argument> detected |
| L1844 | `internal/events/worker.go:195:30` | Magic number: 4, in <argument> detected |
| L1845 | `internal/events/worker.go:199:28` | Magic number: 250, in <argument> detected |
| L1846 | `internal/events/worker.go:224:26` | Magic number: 250, in <argument> detected |
| L1847 | `internal/historyimport/mobile_worker.go:42:27` | Magic number: 500, in <argument> detected |
| L1848 | `internal/historyimport/mobile_worker.go:53:61` | Magic number: 20, in <argument> detected |
| L1849 | `internal/historyimport/mobile_worker.go:177:37` | Magic number: 180, in <argument> detected |
| L1850 | `internal/historyimport/mobile_worker.go:207:36` | Magic number: 120, in <argument> detected |
| L1851 | `internal/historyimport/mobile_worker.go:240:37` | Magic number: 30, in <argument> detected |
| L1852 | `internal/historyimport/worker.go:31:27` | Magic number: 500, in <argument> detected |
| L1853 | `internal/historyimport/worker.go:34:55` | Magic number: 20, in <argument> detected |
| L1854 | `internal/historyimport/worker.go:112:35` | Magic number: 30, in <argument> detected |
| L1855 | `internal/local/files.go:30:17` | Magic number: 0600, in <argument> detected |
| L1856 | `internal/logging/startup.go:44:26` | Magic number: 5, in <argument> detected |
| L1857 | `internal/logging/startup.go:44:29` | Magic number: 3, in <argument> detected |
| L1858 | `internal/logging/startup.go:66:14` | Magic number: 10, in <argument> detected |
| L1859 | `internal/mcpserver/http.go:34:18` | Magic number: 32, in <condition> detected |
| L1860 | `internal/mcpserver/http.go:135:57` | Magic number: 256, in <argument> detected |
| L1861 | `internal/mcpserver/http.go:135:62` | Magic number: 10, in <argument> detected |
| L1862 | `internal/mcpserver/server.go:302:50` | Magic number: 1000, in <assign> detected |
| L1863 | `internal/messaging/manager.go:120:47` | Magic number: 30, in <argument> detected |
| L1864 | `internal/messaging/manager.go:139:63` | Magic number: 3, in <argument> detected |
| L1865 | `internal/mobilebackup/account_inspection.go:77:66` | Magic number: 50, in <argument> detected |
| L1866 | `internal/mobilebackup/archive_library.go:27:30` | Magic number: 32, in <argument> detected |
| L1867 | `internal/mobilebackup/archive_restore.go:45:43` | Magic number: 33, in <argument> detected |
| L1868 | `internal/mobilebackup/attachment.go:62:8` | Magic number: 40, in <case> detected |
| L1869 | `internal/mobilebackup/attachment.go:64:8` | Magic number: 41, in <case> detected |
| L1870 | `internal/mobilebackup/attachment.go:66:8` | Magic number: 42, in <case> detected |
| L1871 | `internal/mobilebackup/attachment.go:68:8` | Magic number: 43, in <case> detected |
| L1872 | `internal/mobilebackup/attachment.go:70:8` | Magic number: 44, in <case> detected |
| L1873 | `internal/mobilebackup/attachment.go:72:8` | Magic number: 45, in <case> detected |
| L1874 | `internal/mobilebackup/attachment.go:74:8` | Magic number: 46, in <case> detected |
| L1875 | `internal/mobilebackup/attachment.go:76:8` | Magic number: 47, in <case> detected |
| L1876 | `internal/mobilebackup/attachment.go:78:8` | Magic number: 48, in <case> detected |
| L1877 | `internal/mobilebackup/attachment.go:80:8` | Magic number: 49, in <case> detected |
| L1878 | `internal/mobilebackup/attachment.go:82:8` | Magic number: 50, in <case> detected |
| L1879 | `internal/mobilebackup/attachment.go:84:8` | Magic number: 66, in <case> detected |
| L1880 | `internal/mobilebackup/attachment.go:86:8` | Magic number: 67, in <case> detected |
| L1881 | `internal/mobilebackup/attachment.go:88:8` | Magic number: 68, in <case> detected |
| L1882 | `internal/mobilebackup/binnet.go:48:15` | Magic number: 6, in <condition> detected |
| L1883 | `internal/mobilebackup/binnet.go:57:15` | Magic number: 8, in <condition> detected |
| L1884 | `internal/mobilebackup/binnet.go:66:15` | Magic number: 7, in <condition> detected |
| L1885 | `internal/mobilebackup/checksum.go:18:15` | Magic number: 16, in <condition> detected |
| L1886 | `internal/mobilebackup/checksum.go:19:28` | Magic number: 606290984, in <argument> detected |
| L1887 | `internal/mobilebackup/checksum.go:19:62` | Magic number: 1640531535, in <argument> detected |
| L1888 | `internal/mobilebackup/checksum.go:20:107` | Magic number: 13, in <argument> detected |
| L1889 | `internal/mobilebackup/checksum.go:28:56` | Magic number: 7, in <argument> detected |
| L1890 | `internal/mobilebackup/checksum.go:28:83` | Magic number: 12, in <argument> detected |
| L1891 | `internal/mobilebackup/checksum.go:28:111` | Magic number: 18, in <argument> detected |
| L1892 | `internal/mobilebackup/checksum.go:34:69` | Magic number: 17, in <argument> detected |
| L1893 | `internal/mobilebackup/checksum.go:38:41` | Magic number: 11, in <argument> detected |
| L1894 | `internal/mobilebackup/checksum.go:40:12` | Magic number: 15, in <operation> detected |
| L1895 | `internal/mobilebackup/checksum.go:42:12` | Magic number: 13, in <operation> detected |
| L1896 | `internal/mobilebackup/checksum.go:44:12` | Magic number: 16, in <operation> detected |
| L1897 | `internal/mobilebackup/control_scan.go:50:45` | Magic number: 30, in <argument> detected |
| L1898 | `internal/mobilebackup/control_scan.go:57:52` | Magic number: 253402300800000, in <argument> detected |
| L1899 | `internal/mobilebackup/control_scan.go:70:8` | Magic number: 50, in <argument> detected |
| L1900 | `internal/mobilebackup/download.go:41:65` | Magic number: 120, in <argument> detected |
| L1901 | `internal/mobilebackup/header.go:76:17` | Magic number: 4, in <condition> detected |
| L1902 | `internal/mobilebackup/header.go:88:18` | Magic number: 4, in <condition> detected |
| L1903 | `internal/mobilebackup/history_page.go:89:83` | Magic number: 5000, in <argument> detected |
| L1904 | `internal/mobilebackup/id_diagnostics.go:22:15` | Magic number: 64, in <condition> detected |
| L1905 | `internal/mobilebackup/id_diagnostics.go:40:15` | Magic number: 20, in <condition> detected |
| L1906 | `internal/mobilebackup/mentions.go:43:8` | Magic number: 100, in <case> detected |
| L1907 | `internal/mobilebackup/mentions.go:45:8` | Magic number: 101, in <case> detected |
| L1908 | `internal/mobilebackup/mentions.go:47:8` | Magic number: 102, in <case> detected |
| L1909 | `internal/mobilebackup/mentions.go:49:8` | Magic number: 103, in <case> detected |
| L1910 | `internal/mobilebackup/message_conversion.go:118:41` | Magic number: 50, in <argument> detected |
| L1911 | `internal/mobilebackup/message_conversion.go:234:51` | Magic number: 6, in <argument> detected |
| L1912 | `internal/mobilebackup/message_conversion.go:256:51` | Magic number: 6, in <argument> detected |
| L1913 | `internal/mobilebackup/offer_runner.go:81:60` | Magic number: 15, in <argument> detected |
| L1914 | `internal/mobilebackup/page_walk.go:40:45` | Magic number: 120, in <argument> detected |
| L1915 | `internal/mobilebackup/quote.go:48:32` | Magic number: 6, in <argument> detected |
| L1916 | `internal/mobilebackup/quote.go:67:15` | Magic number: 4, in <condition> detected |
| L1917 | `internal/mobilebackup/quote.go:69:16` | Magic number: 80, in <condition> detected |
| L1918 | `internal/mobilebackup/quote.go:78:8` | Magic number: 81, in <case> detected |
| L1919 | `internal/mobilebackup/quote.go:80:8` | Magic number: 82, in <case> detected |
| L1920 | `internal/mobilebackup/quote.go:82:8` | Magic number: 84, in <case> detected |
| L1921 | `internal/mobilebackup/quote.go:84:8` | Magic number: 85, in <case> detected |
| L1922 | `internal/mobilebackup/quote_fields.go:61:8` | Magic number: 86, in <case> detected |
| L1923 | `internal/mobilebackup/quote_fields.go:63:8` | Magic number: 87, in <case> detected |
| L1924 | `internal/mobilebackup/quote_fields.go:65:8` | Magic number: 88, in <case> detected |
| L1925 | `internal/mobilebackup/quote_fields.go:67:8` | Magic number: 90, in <case> detected |
| L1926 | `internal/mobilebackup/retained_archive.go:292:17` | Magic number: 4, in <argument> detected |
| L1927 | `internal/mobilebackup/retained_archive.go:304:12` | Magic number: 4, in <operation> detected |
| L1928 | `internal/mobilebackup/retained_archive.go:323:75` | Magic number: 0600, in <argument> detected |
| L1929 | `internal/mobilebackup/retained_archive.go:454:95` | Magic number: 4, in <argument> detected |
| L1930 | `internal/mobilebackup/retained_archive.go:505:11` | Magic number: 4, in <operation> detected |
| L1931 | `internal/mobilebackup/retained_archive.go:543:28` | Magic number: 32, in <argument> detected |
| L1932 | `internal/mobilebackup/row_page.go:48:7` | Magic number: 3, in <case> detected |
| L1933 | `internal/mobilebackup/row_page.go:48:10` | Magic number: 4, in <case> detected |
| L1934 | `internal/mobilebackup/row_page.go:48:13` | Magic number: 31, in <case> detected |
| L1935 | `internal/mobilebackup/row_page.go:50:7` | Magic number: 6, in <case> detected |
| L1936 | `internal/mobilebackup/row_page.go:52:7` | Magic number: 10, in <case> detected |
| L1937 | `internal/mobilebackup/row_page.go:54:7` | Magic number: 12, in <case> detected |
| L1938 | `internal/mobilebackup/row_page.go:56:7` | Magic number: 15, in <case> detected |
| L1939 | `internal/mobilebackup/row_page.go:58:7` | Magic number: 18, in <case> detected |
| L1940 | `internal/mobilebackup/row_page.go:60:7` | Magic number: 19, in <case> detected |
| L1941 | `internal/mobilebackup/row_page.go:62:7` | Magic number: 22, in <case> detected |
| L1942 | `internal/mobilebackup/row_page.go:64:7` | Magic number: 23, in <case> detected |
| L1943 | `internal/mobilebackup/row_page.go:66:7` | Magic number: 24, in <case> detected |
| L1944 | `internal/mobilebackup/row_page.go:68:7` | Magic number: 53, in <case> detected |
| L1945 | `internal/mobilebackup/row_page.go:75:7` | Magic number: 20, in <case> detected |
| L1946 | `internal/mobilebackup/row_page.go:75:11` | Magic number: 21, in <case> detected |
| L1947 | `internal/mobilebackup/row_page.go:75:15` | Magic number: 25, in <case> detected |
| L1948 | `internal/mobilebackup/row_page.go:75:19` | Magic number: 26, in <case> detected |
| L1949 | `internal/mobilebackup/row_page.go:75:23` | Magic number: 29, in <case> detected |
| L1950 | `internal/mobilebackup/row_page.go:75:27` | Magic number: 32, in <case> detected |
| L1951 | `internal/mobilebackup/row_page.go:75:31` | Magic number: 33, in <case> detected |
| L1952 | `internal/mobilebackup/row_page.go:75:35` | Magic number: 34, in <case> detected |
| L1953 | `internal/mobilebackup/row_page.go:75:39` | Magic number: 35, in <case> detected |
| L1954 | `internal/mobilebackup/row_page.go:75:43` | Magic number: 36, in <case> detected |
| L1955 | `internal/mobilebackup/row_page.go:75:47` | Magic number: 45, in <case> detected |
| L1956 | `internal/mobilebackup/row_page.go:75:51` | Magic number: 51, in <case> detected |
| L1957 | `internal/mobilebackup/row_page.go:75:55` | Magic number: 52, in <case> detected |
| L1958 | `internal/mobilebackup/snapshot.go:172:85` | Magic number: 0600, in <argument> detected |
| L1959 | `internal/mobilebackup/snapshot.go:201:29` | Magic number: 64, in <argument> detected |
| L1960 | `internal/mobilebackup/snapshot.go:206:23` | Magic number: 32, in <argument> detected |
| L1961 | `internal/mobilebackup/snapshot.go:211:77` | Magic number: 0600, in <argument> detected |
| L1962 | `internal/mobilebackup/snapshot.go:242:43` | Magic number: 33, in <argument> detected |
| L1963 | `internal/mobilebackup/snapshot.go:364:68` | Magic number: 0600, in <argument> detected |
| L1964 | `internal/mobilebackup/snapshot.go:457:60` | Magic number: 64, in <operation> detected |
| L1965 | `internal/mobilebackup/snapshot.go:459:79` | Magic number: 4, in <argument> detected |
| L1966 | `internal/mobilebackup/snapshot.go:511:43` | Magic number: 4, in <argument> detected |
| L1967 | `internal/mobilebackup/snapshot_page.go:135:18` | Magic number: 20, in <condition> detected |
| L1968 | `internal/mobilebackup/snapshot_recalls.go:74:43` | Magic number: 253402300800000, in <argument> detected |
| L1969 | `internal/mobilebackup/snapshot_terminal.go:30:45` | Magic number: 4, in <argument> detected |
| L1970 | `internal/mobilebackup/snapshot_terminal.go:65:110` | Magic number: 4, in <argument> detected |
| L1971 | `internal/mobilebackup/sqlite.go:109:42` | Magic number: 20, in <argument> detected |
| L1972 | `internal/mobilebackup/sqlite.go:203:24` | Magic number: 20, in <condition> detected |
| L1973 | `internal/mobilebackup/sqlite.go:226:45` | Magic number: 30, in <argument> detected |
| L1974 | `internal/mobilebackup/sqlite.go:377:29` | Magic number: 5000, in <condition> detected |
| L1975 | `internal/mobilebackup/xz.go:38:44` | Magic number: 4096, in <assign> detected |
| L1976 | `internal/mobilebackup/xz.go:82:8` | Magic number: 4, in <case> detected |
| L1977 | `internal/mobilebackup/xz.go:84:8` | Magic number: 10, in <case> detected |
| L1978 | `internal/mobilebackup/xz.go:89:71` | Magic number: 4, in <operation> detected |
| L1979 | `internal/mobilebackup/xz.go:90:44` | Magic number: 24, in <argument> detected |
| L1980 | `internal/mobilebackup/xz.go:93:23` | Magic number: 12, in <operation> detected |
| L1981 | `internal/mobilebackup/xz.go:116:23` | Magic number: 3, in <operation> detected |
| L1982 | `internal/mobilebackup/xz.go:116:36` | Magic number: 3, in <argument> detected |
| L1983 | `internal/mobilebackup/xz.go:125:43` | Magic number: 12, in <operation> detected |
| L1984 | `internal/mobilebackup/xz.go:131:18` | Magic number: 12, in <operation> detected |
| L1985 | `internal/mobilebackup/xz.go:136:35` | Magic number: 3, in <operation> detected |
| L1986 | `internal/mobilebackup/xz.go:136:48` | Magic number: 3, in <argument> detected |
| L1987 | `internal/mobilebackup/xz.go:157:37` | Magic number: 4, in <operation> detected |
| L1988 | `internal/mobilebackup/xz.go:188:16` | Magic number: 40, in <condition> detected |
| L1989 | `internal/mobilebackup/xz.go:192:17` | Magic number: 40, in <condition> detected |
| L1990 | `internal/mobilebackup/xz.go:195:56` | Magic number: 11, in <operation> detected |
| L1991 | `internal/mobilebackup/xz.go:205:21` | Magic number: 0x7f, in <argument> detected |
| L1992 | `internal/mobilebackup/xz.go:205:31` | Magic number: 7, in <operation> detected |
| L1993 | `internal/netpolicy/http.go:70:33` | Magic number: 5, in <assign> detected |
| L1994 | `internal/netpolicy/http.go:75:24` | Magic number: 5, in <assign> detected |
| L1995 | `internal/netpolicy/http.go:75:64` | Magic number: 5, in <assign> detected |
| L1996 | `internal/service/account_archive.go:39:45` | Magic number: 420, in <argument> detected |
| L1997 | `internal/service/account_archive.go:170:42` | Magic number: 120, in <argument> detected |
| L1998 | `internal/service/archive_catalog.go:35:10` | Magic number: 20, in <return> detected |
| L1999 | `internal/service/archive_catalog.go:76:36` | Magic number: 7, in <operation> detected |
| L2000 | `internal/service/archive_catalog.go:76:39` | Magic number: 8, in <argument> detected |
| L2001 | `internal/service/archive_catalog.go:108:48` | Magic number: 8, in <argument> detected |
| L2002 | `internal/service/archive_catalog.go:115:45` | Magic number: 8, in <argument> detected |
| L2003 | `internal/service/archive_messages.go:321:28` | Magic number: 2048, in <operation> detected |
| L2004 | `internal/service/bridge.go:50:18` | Magic number: 40, in <assign> detected |
| L2005 | `internal/service/bridge.go:189:24` | Magic number: 256, in <condition> detected |
| L2006 | `internal/service/bridge.go:208:24` | Magic number: 4096, in <condition> detected |
| L2007 | `internal/service/bridge.go:214:72` | Magic number: 24, in <argument> detected |
| L2008 | `internal/service/cli.go:93:48` | Magic number: 35, in <argument> detected |
| L2009 | `internal/service/cli.go:109:41` | Magic number: 5, in <argument> detected |
| L2010 | `internal/service/cli.go:129:57` | Magic number: 5, in <argument> detected |
| L2011 | `internal/service/cli.go:146:48` | Magic number: 3, in <argument> detected |
| L2012 | `internal/service/cli_mobile_ledger.go:47:47` | Magic number: 16, in <operation> detected |
| L2013 | `internal/service/cli_mobile_ledger.go:47:51` | Magic number: 10, in <operation> detected |
| L2014 | `internal/service/cli_mobile_ledger.go:71:19` | Magic number: 3, in <condition> detected |
| L2015 | `internal/service/cli_mobile_ledger.go:104:19` | Magic number: 4, in <condition> detected |
| L2016 | `internal/service/cli_mobile_ledger.go:116:12` | Magic number: 10, in <operation> detected |
| L2017 | `internal/service/cli_mobile_ledger.go:119:12` | Magic number: 140, in <operation> detected |
| L2018 | `internal/service/cli_mobile_ledger.go:122:12` | Magic number: 40, in <operation> detected |
| L2019 | `internal/service/cli_mobile_ledger.go:125:12` | Magic number: 205, in <operation> detected |
| L2020 | `internal/service/cli_mobile_ledger.go:128:12` | Magic number: 445, in <operation> detected |
| L2021 | `internal/service/mobile_archive_probe.go:67:46` | Magic number: 420, in <argument> detected |
| L2022 | `internal/service/mobile_archive_probe.go:99:33` | Magic number: 500, in <argument> detected |
| L2023 | `internal/service/mobile_archive_probe.go:114:5` | Magic number: 50, in <argument> detected |
| L2024 | `internal/service/mobile_archive_probe.go:115:5` | Magic number: 100, in <argument> detected |
| L2025 | `internal/service/mobile_ledger_control.go:30:14` | Magic number: 4, in <condition> detected |
| L2026 | `internal/service/mobile_offer_probe.go:23:44` | Magic number: 185, in <argument> detected |
| L2027 | `internal/service/recall_diagnostic.go:58:50` | Magic number: 30, in <argument> detected |
| L2028 | `internal/service/recall_diagnostic.go:99:44` | Magic number: 0700, in <argument> detected |
| L2029 | `internal/service/recall_diagnostic.go:122:43` | Magic number: 513, in <argument> detected |
| L2030 | `internal/service/recall_diagnostic.go:151:28` | Magic number: 21, in <argument> detected |
| L2031 | `internal/service/recall_diagnostic.go:156:63` | Magic number: 0600, in <argument> detected |
| L2032 | `internal/service/run.go:113:55` | Magic number: 16, in <argument> detected |
| L2033 | `internal/service/run.go:113:59` | Magic number: 10, in <argument> detected |
| L2034 | `internal/service/run.go:251:97` | Magic number: 512, in <argument> detected |
| L2035 | `internal/service/run.go:251:102` | Magic number: 20, in <argument> detected |
| L2036 | `internal/service/run.go:256:106` | Magic number: 30, in <argument> detected |
| L2037 | `internal/service/run.go:265:6` | Magic number: 30, in <argument> detected |
| L2038 | `internal/service/run.go:336:22` | Magic number: 5, in <assign> detected |
| L2039 | `internal/service/run.go:337:22` | Magic number: 15, in <assign> detected |
| L2040 | `internal/service/run.go:338:22` | Magic number: 45, in <assign> detected |
| L2041 | `internal/service/run.go:339:22` | Magic number: 30, in <assign> detected |
| L2042 | `internal/service/run.go:345:22` | Magic number: 5, in <assign> detected |
| L2043 | `internal/service/run.go:346:22` | Magic number: 10, in <assign> detected |
| L2044 | `internal/service/run.go:347:22` | Magic number: 450, in <assign> detected |
| L2045 | `internal/service/run.go:349:31` | Magic number: 8, in <argument> detected |
| L2046 | `internal/service/run.go:390:28` | Magic number: 5, in <argument> detected |
| L2047 | `internal/service/run.go:408:62` | Magic number: 5, in <argument> detected |
| L2048 | `internal/service/run.go:417:75` | Magic number: 500, in <argument> detected |
| L2049 | `internal/service/run.go:442:66` | Magic number: 10, in <argument> detected |
| L2050 | `internal/service/run.go:511:48` | Magic number: 30, in <argument> detected |
| L2051 | `internal/service/run.go:636:24` | Magic number: 40, in <argument> detected |
| L2052 | `internal/service/telemetry.go:19:27` | Magic number: 500, in <argument> detected |
| L2053 | `internal/service/token.go:16:26` | Magic number: 32, in <argument> detected |
| L2054 | `internal/storage/browse.go:111:29` | Magic number: 150, in <operation> detected |
| L2055 | `internal/storage/contacts.go:46:21` | Magic number: 200, in <condition> detected |
| L2056 | `internal/storage/deliveries.go:23:16` | Magic number: 100000, in <assign> detected |
| L2057 | `internal/storage/deliveries.go:24:16` | Magic number: 256, in <assign> detected |
| L2058 | `internal/storage/deliveries.go:24:23` | Magic number: 20, in <assign> detected |
| L2059 | `internal/storage/deliveries.go:25:16` | Magic number: 256, in <assign> detected |
| L2060 | `internal/storage/deliveries.go:27:16` | Magic number: 30, in <assign> detected |
| L2061 | `internal/storage/events.go:220:15` | Magic number: 100, in <condition> detected |
| L2062 | `internal/storage/history_expiry.go:61:20` | Magic number: 50, in <condition> detected |
| L2063 | `internal/storage/history_operations.go:349:20` | Magic number: 50, in <condition> detected |
| L2064 | `internal/storage/history_operations.go:721:11` | Magic number: 30, in <operation> detected |
| L2065 | `internal/storage/history_operations.go:723:11` | Magic number: 180, in <operation> detected |
| L2066 | `internal/storage/mobile_backup.go:141:14` | Magic number: 10000, in <condition> detected |
| L2067 | `internal/storage/preload_directory.go:14:20` | Magic number: 5000, in <condition> detected |
| L2068 | `internal/storage/store.go:80:21` | Magic number: 32, in <argument> detected |
| L2069 | `internal/storage/store.go:690:30` | Magic number: 150, in <operation> detected |
| L2070 | `internal/storage/views.go:266:64` | Magic number: 8000, in <argument> detected |
| L2071 | `internal/storage/views.go:380:50` | Magic number: 1000, in <condition> detected |
| L2072 | `internal/storage/views.go:381:46` | Magic number: 1000, in <argument> detected |
| L2073 | `internal/storage/views.go:385:27` | Magic number: 1000, in <argument> detected |
| L2074 | `internal/toolruntime/binder.go:48:35` | Magic number: 20, in <argument> detected |
| L2075 | `internal/toolruntime/binder.go:68:40` | Magic number: 5, in <argument> detected |
| L2076 | `internal/toolruntime/binder.go:69:39` | Magic number: 5, in <argument> detected |
| L2077 | `internal/toolruntime/runtime.go:543:26` | Magic number: 20, in <argument> detected |
| L2078 | `internal/toolruntime/runtime.go:574:38` | Magic number: 20, in <argument> detected |
| L2079 | `internal/toolruntime/runtime.go:584:36` | Magic number: 20, in <argument> detected |
| L2080 | `internal/toolruntime/runtime.go:603:46` | Magic number: 20, in <argument> detected |
| L2081 | `internal/toolruntime/runtime.go:616:42` | Magic number: 20, in <argument> detected |
| L2082 | `internal/toolruntime/runtime.go:634:35` | Magic number: 20, in <argument> detected |
| L2083 | `internal/zalo/adapter.go:46:46` | Magic number: 30, in <assign> detected |
| L2084 | `internal/zalo/adapter.go:183:20` | Magic number: 50, in <argument> detected |
| L2085 | `internal/zalo/events.go:60:32` | Magic number: 15, in <argument> detected |
| L2086 | `internal/zalo/mobile_backup.go:52:73` | Magic number: 2048, in <argument> detected |
| L2087 | `internal/zalo/mobile_backup.go:71:43` | Magic number: 180, in <argument> detected |
| L2088 | `internal/zalo/mobile_backup.go:164:13` | Magic number: 3, in <case> detected |
| L2089 | `internal/zalo/replay.go:51:18` | Magic number: 100, in <case> detected |
| L2090 | `internal/zalo/replay.go:53:21` | Magic number: 10000, in <case> detected |
| L2091 | `tools/harness/internal/architecture/architecture.go:77:16` | Magic number: 20, in <operation> detected |
| L2092 | `tools/harness/internal/architecture/architecture.go:90:42` | Magic number: 60, in <argument> detected |
| L2093 | `tools/harness/internal/performance/bench.go:28:42` | Magic number: 900, in <argument> detected |
| L2094 | `tools/harness/internal/performance/bench.go:30:67` | Magic number: 4, in <argument> detected |
| L2095 | `tools/harness/internal/performance/bench.go:30:70` | Magic number: 20, in <argument> detected |
| L2096 | `tools/harness/internal/performance/bench.go:46:68` | Magic number: 1000, in <argument> detected |
| L2097 | `tools/harness/internal/performance/bench.go:46:107` | Magic number: 50, in <argument> detected |
| L2098 | `tools/harness/internal/performance/bench.go:46:262` | Magic number: 110, in <argument> detected |
| L2099 | `tools/harness/internal/performance/bench.go:76:42` | Magic number: 1200, in <argument> detected |
| L2100 | `tools/harness/internal/performance/bench.go:147:302` | Magic number: 5, in <assign> detected |
| L2101 | `tools/harness/internal/performance/bench.go:147:331` | Magic number: 1000, in <assign> detected |
| L2102 | `tools/harness/internal/performance/bench.go:147:353` | Magic number: 50, in <assign> detected |
| L2103 | `tools/harness/internal/performance/bench.go:147:402` | Magic number: 3, in <assign> detected |
| L2104 | `tools/harness/internal/performance/bench.go:147:477` | Magic number: 110, in <assign> detected |
| L2105 | `tools/harness/internal/performance/bench.go:147:504` | Magic number: 1200, in <assign> detected |
| L2106 | `tools/harness/internal/performance/bench.go:147:534` | Magic number: 900, in <assign> detected |
| L2107 | `tools/harness/internal/performance/bench.go:147:562` | Magic number: 1.15, in <assign> detected |
| L2108 | `tools/harness/internal/performance/bench.go:147:599` | Magic number: 2.0, in <assign> detected |
| L2109 | `tools/harness/internal/performance/bench.go:147:635` | Magic number: 1.20, in <assign> detected |
| L2110 | `tools/harness/internal/performance/bench.go:151:27` | Magic number: 3, in <argument> detected |
| L2111 | `tools/harness/internal/performance/build.go:15:33` | Magic number: 30, in <argument> detected |
| L2112 | `tools/harness/internal/performance/build.go:23:55` | Magic number: 4, in <argument> detected |
| L2113 | `tools/harness/internal/performance/build.go:23:58` | Magic number: 20, in <argument> detected |
| L2114 | `tools/harness/internal/performance/build.go:85:43` | Magic number: 180, in <argument> detected |
| L2115 | `tools/harness/internal/performance/build.go:108:57` | Magic number: 4, in <argument> detected |
| L2116 | `tools/harness/internal/performance/build.go:108:60` | Magic number: 20, in <argument> detected |
| L2117 | `tools/harness/internal/performance/compare.go:30:29` | Magic number: 5, in <assign> detected |
| L2118 | `tools/harness/internal/performance/compare.go:30:48` | Magic number: 50, in <assign> detected |
| L2119 | `tools/harness/internal/performance/compare.go:30:97` | Magic number: 3, in <assign> detected |
| L2120 | `tools/harness/internal/performance/compare.go:30:172` | Magic number: 110, in <assign> detected |
| L2121 | `tools/harness/internal/performance/compare.go:41:21` | Magic number: 15, in <condition> detected |
| L2122 | `tools/harness/internal/performance/compare.go:145:167` | Magic number: 2.0, in <assign> detected |
| L2123 | `tools/harness/internal/performance/compare.go:145:187` | Magic number: 1.20, in <assign> detected |
| L2124 | `tools/harness/internal/performance/compare.go:171:45` | Magic number: 1.15, in <assign> detected |
| L2125 | `tools/harness/internal/performance/compare.go:171:82` | Magic number: 2.0, in <assign> detected |
| L2126 | `tools/harness/internal/performance/compare.go:252:51` | Magic number: 1.20, in <operation> detected |
| L2127 | `tools/harness/internal/performance/compare.go:253:232` | Magic number: 1.20, in <assign> detected |
| L2128 | `tools/harness/internal/performance/json.go:24:44` | Magic number: .0078683, in <assign> detected |
| L2129 | `tools/harness/internal/performance/json.go:24:64` | Magic number: .0070726337, in <assign> detected |
| L2130 | `tools/harness/internal/performance/json.go:24:88` | Magic number: .0069967799, in <assign> detected |
| L2131 | `tools/harness/internal/performance/json.go:94:15` | Magic number: 64, in <condition> detected |
| L2132 | `tools/harness/internal/performance/process.go:54:23` | Magic number: 8192, in <argument> detected |

### modernize (5)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2133–L2137: применить адресно после сверки go version tools module, проверить errors.AsType, CutPrefix, SplitSeq, maps.Copy и slices.Contains на текущих типах. Проверить nil map/empty input и порядок итерации. Новые stdlib imports согласовать с точными architecture allowlists. Не запускать очередной бесконтрольный --fix на весь root.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2133 | `tools/harness/internal/architecture/architecture.go:122:6` | errorsastype: errors.As can be simplified using AsType[*exec.ExitError] |
| L2134 | `tools/harness/internal/architecture/architecture.go:308:5` | stringscutprefix: HasPrefix + TrimPrefix can be simplified to CutPrefix |
| L2135 | `tools/harness/internal/performance/bench.go:127:26` | stringsseq: Ranging over SplitSeq is more efficient |
| L2136 | `tools/harness/internal/performance/bench.go:148:4` | mapsloop: Replace m[k]=v loop with maps.Copy |
| L2137 | `tools/harness/internal/performance/compare.go:137:2` | slicescontains: Loop can be simplified using slices.Contains |

### musttag (33)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Добавить явные json tags, в точности повторяющие существующие имена/omitempty и порядок полей. Критично L2139 SendRequest.Fingerprint: json:"Recipient", json:"Text", json:"Reply" (без omitempty); L2169 ReadIdentity: json:"All", json:"Selected". Lower snake_case здесь изменит hash/persistence semantics. Для внешних SDK structs/custom MarshalJSON, которые нельзя корректно разметить в этом repo, локальный nolint:musttag с указанием владельца codec. Проверить old/new exact bytes на nil/empty/Unicode.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2138 | `docs/contracts/sending_test.go:68:32` | the given struct should be annotated with the &#96;json&#96; tag |
| L2139 | `internal/domain/sending.go:41:23` | the given struct should be annotated with the &#96;json&#96; tag |
| L2140 | `internal/evalfixture/offline.go:226:31` | the given struct should be annotated with the &#96;json&#96; tag |
| L2141 | `internal/mcpserver/sdk_identity_test.go:144:36` | the given struct should be annotated with the &#96;json&#96; tag |
| L2142 | `internal/mcpserver/sdk_identity_test.go:182:53` | the given struct should be annotated with the &#96;json&#96; tag |
| L2143 | `internal/mcpserver/sdk_identity_test.go:197:7` | the given struct should be annotated with the &#96;json&#96; tag |
| L2144 | `internal/mcpserver/sdk_identity_test.go:274:5` | the given struct should be annotated with the &#96;json&#96; tag |
| L2145 | `internal/mcpserver/sdk_lifecycle_test.go:142:31` | the given struct should be annotated with the &#96;json&#96; tag |
| L2146 | `internal/mcpserver/sdk_lifecycle_test.go:171:40` | the given struct should be annotated with the &#96;json&#96; tag |
| L2147 | `internal/mcpserver/sdk_lifecycle_test.go:348:52` | the given struct should be annotated with the &#96;json&#96; tag |
| L2148 | `internal/mcpserver/sdk_stdio_identity_test.go:148:34` | the given struct should be annotated with the &#96;json&#96; tag |
| L2149 | `internal/mcpserver/sdk_stdio_identity_test.go:184:34` | the given struct should be annotated with the &#96;json&#96; tag |
| L2150 | `internal/mcpserver/sdk_stdio_identity_test.go:213:4` | the given struct should be annotated with the &#96;json&#96; tag |
| L2151 | `internal/mcpserver/sdk_stdio_identity_test.go:229:4` | the given struct should be annotated with the &#96;json&#96; tag |
| L2152 | `internal/mcpserver/sdk_stdio_identity_test.go:258:3` | the given struct should be annotated with the &#96;json&#96; tag |
| L2153 | `internal/mobilebackup/encrypted_archive_test.go:29:30` | the given struct should be annotated with the &#96;json&#96; tag |
| L2154 | `internal/mobilebackup/encrypted_archive_test.go:118:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2155 | `internal/mobilebackup/header_test.go:38:31` | the given struct should be annotated with the &#96;json&#96; tag |
| L2156 | `internal/mobilebackup/message_conversion_test.go:302:40` | the given struct should be annotated with the &#96;json&#96; tag |
| L2157 | `internal/mobilebackup/xz_test.go:24:30` | the given struct should be annotated with the &#96;json&#96; tag |
| L2158 | `internal/mobilebackup/xz_test.go:209:30` | the given struct should be annotated with the &#96;json&#96; tag |
| L2159 | `internal/observability/native_test.go:123:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2160 | `internal/observability/native_test.go:223:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2161 | `internal/observability/native_test.go:307:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2162 | `internal/observability/native_test.go:399:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2163 | `internal/observability/observer_test.go:169:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2164 | `internal/observability/observer_test.go:242:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2165 | `internal/service/account_archive_test.go:80:29` | the given struct should be annotated with the &#96;json&#96; tag |
| L2166 | `internal/service/mobile_archive_probe_test.go:35:31` | the given struct should be annotated with the &#96;json&#96; tag |
| L2167 | `internal/service/mobile_history_test.go:54:31` | the given struct should be annotated with the &#96;json&#96; tag |
| L2168 | `internal/service/mobile_page_test.go:113:31` | the given struct should be annotated with the &#96;json&#96; tag |
| L2169 | `internal/storage/read_authority.go:34:27` | the given struct should be annotated with the &#96;json&#96; tag |
| L2170 | `internal/toolruntime/command_scope_test.go:43:27` | the given struct should be annotated with the &#96;json&#96; tag |

### nestif (39)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Guard clauses и самостоятельные этапы вместо глубоких ветвей; в transaction/worker code сохранить те же commit/rollback/cancel paths. В tests допустим локальный nolint:nestif для намеренного adversarial сценария, если table-driven вариант скрывает последовательность.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2171 | `cmd/zl-mcp-eval/main.go:58:1` | &#96;if *checkpoint != ""&#96; has complex nested blocks (complexity: 5) |
| L2172 | `cmd/zl-mcp-eval/main.go:98:1` | &#96;if *child != ""&#96; has complex nested blocks (complexity: 12) |
| L2173 | `internal/app/search/read.go:220:1` | &#96;if len(p.Hits) == 0&#96; has complex nested blocks (complexity: 10) |
| L2174 | `internal/collector/join.go:435:1` | &#96;if member&#96; has complex nested blocks (complexity: 6) |
| L2175 | `internal/collector/run.go:65:1` | &#96;if ok&#96; has complex nested blocks (complexity: 8) |
| L2176 | `internal/collector/run.go:326:1` | &#96;if v.Method == "cli_preview" \|\| v.Method == "cli_approve"&#96; has complex nested blocks (complexity: 7) |
| L2177 | `internal/domain/mobile_backup_request.go:27:1` | &#96;if r.ArchiveScope != ""&#96; has complex nested blocks (complexity: 5) |
| L2178 | `internal/evalfixture/fixture.go:217:1` | &#96;if opts.ApprovalFile != "" \|\| opts.ApprovalTokenFile != ""&#96; has complex nested blocks (complexity: 18) |
| L2179 | `internal/evaluation/checkpoints.go:184:1` | &#96;if t.Kind == "resource"&#96; has complex nested blocks (complexity: 5) |
| L2180 | `internal/evaluation/checkpoints.go:424:1` | &#96;if ok&#96; has complex nested blocks (complexity: 10) |
| L2181 | `internal/evaluation/runner.go:228:1` | &#96;if e == nil&#96; has complex nested blocks (complexity: 6) |
| L2182 | `internal/events/subscriptions.go:169:1` | &#96;if method == "events/list"&#96; has complex nested blocks (complexity: 35) |
| L2183 | `internal/events/worker.go:121:1` | &#96;if requestErr == nil&#96; has complex nested blocks (complexity: 5) |
| L2184 | `internal/historyimport/mobile_worker.go:176:1` | &#96;if errors.Is(e, sql.ErrNoRows)&#96; has complex nested blocks (complexity: 10) |
| L2185 | `internal/historyimport/mobile_worker.go:291:1` | &#96;if e != nil&#96; has complex nested blocks (complexity: 5) |
| L2186 | `internal/historyimport/worker.go:126:1` | &#96;if op.Status.Source == "conversation_preload"&#96; has complex nested blocks (complexity: 6) |
| L2187 | `internal/mcpserver/http.go:171:1` | &#96;if strings.HasPrefix(request.Method, "events/")&#96; has complex nested blocks (complexity: 12) |
| L2188 | `internal/messaging/manager.go:47:1` | &#96;if method == "zalo_get_send_status"&#96; has complex nested blocks (complexity: 5) |
| L2189 | `internal/mobilebackup/account_inspection.go:82:1` | &#96;if err != nil&#96; has complex nested blocks (complexity: 10) |
| L2190 | `internal/mobilebackup/snapshot.go:195:1` | &#96;if errors.Is(err, os.ErrNotExist)&#96; has complex nested blocks (complexity: 7) |
| L2191 | `internal/mobilebackup/snapshot_page.go:135:1` | &#96;if row.Type == 20&#96; has complex nested blocks (complexity: 8) |
| L2192 | `internal/mobilebackup/sqlite.go:371:1` | &#96;if snapshot&#96; has complex nested blocks (complexity: 9) |
| L2193 | `internal/service/archive_catalog.go:79:1` | &#96;if token != ""&#96; has complex nested blocks (complexity: 6) |
| L2194 | `internal/service/archive_messages_test.go:71:1` | &#96;if !testedResource&#96; has complex nested blocks (complexity: 8) |
| L2195 | `internal/service/mobile_ledger_control.go:134:1` | &#96;if method == "cli_prepare_mobile_backup"&#96; has complex nested blocks (complexity: 11) |
| L2196 | `internal/service/recall_diagnostic.go:113:1` | &#96;if e == nil&#96; has complex nested blocks (complexity: 5) |
| L2197 | `internal/service/run.go:517:1` | &#96;if err == nil&#96; has complex nested blocks (complexity: 16) |
| L2198 | `internal/service/token.go:15:1` | &#96;if errors.Is(err, os.ErrNotExist)&#96; has complex nested blocks (complexity: 7) |
| L2199 | `internal/storage/events.go:212:1` | &#96;if reuse&#96; has complex nested blocks (complexity: 5) |
| L2200 | `internal/storage/history_operations.go:384:1` | &#96;if !s.AllowsConversation(op.Status.Ref())&#96; has complex nested blocks (complexity: 10) |
| L2201 | `internal/storage/mobile_history_acquisition_test.go:112:1` | &#96;if mode == "unreserved" \|\| mode == "stale-prepare" \|\| mode == "cancel-before-bind"&#96; has complex nested blocks (complexity: 5) |
| L2202 | `internal/storage/mobile_history_acquisition_test.go:590:1` | &#96;if rollback&#96; has complex nested blocks (complexity: 6) |
| L2203 | `internal/storage/views.go:434:1` | &#96;if e == nil&#96; has complex nested blocks (complexity: 5) |
| L2204 | `internal/toolruntime/runtime.go:173:1` | &#96;if len(extra) == 1&#96; has complex nested blocks (complexity: 12) |
| L2205 | `internal/zalo/preload.go:127:1` | &#96;if batch.kind == domain.ConversationDirect&#96; has complex nested blocks (complexity: 6) |
| L2206 | `internal/zalo/sending.go:29:1` | &#96;if err != nil&#96; has complex nested blocks (complexity: 6) |
| L2207 | `tools/harness/internal/architecture/architecture.go:172:1` | &#96;if d.IsDir()&#96; has complex nested blocks (complexity: 5) |
| L2208 | `tools/harness/internal/performance/bench.go:38:1` | &#96;if e == nil&#96; has complex nested blocks (complexity: 7) |
| L2209 | `tools/harness/internal/performance/bench.go:119:1` | &#96;if cpuModel == ""&#96; has complex nested blocks (complexity: 6) |

### nilerr (13)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Не превращать штатное отсутствие работы/отказ в авторизации в ошибку. L2210–L2218: claim race/ErrHistoryState и cancellation означают завершение worker без retry; локальный nolint:nilerr с конкретной причиной только на этих return. L2219/L2220: shutdown collectSession по ctx.Err() — успешная остановка, оставить. L2221/L2222: malformed ID/digest означает false (cleanup не разрешён), оставить fail-closed predicate с nolint. Не подавлять реальные storage errors в соседних ветвях.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2210 | `internal/historyimport/mobile_worker.go:109:3` | error is not nil (line 108) but it returns nil |
| L2211 | `internal/historyimport/mobile_worker.go:132:4` | error is not nil (line 131) but it returns nil |
| L2212 | `internal/historyimport/mobile_worker.go:316:3` | error is not nil (line 315) but it returns nil |
| L2213 | `internal/historyimport/worker.go:76:3` | error is not nil (line 75) but it returns nil |
| L2214 | `internal/historyimport/worker.go:96:4` | error is not nil (line 95) but it returns nil |
| L2215 | `internal/historyimport/worker.go:102:4` | error is not nil (line 101) but it returns nil |
| L2216 | `internal/historyimport/worker.go:115:4` | error is not nil (line 114) but it returns nil |
| L2217 | `internal/historyimport/worker.go:164:4` | error is not nil (line 163) but it returns nil |
| L2218 | `internal/historyimport/worker.go:201:4` | error is not nil (line 200) but it returns nil |
| L2219 | `internal/service/run.go:515:4` | error is not nil (line 514) but it returns nil |
| L2220 | `internal/service/run.go:556:5` | error is not nil (line 555) but it returns nil |
| L2221 | `internal/storage/mobile_snapshot_cleanup.go:19:3` | error is not nil (line 18) but it returns nil |
| L2222 | `internal/storage/mobile_snapshot_cleanup.go:24:4` | error is not nil (line 22) but it returns nil |

### nilnil (4)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2223: nil provider означает disabled instrumentation; L2224: пустая delivery queue; L2225: missing/null optional value; L2226: fake моделирует пустой результат. Во всех четырёх оставить (nil,nil) с локальным nolint:nilnil, описывающим эту семантику. Новый sentinel изменил бы контракт.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2223 | `internal/observability/observer.go:65:3` | return both a &#96;nil&#96; error and an invalid value: use a sentinel error instead |
| L2224 | `internal/storage/deliveries.go:286:4` | return both a &#96;nil&#96; error and an invalid value: use a sentinel error instead |
| L2225 | `internal/zalo/history.go:146:3` | return both a &#96;nil&#96; error and an invalid value: use a sentinel error instead |
| L2226 | `internal/zalo/mobile_backup_test.go:220:50` | return both a &#96;nil&#96; error and an invalid value: use a sentinel error instead |

### noctx (6)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2229/L2232: оставить exec.Command с nolint:noctx — cancellation уже реализована владельцем process group с Kill/Wait/drain и output budget. Автозамена на CommandContext не гарантирует cleanup descendants. L2227/L2228/L2230/L2231: net.ListenConfig.Listen с существующим lifecycle ctx, сохранив явный Close при shutdown (ctx Listen не закрывает готовый listener). Проверить startup cancellation.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2227 | `internal/evalfixture/fixture.go:279:29` | net.Listen must not be called. use (*net.ListenConfig).Listen |
| L2228 | `internal/evaluation/model.go:97:29` | net.Listen must not be called. use (*net.ListenConfig).Listen |
| L2229 | `internal/evaluation/process.go:50:21` | os/exec.Command must not be called. use os/exec.CommandContext |
| L2230 | `internal/service/run.go:310:29` | net.Listen must not be called. use (*net.ListenConfig).Listen |
| L2231 | `internal/service/run.go:322:24` | net.Listen must not be called. use (*net.ListenConfig).Listen |
| L2232 | `tools/harness/internal/performance/process.go:33:21` | os/exec.Command must not be called. use os/exec.CommandContext |

### nonamedreturns (51)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Сохранить named results там, где defer читает err, очищает result, фиксирует telemetry или собирает финальный report: локальный nolint:nonamedreturns с конкретной обязанностью defer. У простых функций без такого использования убрать имена. В parser Clear-on-error не заменять result копией: это меняет, какой buffer очищается.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2233 | `internal/evaluation/benchmark.go:39:73` | named return "result" with type "Measurement" found |
| L2234 | `internal/evaluation/model.go:71:84` | named return "out" with type "Output" found |
| L2235 | `internal/evaluation/runner.go:135:46` | named return "report" with type "Report" found |
| L2236 | `internal/evaluation/runner.go:135:61` | named return "err" with type "error" found |
| L2237 | `internal/evaluation/scenarios.go:22:55` | named return "out" with type "Output" found |
| L2238 | `internal/mcpserver/server.go:239:80` | named return "response" with type "*mcp.CallToolResult" found |
| L2239 | `internal/mobilebackup/attachment.go:44:57` | named return "result" with type "Attachment" found |
| L2240 | `internal/mobilebackup/binnet.go:33:53` | named return "result" with type "BinNet" found |
| L2241 | `internal/mobilebackup/control_scan.go:37:4` | named return "result" with type "OwnDirectRecallSet" found |
| L2242 | `internal/mobilebackup/expiry.go:13:55` | named return "expiresMS" with type "int64" found |
| L2243 | `internal/mobilebackup/expiry.go:13:72` | named return "declared" with type "bool" found |
| L2244 | `internal/mobilebackup/expiry.go:13:87` | named return "err" with type "error" found |
| L2245 | `internal/mobilebackup/history_page.go:25:4` | named return "result" with type "domain.MobileHistoryPage" found |
| L2246 | `internal/mobilebackup/history_page.go:25:37` | named return "err" with type "error" found |
| L2247 | `internal/mobilebackup/mentions.go:26:54` | named return "result" with type "Mention" found |
| L2248 | `internal/mobilebackup/message_conversion.go:42:4` | named return "result" with type "ConvertedArchivePage" found |
| L2249 | `internal/mobilebackup/message_conversion.go:42:33` | named return "err" with type "error" found |
| L2250 | `internal/mobilebackup/message_conversion.go:97:4` | named return "result" with type "ConvertedArchivePage" found |
| L2251 | `internal/mobilebackup/message_conversion.go:251:36` | named return "text" with type "string" found |
| L2252 | `internal/mobilebackup/message_conversion.go:251:49` | named return "rich" with type "bool" found |
| L2253 | `internal/mobilebackup/message_conversion.go:251:55` | named return "supported" with type "bool" found |
| L2254 | `internal/mobilebackup/message_conversion.go:251:66` | named return "valid" with type "bool" found |
| L2255 | `internal/mobilebackup/page_expiry.go:6:53` | named return "expiredMessages" with type "int" found |
| L2256 | `internal/mobilebackup/page_expiry.go:6:70` | named return "expiredQuotes" with type "int" found |
| L2257 | `internal/mobilebackup/page_expiry.go:6:89` | named return "err" with type "error" found |
| L2258 | `internal/mobilebackup/page_walk.go:30:4` | named return "summary" with type "ArchiveWalkSummary" found |
| L2259 | `internal/mobilebackup/prepared_page.go:53:4` | named return "result" with type "PreparedArchivePage" found |
| L2260 | `internal/mobilebackup/quote.go:37:59` | named return "result" with type "QuoteScalars" found |
| L2261 | `internal/mobilebackup/quote_fields.go:37:52` | named return "result" with type "Quote" found |
| L2262 | `internal/mobilebackup/row_page.go:87:4` | named return "result" with type "PreparedRowPage" found |
| L2263 | `internal/mobilebackup/snapshot_page.go:86:4` | named return "result" with type "SnapshotPage" found |
| L2264 | `internal/mobilebackup/snapshot_recalls.go:39:4` | named return "targets" with type "*snapshotRecallTargets" found |
| L2265 | `internal/mobilebackup/sqlite.go:149:4` | named return "batch" with type "SQLiteBatch" found |
| L2266 | `internal/mobilebackup/sqlite.go:149:23` | named return "err" with type "error" found |
| L2267 | `internal/mobilebackup/sqlite.go:177:4` | named return "batch" with type "SQLiteBatch" found |
| L2268 | `internal/mobilebackup/time_window.go:10:49` | named return "from" with type "int64" found |
| L2269 | `internal/mobilebackup/time_window.go:10:55` | named return "to" with type "int64" found |
| L2270 | `internal/mobilebackup/time_window.go:10:65` | named return "ok" with type "bool" found |
| L2271 | `internal/mobilebackup/tlv.go:35:50` | named return "result" with type "TLV" found |
| L2272 | `internal/observability/observer.go:131:87` | named return "err" with type "error" found |
| L2273 | `internal/observability/observer.go:185:107` | named return "err" with type "error" found |
| L2274 | `internal/observability/observer.go:284:4` | named return "err" with type "error" found |
| L2275 | `internal/service/mobile_backup.go:146:4` | named return "result" with type "mobilebackup.PreparedArchivePage" found |
| L2276 | `internal/service/mobile_backup.go:146:45` | named return "err" with type "error" found |
| L2277 | `internal/service/mobile_backup.go:186:4` | named return "result" with type "mobilebackup.ArchiveWalkSummary" found |
| L2278 | `internal/service/mobile_backup.go:186:44` | named return "err" with type "error" found |
| L2279 | `internal/storage/contact_profiles.go:17:4` | named return "ids" with type "[]string" found |
| L2280 | `internal/storage/contact_profiles.go:17:18` | named return "cursor" with type "string" found |
| L2281 | `internal/storage/contact_profiles.go:17:33` | named return "more" with type "bool" found |
| L2282 | `internal/storage/contact_profiles.go:17:44` | named return "err" with type "error" found |
| L2283 | `tools/harness/internal/architecture/architecture.go:449:55` | named return "e" with type "Evidence" found |

### perfsprint (11)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2284–L2294: fmt.Errorf без formatting заменить errors.New, добавить errors в точный пакетный allowlist при необходимости. Текст ошибок оставить побайтно; никаких новых wrappers/sentinels.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2284 | `tools/harness/cmd/harness/main.go:71:12` | error-format: fmt.Errorf can be replaced with errors.New |
| L2285 | `tools/harness/cmd/harness/main.go:82:12` | error-format: fmt.Errorf can be replaced with errors.New |
| L2286 | `tools/harness/internal/performance/bench.go:40:8` | error-format: fmt.Errorf can be replaced with errors.New |
| L2287 | `tools/harness/internal/performance/bench.go:47:10` | error-format: fmt.Errorf can be replaced with errors.New |
| L2288 | `tools/harness/internal/performance/bench.go:117:11` | error-format: fmt.Errorf can be replaced with errors.New |
| L2289 | `tools/harness/internal/performance/bench.go:137:11` | error-format: fmt.Errorf can be replaced with errors.New |
| L2290 | `tools/harness/internal/performance/bench.go:172:12` | error-format: fmt.Errorf can be replaced with errors.New |
| L2291 | `tools/harness/internal/performance/build.go:96:11` | error-format: fmt.Errorf can be replaced with errors.New |
| L2292 | `tools/harness/internal/performance/build.go:133:11` | error-format: fmt.Errorf can be replaced with errors.New |
| L2293 | `tools/harness/internal/performance/json.go:36:15` | error-format: fmt.Errorf can be replaced with errors.New |
| L2294 | `tools/harness/internal/performance/json.go:40:15` | error-format: fmt.Errorf can be replaced with errors.New |

### predeclared (4)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2295–L2298: copy переименовать по содержимому (clonedRequest, copiedPage, cookieCopy); не менять значение/ownership.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2295 | `internal/mcpserver/http_test.go:32:2` | variable copy has same name as predeclared identifier |
| L2296 | `internal/mobilebackup/snapshot_page.go:117:3` | variable copy has same name as predeclared identifier |
| L2297 | `internal/service/events_restart_test.go:77:2` | variable copy has same name as predeclared identifier |
| L2298 | `internal/zalo/cookies.go:38:3` | variable copy has same name as predeclared identifier |

### reassign (4)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2299–L2302: тест CLI использует os.Args/os.Stdout. Предпочтительно вызывать run(args, writer), если такая точка уже есть, либо subprocess. Пока тест намеренно проверяет process-global CLI entrypoint — локальный nolint:reassign с причиной, обязательный restore через t.Cleanup/defer, запрет parallel для всего конфликтующего scope.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2299 | `internal/service/cli_recall_command_test.go:50:3` | reassigning variable Args in other package os |
| L2300 | `internal/service/cli_recall_command_test.go:51:3` | reassigning variable Stdout in other package os |
| L2301 | `internal/service/cli_recall_command_test.go:57:2` | reassigning variable Args in other package os |
| L2302 | `internal/service/cli_recall_command_test.go:58:2` | reassigning variable Stdout in other package os |

### recvcheck (21)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Большинство указанных типов намеренно имеют value String/GoString для redaction и pointer Clear для очистки. Не переводить redaction methods на pointer: value formatting может раскрыть поля. Для таких типов локальный nolint:recvcheck с причиной «value formatting must redact; Clear mutates owned buffers». Для остальных проверить method sets и унифицировать receivers только без изменения интерфейсов/копирования mutex.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2303 | `internal/domain/mobile_history.go:68:6` | the methods of "MobileHistoryPage" use pointer receiver and non-pointer receiver. |
| L2304 | `internal/mobilebackup/account_fetch.go:13:6` | the methods of "AccountArchive" use pointer receiver and non-pointer receiver. |
| L2305 | `internal/mobilebackup/archive.go:14:6` | the methods of "PlainArchive" use pointer receiver and non-pointer receiver. |
| L2306 | `internal/mobilebackup/attachment.go:19:6` | the methods of "Attachment" use pointer receiver and non-pointer receiver. |
| L2307 | `internal/mobilebackup/binnet.go:7:6` | the methods of "BinNet" use pointer receiver and non-pointer receiver. |
| L2308 | `internal/mobilebackup/control_scan.go:12:6` | the methods of "OwnDirectRecallSet" use pointer receiver and non-pointer receiver. |
| L2309 | `internal/mobilebackup/encrypted_archive.go:11:6` | the methods of "Format1Archive" use pointer receiver and non-pointer receiver. |
| L2310 | `internal/mobilebackup/mentions.go:8:6` | the methods of "Mention" use pointer receiver and non-pointer receiver. |
| L2311 | `internal/mobilebackup/message_conversion.go:14:6` | the methods of "ConvertedArchivePage" use pointer receiver and non-pointer receiver. |
| L2312 | `internal/mobilebackup/prepared_page.go:20:6` | the methods of "PreparedArchivePage" use pointer receiver and non-pointer receiver. |
| L2313 | `internal/mobilebackup/quote.go:10:6` | the methods of "QuoteScalars" use pointer receiver and non-pointer receiver. |
| L2314 | `internal/mobilebackup/quote_fields.go:18:6` | the methods of "Quote" use pointer receiver and non-pointer receiver. |
| L2315 | `internal/mobilebackup/retained_archive.go:42:6` | the methods of "RetainedArchiveStore" use pointer receiver and non-pointer receiver. |
| L2316 | `internal/mobilebackup/row_page.go:22:6` | the methods of "PreparedRowPage" use pointer receiver and non-pointer receiver. |
| L2317 | `internal/mobilebackup/selected_fetch.go:11:6` | the methods of "SelectedArchive" use pointer receiver and non-pointer receiver. |
| L2318 | `internal/mobilebackup/snapshot.go:41:6` | the methods of "SnapshotStore" use pointer receiver and non-pointer receiver. |
| L2319 | `internal/mobilebackup/snapshot_page.go:51:6` | the methods of "SnapshotPage" use pointer receiver and non-pointer receiver. |
| L2320 | `internal/mobilebackup/snapshot_recalls.go:9:6` | the methods of "snapshotRecallTargets" use pointer receiver and non-pointer receiver. |
| L2321 | `internal/mobilebackup/sqlite.go:43:6` | the methods of "SQLiteBatch" use pointer receiver and non-pointer receiver. |
| L2322 | `internal/mobilebackup/tlv.go:12:6` | the methods of "TLV" use pointer receiver and non-pointer receiver. |
| L2323 | `internal/service/archive_messages.go:16:6` | the methods of "archivePosition" use pointer receiver and non-pointer receiver. |

### revive (98)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Исправить doc comments вместе с godoclint/staticcheck. Неиспользуемые параметры интерфейсных реализаций/fakes обозначить _ без изменения signature; параметры собственных private functions удалить с callers. L2332: перенести ctx первым у private функции с обновлением callers. L2337/L2338: в test helpers сохранить t первым, ctx вторым с локальным nolint:revive и причиной test helper convention. L2336: если Preview используется снаружи collector, дать возвращаемому типу имя PreviewResult и обновить callers; если метод только внутренний — сделать его private. Не вводить interface ради правила. L2390: добавить комментарий у blank import о регистрации SQLite driver. Builtin copy исправить вместе с predeclared. Не переименовывать wire keys или protocol types ради style.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2324 | `internal/app/eventaccess/journal.go:8:1` | exported: comment on exported type Subscription should be of the form "Subscription ..." (with optional leading article) |
| L2325 | `internal/app/eventaccess/journal.go:43:1` | exported: comment on exported method EventData.MarshalJSON should be of the form "MarshalJSON ..." |
| L2326 | `internal/app/history/command.go:10:1` | exported: comment on exported type StartRequest should be of the form "StartRequest ..." (with optional leading article) |
| L2327 | `internal/collector/contact_profiles_test.go:59:50` | unused-parameter: parameter 'ids' seems to be unused, consider removing or renaming it as _ |
| L2328 | `internal/collector/contact_profiles_test.go:200:35` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2329 | `internal/collector/contacts_test.go:50:32` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2330 | `internal/collector/contacts_test.go:121:51` | unused-parameter: parameter 'page' seems to be unused, consider removing or renaming it as _ |
| L2331 | `internal/collector/contacts_test.go:121:57` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2332 | `internal/collector/conversations.go:20:2` | context-as-argument: context.Context should be the first parameter of a function |
| L2333 | `internal/collector/history_test.go:33:43` | unused-parameter: parameter 'ref' seems to be unused, consider removing or renaming it as _ |
| L2334 | `internal/collector/history_test.go:33:71` | unused-parameter: parameter 'cursor' seems to be unused, consider removing or renaming it as _ |
| L2335 | `internal/collector/history_test.go:33:86` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2336 | `internal/collector/join.go:157:64` | unexported-return: exported method Preview returns unexported type collector.preview, which can be annoying to use |
| L2337 | `internal/evalfixture/fixture_test.go:97:35` | context-as-argument: context.Context should be the first parameter of a function |
| L2338 | `internal/evalfixture/fixture_test.go:116:2` | context-as-argument: context.Context should be the first parameter of a function |
| L2339 | `internal/evaluation/process_test.go:17:24` | unused-parameter: parameter 't' seems to be unused, consider removing or renaming it as _ |
| L2340 | `internal/evaluation/scenarios.go:357:1` | exported: comment on exported function DefaultChildOptions should be of the form "DefaultChildOptions ..." |
| L2341 | `internal/events/webhook_test.go:123:10` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2342 | `internal/events/webhook_test.go:123:31` | unused-parameter: parameter 'network' seems to be unused, consider removing or renaming it as _ |
| L2343 | `internal/historyimport/mcp_test.go:125:59` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2344 | `internal/historyimport/mcp_test.go:125:108` | unused-parameter: parameter 'cursor' seems to be unused, consider removing or renaming it as _ |
| L2345 | `internal/historyimport/mcp_test.go:125:123` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2346 | `internal/historyimport/worker_test.go:126:8` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2347 | `internal/historyimport/worker_test.go:185:53` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2348 | `internal/historyimport/worker_test.go:185:102` | unused-parameter: parameter 'cursor' seems to be unused, consider removing or renaming it as _ |
| L2349 | `internal/historyimport/worker_test.go:185:117` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2350 | `internal/historyimport/worker_test.go:206:102` | unused-parameter: parameter 'cursor' seems to be unused, consider removing or renaming it as _ |
| L2351 | `internal/historyimport/worker_test.go:206:117` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2352 | `internal/historyimport/worker_test.go:399:8` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2353 | `internal/historyimport/worker_test.go:399:83` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2354 | `internal/mcpserver/http_test.go:32:2` | redefines-builtin-id: redefinition of the built-in function copy |
| L2355 | `internal/messaging/manager_test.go:138:49` | unused-parameter: parameter 'text' seems to be unused, consider removing or renaming it as _ |
| L2356 | `internal/mobilebackup/archive_tokens.go:9:1` | exported: comment on exported method RetainedArchiveStore.SealArchiveToken should be of the form "SealArchiveToken ..." |
| L2357 | `internal/mobilebackup/control_scan_test.go:75:8` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2358 | `internal/mobilebackup/history_worker_test.go:69:9` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2359 | `internal/mobilebackup/history_worker_test.go:632:8` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2360 | `internal/mobilebackup/offer_logging_test.go:50:8` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2361 | `internal/mobilebackup/offer_runner_test.go:109:10` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2362 | `internal/mobilebackup/offer_runner_test.go:155:8` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2363 | `internal/mobilebackup/offer_runner_test.go:203:8` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2364 | `internal/mobilebackup/offer_runner_test.go:220:32` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2365 | `internal/mobilebackup/own_recall_test.go:20:8` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2366 | `internal/mobilebackup/recall_history_test.go:109:8` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2367 | `internal/mobilebackup/row_page_test.go:46:8` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2368 | `internal/mobilebackup/row_page_test.go:91:10` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2369 | `internal/mobilebackup/row_page_test.go:91:29` | unused-parameter: parameter 'r' seems to be unused, consider removing or renaming it as _ |
| L2370 | `internal/mobilebackup/selected_fetch_test.go:45:46` | unused-parameter: parameter 'r' seems to be unused, consider removing or renaming it as _ |
| L2371 | `internal/mobilebackup/selected_fetch_test.go:55:8` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2372 | `internal/mobilebackup/selected_fetch_test.go:94:48` | unused-parameter: parameter 'r' seems to be unused, consider removing or renaming it as _ |
| L2373 | `internal/mobilebackup/selected_fetch_test.go:106:10` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2374 | `internal/mobilebackup/snapshot_page.go:117:3` | redefines-builtin-id: redefinition of the built-in function copy |
| L2375 | `internal/mobilebackup/sqlite.go:36:1` | exported: comment on exported type SQLiteCoverage should be of the form "SQLiteCoverage ..." (with optional leading article) |
| L2376 | `internal/service/events_restart_test.go:77:2` | redefines-builtin-id: redefinition of the built-in function copy |
| L2377 | `internal/service/history_test.go:41:2` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2378 | `internal/service/history_test.go:43:2` | unused-parameter: parameter 'cursor' seems to be unused, consider removing or renaming it as _ |
| L2379 | `internal/service/history_test.go:44:2` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2380 | `internal/service/history_test.go:63:2` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2381 | `internal/service/history_test.go:65:2` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2382 | `internal/service/mobile_backup_test.go:28:2` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2383 | `internal/service/mobile_page_test.go:54:2` | unused-parameter: parameter 'req' seems to be unused, consider removing or renaming it as _ |
| L2384 | `internal/service/mobile_page_test.go:55:2` | unused-parameter: parameter 'client' seems to be unused, consider removing or renaming it as _ |
| L2385 | `internal/service/run_test.go:37:2` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2386 | `internal/service/sending_test.go:64:2` | unused-parameter: parameter 'peer' seems to be unused, consider removing or renaming it as _ |
| L2387 | `internal/storage/event_journal.go:136:1` | exported: comment on exported method Store.ReadSubscriptionEvents should be of the form "ReadSubscriptionEvents ..." |
| L2388 | `internal/storage/incoming_test.go:138:21` | unused-parameter: parameter 'm' seems to be unused, consider removing or renaming it as _ |
| L2389 | `internal/storage/incoming_test.go:163:21` | unused-parameter: parameter 'm' seems to be unused, consider removing or renaming it as _ |
| L2390 | `internal/storage/store.go:19:2` | blank-imports: a blank import should be only in a main or test package, or have a comment justifying it |
| L2391 | `internal/zalo/contact_profiles_test.go:70:38` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2392 | `internal/zalo/cookies.go:38:3` | redefines-builtin-id: redefinition of the built-in function copy |
| L2393 | `internal/zalo/history_test.go:24:2` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2394 | `internal/zalo/history_test.go:25:2` | unused-parameter: parameter 'id' seems to be unused, consider removing or renaming it as _ |
| L2395 | `internal/zalo/history_test.go:25:6` | unused-parameter: parameter 'cursor' seems to be unused, consider removing or renaming it as _ |
| L2396 | `internal/zalo/history_test.go:26:2` | unused-parameter: parameter 'limit' seems to be unused, consider removing or renaming it as _ |
| L2397 | `internal/zalo/mobile_backup_test.go:35:55` | unused-parameter: parameter 'host' seems to be unused, consider removing or renaming it as _ |
| L2398 | `internal/zalo/mobile_backup_test.go:43:47` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2399 | `internal/zalo/mobile_backup_test.go:53:52` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2400 | `internal/zalo/mobile_backup_test.go:53:81` | unused-parameter: parameter 'groups' seems to be unused, consider removing or renaming it as _ |
| L2401 | `internal/zalo/mobile_backup_test.go:161:51` | unused-parameter: parameter 'p' seems to be unused, consider removing or renaming it as _ |
| L2402 | `internal/zalo/mobile_identities_test.go:22:56` | unused-parameter: parameter 'ctx' seems to be unused, consider removing or renaming it as _ |
| L2403 | `internal/zalo/mobile_identities_test.go:22:77` | unused-parameter: parameter 'direct' seems to be unused, consider removing or renaming it as _ |
| L2404 | `internal/zalo/mobile_identities_test.go:22:85` | unused-parameter: parameter 'groups' seems to be unused, consider removing or renaming it as _ |
| L2405 | `tools/harness/internal/performance/performance_test.go:96:27` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2406 | `tools/harness/internal/performance/performance_test.go:102:29` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2407 | `tools/harness/internal/performance/performance_test.go:102:96` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2408 | `tools/harness/internal/performance/performance_test.go:106:28` | unused-parameter: parameter 'c' seems to be unused, consider removing or renaming it as _ |
| L2409 | `tools/harness/internal/performance/performance_test.go:111:21` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2410 | `tools/harness/internal/performance/performance_test.go:111:103` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2411 | `tools/harness/internal/performance/performance_test.go:111:179` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2412 | `tools/harness/internal/performance/performance_test.go:111:243` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2413 | `tools/harness/internal/performance/performance_test.go:111:320` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2414 | `tools/harness/internal/performance/performance_test.go:111:409` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2415 | `tools/harness/internal/performance/performance_test.go:111:485` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2416 | `tools/harness/internal/performance/performance_test.go:111:900` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2417 | `tools/harness/internal/performance/performance_test.go:112:16` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2418 | `tools/harness/internal/performance/performance_test.go:112:100` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2419 | `tools/harness/internal/performance/performance_test.go:112:211` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2420 | `tools/harness/internal/performance/performance_test.go:112:327` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |
| L2421 | `tools/harness/internal/performance/performance_test.go:112:429` | unused-parameter: parameter 'b' seems to be unused, consider removing or renaming it as _ |

### sloglint (63)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Использовать уже внедрённый logger и *Context методы там, где ctx есть. Для composition root/startup и существующего глобально настроенного redacting logger допустим локальный nolint:sloglint с причиной владения logger. Не протаскивать logger через domain только ради правила. Сохранить bounded allowlist полей; raw err/payload/account IDs не добавлять в лог для обработки иных lint ошибок.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2422 | `cmd/zl-mcp-eval-fixture/main.go:71:3` | default logger should not be used |
| L2423 | `internal/collector/join.go:392:4` | default logger should not be used |
| L2424 | `internal/collector/join.go:405:4` | default logger should not be used |
| L2425 | `internal/collector/run.go:164:5` | default logger should not be used |
| L2426 | `internal/historyimport/worker.go:249:2` | default logger should not be used |
| L2427 | `internal/mcpserver/discovery_logging.go:115:2` | default logger should not be used |
| L2428 | `internal/mcpserver/server.go:257:3` | default logger should not be used |
| L2429 | `internal/mobilebackup/account_fetch.go:81:2` | default logger should not be used |
| L2430 | `internal/mobilebackup/archive.go:45:3` | default logger should not be used |
| L2431 | `internal/mobilebackup/archive.go:61:3` | default logger should not be used |
| L2432 | `internal/mobilebackup/blocks.go:57:3` | default logger should not be used |
| L2433 | `internal/mobilebackup/blocks.go:91:3` | default logger should not be used |
| L2434 | `internal/mobilebackup/download.go:107:3` | default logger should not be used |
| L2435 | `internal/mobilebackup/download.go:111:3` | default logger should not be used |
| L2436 | `internal/mobilebackup/download.go:122:3` | default logger should not be used |
| L2437 | `internal/mobilebackup/download.go:126:3` | default logger should not be used |
| L2438 | `internal/mobilebackup/download.go:140:3` | default logger should not be used |
| L2439 | `internal/mobilebackup/encrypted_archive.go:41:3` | default logger should not be used |
| L2440 | `internal/mobilebackup/encrypted_archive.go:46:3` | default logger should not be used |
| L2441 | `internal/mobilebackup/offer_runner.go:37:4` | default logger should not be used |
| L2442 | `internal/mobilebackup/offer_runner.go:45:4` | default logger should not be used |
| L2443 | `internal/mobilebackup/offer_runner.go:65:2` | default logger should not be used |
| L2444 | `internal/mobilebackup/offer_runner.go:85:3` | default logger should not be used |
| L2445 | `internal/mobilebackup/offer_runner.go:106:4` | default logger should not be used |
| L2446 | `internal/mobilebackup/offer_runner.go:118:3` | default logger should not be used |
| L2447 | `internal/mobilebackup/prepared_page.go:57:4` | default logger should not be used |
| L2448 | `internal/mobilebackup/selected_fetch.go:49:2` | default logger should not be used |
| L2449 | `internal/mobilebackup/selected_fetch.go:59:3` | default logger should not be used |
| L2450 | `internal/mobilebackup/sqlite.go:182:4` | default logger should not be used |
| L2451 | `internal/mobilebackup/sqlite.go:207:3` | default logger should not be used |
| L2452 | `internal/service/mobile_archive_probe.go:125:7` | default logger should not be used |
| L2453 | `internal/service/mobile_archive_probe.go:167:3` | default logger should not be used |
| L2454 | `internal/service/mobile_archive_probe.go:174:2` | default logger should not be used |
| L2455 | `internal/service/mobile_backup.go:113:3` | default logger should not be used |
| L2456 | `internal/service/run.go:194:3` | default logger should not be used |
| L2457 | `internal/service/run.go:228:4` | default logger should not be used |
| L2458 | `internal/service/run.go:258:3` | default logger should not be used |
| L2459 | `internal/service/run.go:268:3` | default logger should not be used |
| L2460 | `internal/service/run.go:381:2` | default logger should not be used |
| L2461 | `internal/service/run.go:421:2` | default logger should not be used |
| L2462 | `internal/service/run.go:585:4` | default logger should not be used |
| L2463 | `internal/service/run.go:628:3` | default logger should not be used |
| L2464 | `internal/zalo/adapter.go:283:2` | default logger should not be used |
| L2465 | `internal/zalo/events.go:75:4` | default logger should not be used |
| L2466 | `internal/zalo/events.go:78:4` | default logger should not be used |
| L2467 | `internal/zalo/events.go:178:3` | default logger should not be used |
| L2468 | `internal/zalo/events.go:199:2` | default logger should not be used |
| L2469 | `internal/zalo/events.go:218:2` | default logger should not be used |
| L2470 | `internal/zalo/mobile_backup.go:134:4` | default logger should not be used |
| L2471 | `internal/zalo/mobile_backup.go:145:4` | default logger should not be used |
| L2472 | `internal/zalo/mobile_backup.go:182:6` | default logger should not be used |
| L2473 | `internal/zalo/mobile_backup.go:186:6` | default logger should not be used |
| L2474 | `internal/zalo/mobile_backup.go:192:6` | default logger should not be used |
| L2475 | `internal/zalo/mobile_backup.go:207:6` | default logger should not be used |
| L2476 | `internal/zalo/mobile_backup.go:211:6` | default logger should not be used |
| L2477 | `internal/zalo/mobile_backup.go:214:5` | default logger should not be used |
| L2478 | `internal/zalo/mobile_backup.go:217:6` | default logger should not be used |
| L2479 | `internal/zalo/mobile_backup.go:227:5` | default logger should not be used |
| L2480 | `internal/zalo/mobile_backup.go:289:2` | default logger should not be used |
| L2481 | `internal/zalo/mobile_backup.go:325:2` | default logger should not be used |
| L2482 | `internal/zalo/mobile_identities.go:62:3` | default logger should not be used |
| L2483 | `internal/zalo/mobile_identities.go:67:3` | default logger should not be used |
| L2484 | `internal/zalo/replay.go:62:3` | default logger should not be used |

### spancheck (6)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2485–L2490: End уже находится в deferred closure с recover, outcome attributes и повторным panic. Оставить локальный nolint:spancheck на диагностируемых местах, объяснив closure lifecycle; не добавлять второй span.End и не переставлять End до SetAttributes. Существующие panic/identity/span tests обязательны. Это обоснование путей next(), не обещание корректности произвольного паникующего exporter.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2485 | `internal/observability/observer.go:152:2` | span.End is not called on all paths, possible memory leak |
| L2486 | `internal/observability/observer.go:169:2` | return can be reached without calling span.End |
| L2487 | `internal/observability/observer.go:189:2` | span.End is not called on all paths, possible memory leak |
| L2488 | `internal/observability/observer.go:210:2` | return can be reached without calling span.End |
| L2489 | `internal/observability/observer.go:296:2` | span.End is not called on all paths, possible memory leak |
| L2490 | `internal/observability/observer.go:311:2` | return can be reached without calling span.End |

### sqlclosecheck (14)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Добавить defer rows.Close сразу после успешного Query в ограниченном helper scope, а явное раннее закрытие сохранить там, где оно освобождает connection перед следующей операцией. Либо локальный nolint:sqlclosecheck для намеренного early close, если все error/return paths уже покрыты. В циклах не накапливать defer до конца всей операции. rows.Err и ошибки закрытия/commit проверять отдельно.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2491 | `internal/collector/join.go:484:14` | Close should use defer |
| L2492 | `internal/collector/join.go:550:14` | Close should use defer |
| L2493 | `internal/mobilebackup/sqlite.go:305:17` | Close should use defer |
| L2494 | `internal/mobilebackup/sqlite.go:361:21` | Close should use defer |
| L2495 | `internal/storage/browse.go:107:14` | Close should use defer |
| L2496 | `internal/storage/conversation_views.go:150:14` | Close should use defer |
| L2497 | `internal/storage/deliveries.go:95:14` | Close should use defer |
| L2498 | `internal/storage/deliveries.go:150:15` | Close should use defer |
| L2499 | `internal/storage/event_journal.go:189:14` | Close should use defer |
| L2500 | `internal/storage/events.go:47:14` | Close should use defer |
| L2501 | `internal/storage/history_operations.go:568:14` | Close should use defer |
| L2502 | `internal/storage/performance_test.go:39:19` | Close should use defer |
| L2503 | `internal/storage/store.go:680:14` | Close should use defer |
| L2504 | `internal/storage/views.go:40:14` | Close should use defer |

### staticcheck (15)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Doc diagnostics исправить. L2508 — receive из done сохранить, удалить только присваивание. L2513 — два явных последовательных Remove с отдельными assertions: второй проверяет идемпотентность, удалять нельзя. L2510/L2511: lowercase только если error text не зафиксирован wire/CLI контрактом, иначе локальный nolint:staticcheck с причиной сохранения текста. L2516/L2519 De Morgan допустим без смены short-circuit. L2517/L2518: protocol compatibility exception, см. адресные решения; не заменять RSA padding в одностороннем порядке.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2505 | `internal/app/eventaccess/journal.go:8:1` | ST1021: comment on exported type Subscription should be of the form "Subscription ..." (with optional leading article) |
| L2506 | `internal/app/eventaccess/journal.go:43:1` | ST1020: comment on exported method MarshalJSON should be of the form "MarshalJSON ..." |
| L2507 | `internal/app/history/command.go:10:1` | ST1021: comment on exported type StartRequest should be of the form "StartRequest ..." (with optional leading article) |
| L2508 | `internal/evaluation/process.go:70:3` | SA4006: this value of err is never used |
| L2509 | `internal/evaluation/scenarios.go:357:1` | ST1020: comment on exported function DefaultChildOptions should be of the form "DefaultChildOptions ..." |
| L2510 | `internal/mcpserver/http.go:42:15` | ST1005: error strings should not be capitalized |
| L2511 | `internal/messaging/manager.go:16:19` | ST1005: error strings should not be capitalized |
| L2512 | `internal/mobilebackup/archive_tokens.go:9:1` | ST1020: comment on exported method SealArchiveToken should be of the form "SealArchiveToken ..." |
| L2513 | `internal/mobilebackup/retained_archive_test.go:198:8` | SA4000: identical expressions on the left and right side of the '\|\|' operator |
| L2514 | `internal/mobilebackup/sqlite.go:36:1` | ST1021: comment on exported type SQLiteCoverage should be of the form "SQLiteCoverage ..." (with optional leading article) |
| L2515 | `internal/storage/event_journal.go:136:1` | ST1020: comment on exported method ReadSubscriptionEvents should be of the form "ReadSubscriptionEvents ..." |
| L2516 | `internal/storage/subscription_scope.go:48:33` | QF1001: could apply De Morgan's law |
| L2517 | `internal/zalo/mobile_backup.go:270:16` | SA1019: crypto/rsa.DecryptPKCS1v15 has been deprecated since Go 1.26 because it shouldn't be used: PKCS #1 v1.5 encryption is dangerous and should not be used. Whether this function returns an error or not discloses secret information. If an attacker can cause this function to run repeatedly and learn whether each instance returned an error then they can decrypt and forge signatures as if they had the private key. See [draft-irtf-cfrg-rsa-guidance-05] for more information. Use [EncryptOAEP] and [DecryptOAEP] instead. |
| L2518 | `internal/zalo/mobile_backup_test.go:84:17` | SA1019: crypto/rsa.EncryptPKCS1v15 has been deprecated since Go 1.26 because it shouldn't be used: PKCS #1 v1.5 encryption is dangerous and should not be used. See [draft-irtf-cfrg-rsa-guidance-05] for more information. Use [EncryptOAEP] and [DecryptOAEP] instead. |
| L2519 | `tools/harness/internal/performance/json.go:98:6` | QF1001: could apply De Morgan's law |

### unparam (1)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2520: убрать неиспользуемый return Store из sdkBlockingHTTP и обновить callers. Сам store и его cleanup внутри helper сохранить.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2520 | `internal/mcpserver/sdk_lifecycle_test.go:57:51` | sdkBlockingHTTP - result 1 (*github.com/skosovsky/zl-mcp/internal/storage.Store) is never used |

### unqueryvet (2)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2521/L2522: оставить m.* в versioned view migration с локальным nolint:unqueryvet: view intentionally projects full messages schema. Замена списка полей только в старой миграции даст разное поведение свежей и существующей БД. Если понадобится сузить view — отдельная persistence migration и upgrade tests.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2521 | `internal/storage/history_expiry.go:19:31` | avoid SELECT * - explicitly specify needed columns for better performance, maintainability and stability |
| L2522 | `internal/storage/tombstones.go:25:3` | avoid SELECT * - explicitly specify needed columns for better performance, maintainability and stability |

### unused (4)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2523/L2524/L2546/L2547: удалить неиспользуемые private helpers после поиска ссылок во всех build-tag/platform файлах и compile checks. Не оставлять compatibility wrappers или nolint:unused. Тестовые assertions, которые ранее вызывали helper, должны сохраниться после inlining.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2546 | `internal/observability/observer_test.go:325:6` | func observedAge is unused |
| L2523 | `internal/service/mobile_archive_probe.go:16:26` | func (*membershipPort).probeMobileArchive is unused |
| L2524 | `internal/service/mobile_backup.go:45:26` | func (*membershipPort).fetchPreparedMobileArchive is unused |
| L2547 | `internal/zalo/sending_test.go:55:6` | func codePointer is unused |

### usetesting (14)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** Обычные независимые каталоги заменить t.TempDir. Тесты Unix sockets с коротким /tmp пути сохранять с nolint:usetesting и явным t.Cleanup(os.RemoveAll), иначе длинное имя теста может превысить лимит socket path. L2526: TMPDIR child можно задать отдельным t.TempDir, если проверен bound socket path. L2538: shared sync.Once worker нельзя привязать к первому t.TempDir — файл исчезнет до следующих tests; TestMain владеет build+RemoveAll либо build fixture без global caching.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2525 | `internal/collector/run_test.go:36:14` | os.MkdirTemp() could be replaced by t.TempDir() in sessionStore |
| L2526 | `internal/mcpserver/sdk_stdio_identity_test.go:76:93` | os.TempDir() could be replaced by t.TempDir() in sdkStartRawBridge |
| L2527 | `internal/mobilebackup/native_binary_test.go:52:16` | os.MkdirTemp() could be replaced by t.TempDir() in TestNativeBinaryTerminalStartupAcceptance |
| L2528 | `internal/service/cli_recall_command_test.go:18:14` | os.MkdirTemp() could be replaced by t.TempDir() in TestRecallCommandRoutesThroughExistingOwnerSocket |
| L2529 | `internal/service/conversations_test.go:125:14` | os.MkdirTemp() could be replaced by t.TempDir() in testConversationServiceProfile |
| L2530 | `internal/service/events_restart_test.go:126:14` | os.MkdirTemp() could be replaced by t.TempDir() in TestServiceEventsSurviveRestartWithStableDeliveryAndBoundary |
| L2531 | `internal/service/history_test.go:89:16` | os.MkdirTemp() could be replaced by t.TempDir() in TestHTTPHistoryToolsUseExistingCollectorSession |
| L2532 | `internal/service/mobile_backup_recovery_test.go:23:12` | os.MkdirTemp() could be replaced by t.TempDir() in TestServiceRecoversMobileAttemptsBeforeExposingEndpoint |
| L2533 | `internal/service/mobile_ledger_control_test.go:212:12` | os.MkdirTemp() could be replaced by t.TempDir() in TestMobileLedgerUsesProductionControlSocketWithoutSessionRestore |
| L2534 | `internal/service/multiprocess_test.go:42:14` | os.MkdirTemp() could be replaced by t.TempDir() in TestTwoMCPProcessesReadCollectorWrites |
| L2535 | `internal/service/preload_probe_test.go:44:14` | os.MkdirTemp() could be replaced by t.TempDir() in TestPrivatePreloadProbeUsesServiceSessionWithoutPersistingCorpus |
| L2536 | `internal/service/run_test.go:53:16` | os.MkdirTemp() could be replaced by t.TempDir() in TestServicePreservesMCPWhenAuthenticationIsRequired |
| L2537 | `internal/service/sending_test.go:81:14` | os.MkdirTemp() could be replaced by t.TempDir() in TestServiceSendAndReplyUseCollectorSessionAndPersistAcrossRestart |
| L2538 | `tools/harness/internal/performance/performance_test.go:206:13` | os.MkdirTemp() could be replaced by t.TempDir() in worker |

### wastedassign (5)

**Решение пользователя:**

_Заполнить._

**Предлагаемое решение Ann:** L2541 объединить с L1740. L2539/L2540/L2542: убрать только начальное значение, если оно всегда заменяется до чтения; сохранить вызовы с effects. L2543: вместо бесполезного a=... убрать assignment или явно проверить новое состояние, если это цель сценария. Не удалять operations по признаку неиспользуемого результата.

| ID | Файл:строка:колонка | Замечание |
|---|---|---|
| L2539 | `internal/evaluation/benchmark.go:54:2` | assigned to name, but reassigned without using the value |
| L2540 | `internal/evaluation/model.go:31:3` | assigned to t, but reassigned without using the value |
| L2541 | `internal/evaluation/process.go:70:3` | assigned to err, but reassigned without using the value |
| L2542 | `internal/events/subscriptions.go:136:2` | assigned to name, but reassigned without using the value |
| L2543 | `internal/storage/mobile_backup_test.go:255:2` | assigned to a, but never used afterwards |

## Не воспроизводятся в текущем запуске

| ID | Правило | Прежнее место | Прежнее замечание |
|---|---|---|---|
| L0512 | `fatcontext` | `internal/mcpserver/sdk_lifecycle_test.go:220:3` | nested context in loop |
| L0513 | `fatcontext` | `internal/mcpserver/sdk_stdio_identity_test.go:279:3` | nested context in loop |
| L0514 | `fatcontext` | `internal/observability/observer_test.go:265:3` | nested context in function literal |
| L1375 | `golines` | `internal/mobilebackup/message_conversion.go:162:1` | File is not properly formatted |
| L1618 | `govet` | `internal/observability/observer_test.go:192:78` | inline: Call of observability.observedAge should be inlined |
| L1619 | `govet` | `internal/observability/observer_test.go:202:3` | inline: Call of observability.observedAge should be inlined |
| L1620 | `govet` | `internal/observability/observer_test.go:314:3` | inline: Call of observability.observedAge should be inlined |
| L1621 | `govet` | `internal/observability/provider_test.go:154:93` | inline: Call of observability.observedAge should be inlined |
| L1729 | `govet` | `internal/zalo/sending_test.go:23:39` | inline: Call of zalo.codePointer should be inlined |
| L1730 | `govet` | `internal/zalo/sending_test.go:24:20` | inline: Call of zalo.codePointer should be inlined |
| L1731 | `govet` | `internal/zalo/sending_test.go:24:68` | inline: Call of zalo.codePointer should be inlined |
| L1732 | `govet` | `internal/zalo/sending_test.go:25:21` | inline: Call of zalo.codePointer should be inlined |
| L1733 | `govet` | `internal/zalo/sending_test.go:26:31` | inline: Call of zalo.codePointer should be inlined |
| L1734 | `govet` | `internal/zalo/sending_test.go:27:23` | inline: Call of zalo.codePointer should be inlined |
| L1741 | `intrange` | `internal/mobilebackup/header_test.go:106:6` | for loop can be changed to &#96;n := range golden&#96; |
| L1742 | `intrange` | `internal/mobilebackup/snapshot_recalls_test.go:180:5` | for loop can be changed to use an integer range (Go 1.22+) |
| L2545 | `typecheck` | `internal/mcpserver/catalog_diagnostics.go:1:0` | : # github.com/skosovsky/zl-mcp/internal/mcpserver [github.com/skosovsky/zl-mcp/internal/mcpserver.test]<br>internal/mcpserver/sdk_lifecycle_test.go:220:3: non-name invocations[v.message] on left side of :=<br>internal/mcpserver/sdk_stdio_identity_test.go:279:3: non-name invocations[v.message] on left side of := |
| L2544 | `whitespace` | `internal/evaluation/process_test.go:93:57` | unnecessary leading newline |
