# Приёмка: надёжность MCP и skills

Статус: серверная реализация опубликована и установлена (`c67567c5`); приёмка40/43 (93,02%),0выявленных нерешённых ошибок. Открыт R09.4 и зависимые финальные verdict.

Основание: [задача R01–R12](task-mcp-skills-hardening.md). Исходная ревизия: `a3e0baea4396a30fa6902cea6b831bfe39956001`.

Матрица фиксирует обязательный знаменатель. Подкритерии задачи сохраняют силу; объединённая строка PASS только при доказательстве всех её частей. FAIL/NOT_RUN не засчитываются. Удаление критерия или N/A требует согласования scope, а не редактирования процента.

| Критерий | Ожидаемое доказательство | Статус | Фактическое доказательство |
| --- | --- | --- | --- |
| R01.1 | Контракт persisted/default; независимые ready/transport/ack и совместимость зафиксированы | PASS | docs/contracts/journal-processing-readiness.md and executable opt-in readiness_mode=persisted: ready/transport/ack independent; default delivery_terminal strict representation unchanged. |
| R01.2 | До2xx/между2xxиcommit: read+ack без companion, честный pending/sending и без нового сообщения | PASS | Independent actualSDK HTTP/TLS -race TestSyntheticHostPersistedJournalAcknowledgesBothCallbackRaceWindowsWithoutCompanion: read+ack before2xx and post2xx/precommit, sending truthful, no companion/new message. |
| R01.3 | Early ack, retry/restart сохраняют прогресс; отдельно проверено продолжение потерянного/прерванного запуска | PASS | Independent -race actualSDK503/timeout+restart proof keeps processing checkpoint and exact callback ID/body; separate companion crash/missing-run recovery prior synthetic tests preserved. No real-host activation claim. |
| R01.4 | Ошибка/отмена/gap/pruning сохраняют checkpoint/leases; нет busy loop, пропуска ack или ложного delivered | PASS | Persisted order/generation/permissions/recall/retention/cancel/410 tests; earlyack leaves workerbytes/lease unchanged; malformed nested/100k/domain-invalid activity normalized before budget to receipt-bearing gap; validoversized remains error not false gap/ack. |
| R02.1 | Компактный режим явный; старые запросы сохраняют полное представление | PASS | coverage_mode full default; compact opt-in discriminator; existing full keys; TestCoverageGapsSnapshotAndCurrentPolicy. |
| R02.2 | Anchor/search/browse доступны при800+gaps в бюджете ответа | PASS | SDK budget tests for800+ gaps and bounded compact anchor/search/browse; independent SDK budget tests PASS; exact-c675 CI39115464885 completedSUCCESS bothOS. |
| R02.3 | Все gaps читаются по snapshot; append/update/delete/expiry определены | PASS | coverage_gaps.go authenticated snapshot high-water/digest; append excluded, update/delete expires,24h expiry; coverage tests. |
| R02.4 | Пагинация corpus browse не теряет строки; current policy rechecked | PASS | BrowsePage freezes insertion snapshot; reduction rereads current policy; storage and SDK browse budget tests. |
| R03.1 | Tools-only читает полный Unicode-текст корпуса ограниченными страницами | PASS | actual SDK Unicode reconstruction and duplicated wire byte-budget tests; corpus owner rechecks; independent package test PASS. |
| R03.2 | Архивная идентичность отдельная; полное чтение архива и старые resources работают | PASS | Archive capability separate from typed message IDs; issued archive URI full paging and resource compatibility; actual legacy group encoder URI regression PASS. |
| R03.3 | Пустой/граничный текст, неверные/истёкшие/подменённые курсоры проверены | PASS | contentread tests: empty/exact scalar limit, tamper, foreign identity/URI, expiry/restart; safe explicit errors. |
| R03.4 | Policy/retention/recall между страницами; нет upstream writes/media fetch | PASS | Existing owner used per page; policy/identity/recall/retention read path; SDK recall test and no native/media commands. |
| R04.1 | Discovery отличает deferred tools/unsupported host/отсутствующее подключение | PASS | Both service-access references distinguish visible/deferred/unsupported host/explicit missing connection/access denial without invented discovery API. |
| R04.2 | Typed context назван точно; правильный unavailable fallback | PASS | Events entrypoint and workflow name exact typed context; direct ID never fed to group compatibility; unavailable context stays unknown. |
| R05.1 | README/spec/skills/system reference соответствуют принятым возможностям | PASS | README/spec/skills/reference/client/deployment/recovery/contracts inspected; current capabilities and operational host NOT_RUN distinction explicit. |
| R05.2 | Исторические ограничения/отчёты не превращены в новые доказательства | PASS | Historical native/eval reports kept scoped; revised skills not declared validated by historical model evals. |
| R06.1 | Прореживание refs поддерживает транзитивность/циклы/escaping | PASS | Reachability graph follows transitive local pointers/cycles/escaping; unsupported scopes fail closed; publication graph tests. |
| R06.2 | Результаты проверки валидных/невалидных fixtures эквивалентны | PASS | Independent TestPrunedSchemasPreserveRealSDKRequestsAndNontrivialResponses11/11: valid real SDK projections and invalid required/nested/enum/format variants against both validators. |
| R06.3 | Manifest/baselines синхронны; размер каталога до/после зафиксирован | PASS | Published manifest/baselines and generated reference match; publication test reports byte reduction; independent generator/SDK fixture tests PASS; exact-c675 CI39115464885 completedSUCCESS bothOS. |
| R07.1 | Backend сообщает настоящие version/revision/dirty либо явный unknown | PASS | buildinfo safe version/revision/nullable dirty and SDK tests; independent installed HTTP c675 backend revision exact and dirtyfalse. |
| R07.2 | STDIO отличает build bridge и backend без приватных путей | PASS | Independent installed nativeSTDIO c675 bridge and serving backend both report exact clean revision; outputdescriptors equal HTTP/manifest; unknown legacy backend path covered by tests. |
| R07.3 | Версия протокола независима; identity проверена на установленной сборке | PASS | Independent installed binary SHA b9111ff797af83629e029cb26f09db4ddbb612066de3a574b499fb869c7e1ce7; Go metadata exactc675 dirtyfalse CGO0 darwinarm64. CoreHTTP+STDIO2025 and separateEvents2026 read probes PASS. |
| R08.1 | Journal причины из closed enum; старый gap_reason сохранён | PASS | include_diagnostics opt-in closed delivery reasons; legacy gap_reason and old response shape preserved; journal diagnostic tests. |
| R08.2 | Stable delivery reference переживает retry/restart; attempt ID различается | PASS | State-key HMAC stable reference; independent restart/retry tests distinct attempt correlations; no raw row ID disclosure. |
| R08.3 | Client correlation ID совпадает с server log | PASS | Preflight/dispatch failure correlation created before execution and returned in details; final_audit tests verify same local log ID. |
| R08.4 | Нет payload/секретов/адресов/native IDs и high-cardinality metric labels | PASS | Allowlisted logs/diagnostics inspected and tested; no payload/URL/credentials/native IDs; high-cardinality IDs not metric labels. |
| R09.1 | Ограниченный browse после пустого поиска при известном чате/периоде | PASS | Two literal searches then bounded known-chat/known-period browse3x20; no synonym/translation crawl or account dump. |
| R09.2 | Entrypoints маршрутизируют в references без потери trust/permission правил | PASS | Entrypoints route to references conditionally; trust, explicit permissions, stable UUID, unknown/partial and no auto-read/react retained. |
| R09.3 | Frontmatter/ссылки и safe walkthrough всех перечисленных сценариев проверены | PASS | Both quick_validate PASS; links/reference checks PASS; static safe workflow walkthroughs direct context, deferred tools, multilingual browse, blocked/resume, duplicate/unknown/rights/hostile text; synthetic resume independently race-tested. No model-quality claim. |
| R09.4 | Исходные/установленные skills совпадают; сохранённое задание обновлено поддерживаемым путём | NOT_RUN | Source/install skills independently byte-identical17files; saved real-host task migration to explicit readiness_mode=persisted unavailable/not performed. File synchronization alone cannot satisfy combinedcriterion. |
| R10.1 | Legacy search/context описаны как group-only compatibility | PASS | Live27tool descriptors exactly match manifest; legacy search/context identify group-only compatibility and typed alternatives. |
| R10.2 | Typed предпочтительный путь виден; metadata tools и legacy consumers сохранены | PASS | Typed route documented; live group metadata remains separate; old consumers and resources retained with regression tests. |
| R11.1 | Генератор/верификатор живёт в repo, не зависит от/tmp или private runtime | PASS | scripts/reference workflow lives in repository; no private runtime/network/tmp dependency; independent checker PASS. |
| R11.2 | CI проверяет каталог/описания/поля/resources/Events/reactions/CLI/ссылки | PASS | Exactc675 CI39115464885 bothOS generatedreference and5negativecontrols steps success; independent checker27tools/5resources/4templates/6Events/54reactions/20CLI/118links PASS. |
| R11.3 | Архитектурная проза сверена ревью отдельно от генерации | PASS | Architecture prose read independently, stale counts fixed; checked ownership/transport/storage/permissions/callback-vs-processing guarantees against source. |
| R12.1 | Корневые test/race/vet/lint/architecture и обязательные existing offline проверки PASS | PASS | Independent GitHub query exactc675 CI39115464885 completedSUCCESS bothOS: root test/race/vet, lint, architecture100%, mandatory offline and tooling. Local frozen offline50/50cases480/480assertions. No newmodel/liveevals. |
| R12.2 | Вложенныйzcago и tooling: обязательные CI проверки PASS | PASS | Same exact CI: nestedzcago test/race/vet plus tooling mandatory steps independently confirmed success on bothOS. |
| R12.3 | CGO0 macOSarm64/Linuxamd64 и native smoke PASS | PASS | Same exact CI CGO0darwinarm64/darwinamd64/linuxamd64 crossbuilds and native startup cleanup PASS; installed CGO0arm64 metadata independently checked. |
| R12.4 | Публичная точная ревизия: подписанный commit/push и зелёныйCI | PASS | Public signedc675 GitHub verifiedtrue/reasonvalid; exacthead run39115464885 statuscompleted conclusionsuccess independently queried, bothmacOS/Ubuntu jobs and allrequiredsteps success. |
| R12.5 | Проверенный приватный backup/rollback; сохранены данные/config/subscriptions/checkpoints | PASS | Independent stoppedbackup17filehash verification; liveSQLite integrityOK/schema20/all12protectedtable priorrowhashes retained; config/plisthashes unchanged;18savedops, grantsclosed,nocompanion. Recovery-aware rollback guard previously independently tested. |
| R12.6 | Установленный backend: revision/readiness/safeHTTP+STDIO чтение PASS | PASS | Independent currentinstalled HTTP+nativeSTDIO27descriptors exactly matchmanifest; backend/bridge cleanexactc675; authenticated/connected119messages;safeconversation/resource reads; separateEvents2026list6profiles; persistedfield advertised. No ack/subscription/nativewrites. |
| R12.7 | Независимый reviewer1:100% критериев,0 нерешённых ошибок | NOT_RUN | No independent reviewerA100%fullscope verdict assumed; hosttask criterion remainsunproven. |
| R12.8 | Независимый reviewer2:100% критериев,0 нерешённых ошибок | NOT_RUN | This B verdict40/43=93.02%, not100%; outstanding savedhosttask prevents fullacceptance. |

