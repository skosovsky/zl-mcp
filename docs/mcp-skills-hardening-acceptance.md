# Приёмка: надёжность MCP и skills

Статус: реализация продолжается, финальная приёмка не выполнена.

Основание: [задача R01–R12](task-mcp-skills-hardening.md). Исходная ревизия: `a3e0baea4396a30fa6902cea6b831bfe39956001`.

Матрица фиксирует обязательный знаменатель. Подкритерии задачи сохраняют силу; объединённая строка PASS только при доказательстве всех её частей. FAIL/NOT_RUN не засчитываются. Удаление критерия или N/A требует согласования scope, а не редактирования процента.

| Критерий | Ожидаемое доказательство | Статус | Фактическое доказательство |
| --- | --- | --- | --- |
| R01.1 | Контракт блокировки/retry и совместимость зафиксированы | NOT_RUN | — |
| R01.2 | До2xx/между2xxиcommit: последнее событие возобновляется без нового сообщения | NOT_RUN | — |
| R01.3 | Блокировка дольше retry-окна и перезапуск сохраняют продолжение | NOT_RUN | — |
| R01.4 | Повтор/ошибка/отмена не создают busy loop, пропуск ack или ложный delivered | NOT_RUN | — |
| R02.1 | Компактный режим явный; старые запросы сохраняют полное представление | NOT_RUN | — |
| R02.2 | Anchor/search/browse доступны при800+gaps в бюджете ответа | NOT_RUN | — |
| R02.3 | Все gaps читаются по snapshot; append/update/delete/expiry определены | NOT_RUN | — |
| R02.4 | Пагинация corpus browse не теряет строки; current policy rechecked | NOT_RUN | — |
| R03.1 | Tools-only читает полный Unicode-текст корпуса ограниченными страницами | NOT_RUN | — |
| R03.2 | Архивная идентичность отдельная; полное чтение архива и старые resources работают | NOT_RUN | — |
| R03.3 | Пустой/граничный текст, неверные/истёкшие/подменённые курсоры проверены | NOT_RUN | — |
| R03.4 | Policy/retention/recall между страницами; нет upstream writes/media fetch | NOT_RUN | — |
| R04.1 | Discovery отличает deferred tools/unsupported host/отсутствующее подключение | NOT_RUN | — |
| R04.2 | Typed context назван точно; правильный unavailable fallback | NOT_RUN | — |
| R05.1 | README/spec/skills/system reference соответствуют принятым возможностям | NOT_RUN | — |
| R05.2 | Исторические ограничения/отчёты не превращены в новые доказательства | NOT_RUN | — |
| R06.1 | Прореживание refs поддерживает транзитивность/циклы/escaping | NOT_RUN | — |
| R06.2 | Результаты проверки валидных/невалидных fixtures эквивалентны | NOT_RUN | — |
| R06.3 | Manifest/baselines синхронны; размер каталога до/после зафиксирован | NOT_RUN | — |
| R07.1 | Backend сообщает настоящие version/revision/dirty либо явный unknown | NOT_RUN | — |
| R07.2 | STDIO отличает build bridge и backend без приватных путей | NOT_RUN | — |
| R07.3 | Версия протокола независима; identity проверена на установленной сборке | NOT_RUN | — |
| R08.1 | Journal причины из closed enum; старый gap_reason сохранён | NOT_RUN | — |
| R08.2 | Stable delivery reference переживает retry/restart; attempt ID различается | NOT_RUN | — |
| R08.3 | Client correlation ID совпадает с server log | NOT_RUN | — |
| R08.4 | Нет payload/секретов/адресов/native IDs и high-cardinality metric labels | NOT_RUN | — |
| R09.1 | Ограниченный browse после пустого поиска при известном чате/периоде | NOT_RUN | — |
| R09.2 | Entrypoints маршрутизируют в references без потери trust/permission правил | NOT_RUN | — |
| R09.3 | Frontmatter/ссылки и safe walkthrough всех перечисленных сценариев проверены | NOT_RUN | — |
| R09.4 | Исходные/установленные skills совпадают; сохранённое задание обновлено поддерживаемым путём | NOT_RUN | — |
| R10.1 | Legacy search/context описаны как group-only compatibility | NOT_RUN | — |
| R10.2 | Typed предпочтительный путь виден; metadata tools и legacy consumers сохранены | NOT_RUN | — |
| R11.1 | Генератор/верификатор живёт в repo, не зависит от/tmp или private runtime | NOT_RUN | — |
| R11.2 | CI проверяет каталог/описания/поля/resources/Events/reactions/CLI/ссылки | NOT_RUN | — |
| R11.3 | Архитектурная проза сверена ревью отдельно от генерации | NOT_RUN | — |
| R12.1 | Корневые test/race/vet/lint/architecture и обязательные existing offline проверки PASS | NOT_RUN | — |
| R12.2 | Вложенныйzcago и tooling: обязательные CI проверки PASS | NOT_RUN | — |
| R12.3 | CGO0 macOSarm64/Linuxamd64 и native smoke PASS | NOT_RUN | — |
| R12.4 | Публичная точная ревизия: подписанный commit/push и зелёныйCI | NOT_RUN | — |
| R12.5 | Проверенный приватный backup/rollback; сохранены данные/config/subscriptions/checkpoints | NOT_RUN | — |
| R12.6 | Установленный backend: revision/readiness/safeHTTP+STDIO чтение PASS | NOT_RUN | — |
| R12.7 | Независимый reviewer1:100% критериев,0 нерешённых ошибок | NOT_RUN | — |
| R12.8 | Независимый reviewer2:100% критериев,0 нерешённых ошибок | NOT_RUN | — |

## Результаты проверок

Локальные проверки в ходе разработки не являются итоговой проверкой ревизии. Выбранные journal/diagnostics/content tests выполнялись; полный повтор после интеграции обязателен. Новые модельные evals не запускались. Рабочий сервис не обновлялся.

## Решения и открытые зависимости

- R01: реализован опциональный companion `zalo.subscription.journal.ready.v1` с отдельной durable-очередью и автономным sweep всех неподтверждённых business heads; чтение блокировки не требуется для запуска recovery. [Контракт и ограничения](contracts/subscription-recovery-ready.md) закрепляют отдельные control IDs, сохранение business checkpoint/payload и прекращение напоминаний после ack. Actual-SDK synthetic host проверяет обе гонки и восстановление без следующего сообщения; это не доказательство поддержки реальным host. HTTP 2xx означает только приём control callback. Production companion и сохранённое задание host автоматически не менялись; их явное подключение и проверка на целевом host остаются NOT_RUN. Поэтому окончательная R01/R12 приёмка ещё не подтверждена.
- R02: coverage_mode=full по умолчанию сохраняет прежние поля; compact является явным opt-in.
- R07: build identity добавляется в MCP _meta, не изменяя сохранённые operation outcomes.
- R08: diagnostics journal являются opt-in include_diagnostics; reasons закрыты, delivery reference keyed и не содержит native identity.

## Финальные независимые verdict

Reviewer1: NOT_RUN. Полнота не установлена. Число ошибок не установлено.

Reviewer2: NOT_RUN. Полнота не установлена. Число ошибок не установлено.

## Разработка: промежуточные доказательства (не финальный verdict)

На рабочем дереве 2026-10-18 подтверждены:

- Root `go test ./...`: PASS (`/tmp/zl-hardening-root-test4.log`).
- Root `go vet ./...`: PASS до последней тестовой правки; повтор входит в release gate.
- Закреплённый zcago: test/vet PASS (`/tmp/zl-hardening-zcago-{test,vet}.log`).
- Обязательный существующий offline profile: cases50/50, assertions480/480;
  model=NOT_RUN, live=NOT_RUN (`/tmp/zl-hardening-offline.log`).
- Публикуемые схемы:132688→94808байт (-37880) на промежуточном каталоге;
  итоговый размер после всех контрактных решений пересчитать.
- SDK coverage800gaps, Unicode corpus/archive content, build identity и безопасная
  delivery/error correlation проверены targeted tests. Дополнительный контроль
  размера полного MCP-ответа для сильно экранированного текста исправлен и принят
  targeted SDK/race-тестами: обе копии текста, длинные archive URI/cursors и SDK
  metadata укладываются в64KiB, полное содержимое восстанавливается без потерь.
- Architecture harness:100% compliance/coverage и negative controls на
  промежуточном кандидате; итоговая ревизия подлежит повторной сверке.
- Repo reference checker и5negativecontrols PASS:27tools/5resources/4templates/
  5Events/54reactions/20CLI.

Пути /tmp — временные локальные логи разработки, не зависимости CI и не
публикуемые приватные артефакты. Итоговая приёмка должна опираться на воспроизводимые
команды, сохранённый публичный отчёт и CI принятой ревизии. Production backend,
установленные skills и сохранённое задание Ann ещё не обновлялись. Ревьюеры
реализации не являются двумя независимыми финальными рецензентами.

Дополнение: root race-regression PASS (`/tmp/zl-hardening-root-race.log`), zcago
race PASS; CGO0 сборки darwin/arm64, darwin/amd64 и linux/amd64 выполнены;
архитектурный harness test/race/vet PASS. После этого исправление wire budget
отдельно проверено normal/race SDK tests. Это не заменяет release-проверки
будущей итоговой ревизии и не закрывает R01/R12.

Общий lint после последних исправлений:0issues (`/tmp/zl-hardening-lint-candidate4.log`).