## Результаты проверок

Итоговые доказательства опубликованной и установленной ревизии приведены ниже и в двух независимых отчётах. Последующие разделы разработки сохраняют исторический контекст и не заменяют текущий verdict. Новые модельные evals не запускались.

## Решения и открытые зависимости

- R01: основная гонка устранена явным `readiness_mode=persisted`: сохранённая запись доступна независимо от callback receipt, ack меняет только processing checkpoint. Без companion обе гонки и ранний ack/retry/restart проверены actualSDK/TLS тестами. Companion остаётся отдельно проверенной на синтетическом адаптере страховкой отсутствующего/прерванного запуска; в production он не создавался. Каталог и saved task целевого host ещё требуют поддерживаемого обновления (R09.4).
- R02: coverage_mode=full по умолчанию сохраняет прежние поля; compact является явным opt-in.
- R07: build identity добавляется в MCP _meta, не изменяя сохранённые operation outcomes.
- R08: diagnostics journal являются opt-in include_diagnostics; reasons закрыты, delivery reference keyed и не содержит native identity.

## Финальные независимые verdict

Reviewer1:40/43 (93,02%),0выявленных нерешённых ошибок, NOT ACCEPTED. [Отчёт A](reviews/mcp-skills-hardening-release-c67567c-a.md).

Reviewer2:40/43 (93,02%),0выявленных нерешённых ошибок, NOT ACCEPTED. [Отчёт B](reviews/mcp-skills-hardening-release-c67567c-b.md).

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

## Выпущенная промежуточная ревизия 715faf5e

[Коммит](https://github.com/skosovsky/zl-mcp/commit/715faf5ec23dd9b35059584d90fba11fd16ea3b6)
подписан (GitHub verified=true, reason=valid).
[CI39112765094](https://github.com/skosovsky/zl-mcp/actions/runs/39112765094)
завершён успешно на macOS и Ubuntu. Чистый CGO0 Go-бинарник установлен тем же
LaunchAgent после проверенного полного приватного backup state/config/plist/
предыдущего бинарника. Config, plist и grants не менялись; companion не создан.

HTTP2026 и STDIO2025: 27 tools с точными descriptors, 6 Events через HTTP,
подтверждённые backend/bridge revision и dirty=false. Collector authenticated/
connected, 119 сохранённых сообщений и 18 результатов message actions; все
исходные строки 12 защищённых таблиц и checkpoints сохранены. Schema20,
установленные skills 8+9 файлов совпадают с исходниками этой ревизии.

Временный runtime-check первоначально ошибочно проверял /readyz (этот маршрут
принадлежит tunnel-client) и смешивал stateless HTTP2026 с legacy initialize.
Скрипт исправлен по фактическим transport-контрактам; корректные read-only
проверки независимо повторены обоими рецензентами. Это ошибки проверки,
а не обнаруженные отказы backend. Секреты и переписка в отчёт не включены.

Независимые замороженные отчёты: [A](reviews/mcp-skills-hardening-release-715faf5-a.md)
(38/43,88,37%,0ошибок) и [B](reviews/mcp-skills-hardening-release-715faf5-b.md)
(39/43,90,70%,0ошибок), оба NOT ACCEPTED. Различие связано с трактовкой
операционного критерия продолжения; финальные gate не пройдены.

После выпуска пользователь уточнил причинное исправление R01: готовность
сохранённого события не должна зависеть от callback receipt. Новый
[контракт persisted readiness](contracts/journal-processing-readiness.md)
и формулировки четырёх R01-строк выше отражают это уточнение, сохраняя все43
критерия и отдельную приёмку crash recovery. В715faf5 этот режим ещё отсутствует;
старые отчёты не засчитывают его реализацию. Текущий этап — его реализация и
повторная приёмка; source/installed и release/runtime нужно сверить заново.

## Причинное исправление R01: локальная проверка кандидата

`readiness_mode=persisted` реализован отдельным совместимым представлением
существующего журнала. Без companion actual SDK/TLS тесты подтверждают чтение
и ordered ack до HTTP ответа и после 2xx до transport commit; pending/sending
не выдаются за delivered. Ранний ack не очищает outbox и не препятствует
retry после503/timeout и перезапуска с исходными ID/body. Старый default
сохраняет прежний wire contract и блокировку.

Полная schema и typed Event/Activity validation выполняются после owner/privacy
проверок, до бюджета страницы. Повреждённые вложенные данные, включая100КБ,
становятся bounded invalid_payload gap; ordered ack открывает следующую запись.
Валидный oversized v3 сохраняет RESPONSE_TOO_LARGE и исходные bytes, а не
превращается в gap. Оба независимых reviewers закрыли замечание о poison head.

Для интегрированного кандидата PASS: root test/race с -p2, vet, lint (0issues),
architecture (100%,14isolated+6source controls), CGO0darwinarm64/amd64 и
linuxamd64; существующий offline suite50/50cases,480/480assertions. Новые
модельные evals и live Zalo actions не выполнялись. Reference checker:
27tools,6Events,118local links;5generator tests; обе skill validation PASS.
Gitleaks8.24.3: no leaks found. Статический skill walkthrough:12сценариев.
Это локальная проверка кандидата; публикация, CI, установка и saved-host
workflow новой ревизии засчитываются только по отдельным доказательствам.

## Установленное причинное исправление c67567c5

[Подписанная ревизия](https://github.com/skosovsky/zl-mcp/commit/c67567c54408899bc729cf4afc488ba9eaa5217c),
[CI39115464885](https://github.com/skosovsky/zl-mcp/actions/runs/39115464885):
macOS и Ubuntu success. Чистый CGO0 бинарник установлен через прежний LaunchAgent
после остановки и проверенного полного приватного backup. Установленный SHA256:
`b9111ff797af83629e029cb26f09db4ddbb612066de3a574b499fb869c7e1ce7`.

Независимо подтверждены HTTP2026/STDIO2025: точные27tools,6Events,
backend/bridge revision/dirty=false и опубликованный readiness_mode=persisted.
Collector connected/authenticated;119сообщений,18сохранённых операций;
SQLite integrity/schema20; исходные строки12защищённых таблиц, подписки и
checkpoints сохранены. Config/plist не менялись, grants закрыты, companion
не создавался. Все17файлов установленных skills совпадают с исходниками.

Единственный содержательный открытый критерий R09.4: актуальный каталог и
сохранённое задание реального host. Загруженный в этом чате connector всё ещё
объявляет старый journal input без readiness_mode; это не доказывает состояние
отдельного запуска Ann. Поддерживаемых automation-update/refresh инструментов
здесь нет; private host storage/API не использовались.

Для завершения через поддерживаемый интерфейс host: обновить discovery;
подтвердить readiness_mode в фактической схеме; в существующем задании явно
указать persisted для чтения того же журнала. Сохранить подписку, scope,
направление, правила уведомления и запрет действий в Zalo. Перечитать сохранённое
задание и подтвердить режим. Новая подписка или новое Zalo-сообщение для этой
проверки конфигурации не нужны. После доказательства R09.4 оба рецензента
повторяют финальный verdict; до этого R12.7/R12.8 остаются NOT_RUN.