## Независимое предварительное ревью

[Отчёт кандидата](reviews/mcp-skills-hardening-candidate.md): два независимых
первых verdict NOT ACCEPTED, каждый25/43 (58,14%). Замечания к ошибкам content
read, legacy URI, устаревшей прозе и доказательству эквивалентности исправлены
и повторно проверены рецензентами. R01 и итоговый выпуск остаются открытыми.
Повторный предварительный verdictB:28/43 (65,12%), FAIL0, NOT_RUN15; итоговая
приёмка не пройдена. Новый общий процентA после исправлений не вычислялся.

Read-only baseline установленного бинарника: SHA256
`26bb0221fc32528bfadd857a9d9cc5056714600cb9719b48196baaa371092937`,
revision `a3e0baea4396a30fa6902cea6b831bfe39956001`, dirty=false, CGO0 darwin/arm64.
MCP status: authenticated/connected,119stored messages, last_error=null на момент
проверки; это наблюдение, не frozen snapshot и не доказательство финального rollout.
Native startup synthetic smoke PASS (`/tmp/zl-hardening-native-smoke.log`).
Pinned gitleaks8.24.3 проверил дерево: no leaks found
(`/tmp/zl-hardening-secrets-candidate.log`); новые правки перед публикацией
подлежат повторному сканированию.

## Интеграция R01: очередной кандидат

Optional `zalo.subscription.journal.ready.v1` реализован; действующий контракт —
[subscription recovery-ready](contracts/subscription-recovery-ready.md).
Actual HTTP MCP SDK + TLS/HMAC synthetic host проверяет обе race windows,
возобновление без нового бизнес-события, два перезапуска, stable retry/fresh
reminder, duplicate handling, ordered ack и explicit terminal gap. Это проверка
документированного адаптера, а не реального Ann.

Полные root test/vet и обязательный offline profile прошли (50/50 cases,
480/480 assertions; model/live NOT_RUN). В полном race-прогоне обнаружен timeout
пятисекундного startup ожидания существующего history service теста при
параллельной нагрузке; требуется перепроверка, PASS пока не заявляется.
Архитектура, tooling test/race/vet, nested zcago test/race/vet, CGO0 cross-builds
и native startup/restart smoke прошли. Последующие изменения по замечаниям
рецензентов требуют повторной проверки затронутых пакетов.

Обе установленные skills синхронизированы с исходниками: research 8 файлов,
events 9 файлов, побайтовые SHA-сверки PASS; прежние копии сохранены в проверенном
приватном backup. Сохранённое host задание этим не обновлено. Реальная активация
companion, публикация/CI и установленный backend остаются незавершёнными gates.

Последующие независимые замечания C06 (broad revoked gap), C07 (control log
correlation) и C08 (rollback gate) исправлены и независимо перепроверены.
Штатные валидаторы обеих skills PASS; source/installed equality дополнительно
подтверждена reviewer A. Scanner gitleaks 8.24.3 не нашёл секретов в дереве
кандидата; перед публикацией проверяется staged состав.
Изолированный history startup race-тест повторён трижды: PASS; полный race
повторяется с -p 2, чтобы проверить весь набор без прежней перегрузки.

Итоговый каталог после R01: 27 tools, 6 Events. Суммарный compact JSON входных/выходных схем: 135634 → 97754 байт; 11 output schemas прорежены, экономия 37880 байт. Это размер схем, не измерение токенов host.

[Статический walkthrough skills](reviews/mcp-skills-hardening-skill-walkthrough.md):
10/10 маршрутов, включая восемь обязательных сценариев R09.3; проверены
frontmatter, ссылки, 24 зависимости tools, актуальные поля контрактов и 17/17
файлов source/installed. Это доказательство согласованности инструкций, не
модельный eval и не выполнение сохранённого задания реальным host.

Поддерживаемый маршрут изменения saved automation в текущем окружении не
доступен: inventory tools не содержит управления automation, UI-инструмент
отказал в доступе к com.openai.codex. Подписка Ann не изменялась; это открытый
операционный gate, а не основание читать внутреннее хранилище host или обходить
ограничение UI. Остальные работы по выпуску продолжаются.

### Результат общего повторного прогона кандидата

После последних исправлений root `go test -p 2 ./...`, `go test -race -p 2 ./...`,
`go vet ./...` и `make lint` завершились exit 0. Полный race PASS заменяет
предыдущее падение startup ожидания; последний normal/race набор проверяет
интегрированные recovery, rollback и diagnostic изменения. Остальные обязательные
проверки и ограничения перечислены выше. Повторный gitleaks 8.24.3 после всех
правок: no leaks found. Это локальные доказательства кандидата до подписанного
коммита, CI и runtime rollout, а не финальный verdict двух reviewers.
