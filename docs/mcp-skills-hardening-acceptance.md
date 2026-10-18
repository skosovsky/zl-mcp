# Приёмка: надёжность MCP и skills

**Принято: 43/43 критерия (100%), 0 выявленных нерешённых ошибок.**
Оба независимых reviewers подтвердили результат на одной принятой версии продукта.

Принятая и установленная ревизия сервера: `fab50b013a0c648e4b4884cacf324c5a8c6dfa59`.
[Подписанный коммит](https://github.com/skosovsky/zl-mcp/commit/fab50b013a0c648e4b4884cacf324c5a8c6dfa59);
[CI39117306119](https://github.com/skosovsky/zl-mcp/actions/runs/39117306119) — macOS и Ubuntu success.
Дата проверки: 2026-10-18. Основание: [R01–R12](task-mcp-skills-hardening.md).

## Полная матрица

Все исходные 43 критерия сохранены. PASS означает доказательство всей строки,
а не только одного теста. Полные независимые доказательства:
[отчёт A](reviews/mcp-skills-hardening-release-fab50b01-a.md) и
[отчёт B](reviews/mcp-skills-hardening-release-fab50b01-b.md).

| Критерий | Требование | Статус | Проверенное доказательство |
| --- | --- | --- | --- |
| R01.1 | Контракт persisted/default; независимые ready/transport/ack и совместимость зафиксированы | PASS | The processing-readiness contract and executable schemas separate retained readiness, true transport state and processing ack; explicit persisted mode preserves the old delivery-terminal default. |
| R01.2 | До2xx/между2xxиcommit: read+ack без companion, честный pending/sending и без нового сообщения | PASS | Actual HTTP MCP SDK and signed TLS callback tests read and ack before 2xx and between 2xx and delivery commit, without a companion or another message. Sending remains sending until the worker commits receipt. |
| R01.3 | Early ack, retry/restart сохраняют прогресс; отдельно проверено продолжение потерянного/прерванного запуска | PASS | Early ack survives 503, timeout, database/service/SDK restart and retry with identical callback bytes and IDs. Separate durable companion tests cover a missing or interrupted run without pretending it is the primary race fix. |
| R01.4 | Ошибка/отмена/gap/pruning сохраняют checkpoint/leases; нет busy loop, пропуска ack или ложного delivered | PASS | Ordered, generation-scoped receipts, cancellation, policy/retention/recall checks and unfinished outbox bytes remain protected. Poisoned entries become explicit receipted gaps before budgeting; valid oversized entries stay bounded errors without false ack. |
| R02.1 | Компактный режим явный; старые запросы сохраняют полное представление | PASS | Full coverage remains the default with its original complete representation; compact mode is explicit and reports exact snapshot count, truncation and continuation. |
| R02.2 | Anchor/search/browse доступны при800+gaps в бюджете ответа | PASS | SDK budget tests cover anchor, search and browse with at least 800 gaps. Compact replies remain bounded while preserving usable message reads. |
| R02.3 | Все gaps читаются по snapshot; append/update/delete/expiry определены | PASS | Coverage continuation uses signed typed identity, insertion high-water mark, row digest and 24-hour lifetime. Appends are excluded; mutation, deletion and expiry are explicit, not silently mixed snapshots. |
| R02.4 | Пагинация corpus browse не теряет строки; current policy rechecked | PASS | Browse reduction retains its original insertion snapshot and advances only past returned rows. Storage and SDK tests verify no missing or duplicate rows and renewed policy checks. |
| R03.1 | Tools-only читает полный Unicode-текст корпуса ограниченными страницами | PASS | Tools-only SDK tests reconstruct complete retained Unicode text across scalar-aligned pages, including escaped wire payloads and the duplicated MCP text representation. |
| R03.2 | Архивная идентичность отдельная; полное чтение архива и старые resources работают | PASS | Archive capabilities remain distinct from message IDs. Archive paging and old resources work; the real legacy event encoder URI is supported without confusing a colliding direct-chat identity. |
| R03.3 | Пустой/граничный текст, неверные/истёкшие/подменённые курсоры проверены | PASS | Empty and boundary-sized text, invalid targets, tampered or foreign cursors, expiry, process restart and changed content are tested with explicit safe failures. |
| R03.4 | Policy/retention/recall между страницами; нет upstream writes/media fetch | PASS | Each page re-enters the existing authorized owner; policy, retention, recall and archive capability checks remain current. No upstream writes or media fetching are introduced. |
| R04.1 | Discovery отличает deferred tools/unsupported host/отсутствующее подключение | PASS | Both skills distinguish visible tools, deferred discovery, unsupported host facilities, explicit absence of a connection and permission denial, without inventing a universal discovery API. |
| R04.2 | Typed context назван точно; правильный unavailable fallback | PASS | The exact typed context tool is named for conversation events. Direct IDs are never passed to group-only compatibility tools; unavailable context remains an explicit limitation. |
| R05.1 | README/spec/skills/system reference соответствуют принятым возможностям | PASS | README, technical specification, contracts, system reference, connection/deployment guides and skill documentation describe the implemented paths and preserve operational limits. |
| R05.2 | Исторические ограничения/отчёты не превращены в новые доказательства | PASS | Historical native and model-evaluation reports keep their original boundaries. Updated skills are not declared validated by older model runs. |
| R06.1 | Прореживание refs поддерживает транзитивность/циклы/escaping | PASS | Schema publication follows transitive local references, cycles and escaped JSON Pointers; unsupported reference scopes retain the original document rather than changing validation semantics. |
| R06.2 | Результаты проверки валидных/невалидных fixtures эквивалентны | PASS | All 11 pruned output schemas have real SDK request/response fixtures validated before and after pruning. Required-field, nested, enum and format mutations are rejected by both validators. |
| R06.3 | Manifest/baselines синхронны; размер каталога до/после зафиксирован | PASS | Manifest, migration snapshots and generated reference match the executable contracts. Publication tests record the removed schema bytes and verify determinism and caller ownership. |
| R07.1 | Backend сообщает настоящие version/revision/dirty либо явный unknown | PASS | Safe build identity and SDK tests pass. Independently observed installed backend revision fab50b013a0c648e4b4884cacf324c5a8c6dfa59 with dirty=false; unknown metadata remains explicit rather than invented. |
| R07.2 | STDIO отличает build bridge и backend без приватных путей | PASS | Independent native STDIO bridge and backend metadata both report the exact clean fab50b release. HTTP and STDIO descriptors match; the legacy unknown-backend case remains covered by tests. |
| R07.3 | Версия протокола независима; identity проверена на установленной сборке | PASS | Installed SHA256 7efb9d2d40e458a6c5a9d8e4148968b26abafecf46d11002cbddb345ee3b24f8; embedded revision fab50b, dirty=false, CGO=0, darwin/arm64. Core HTTP/STDIO and pinned Events protocol probes are independently successful. |
| R08.1 | Journal причины из closed enum; старый gap_reason сохранён | PASS | Delivery diagnostics are opt-in with a closed reason enumeration. Existing gap reasons and legacy response keys remain unchanged. |
| R08.2 | Stable delivery reference переживает retry/restart; attempt ID различается | PASS | A keyed stable delivery reference links retries and restart to the journal, while individual attempt correlation IDs remain distinct. No native or raw storage identity is disclosed. |
| R08.3 | Client correlation ID совпадает с server log | PASS | Correlation IDs are created before preflight/dispatch and the same ID appears in client failure details and the corresponding safe local audit log. |
| R08.4 | Нет payload/секретов/адресов/native IDs и high-cardinality metric labels | PASS | Allowlisted telemetry and diagnostic tests exclude content, credentials, callback addresses, raw errors and native IDs. Unique diagnostic IDs are not metric labels. |
| R09.1 | Ограниченный browse после пустого поиска при известном чате/периоде | PASS | Empty literal search may fall back to bounded browsing of an already known chat and period, without translation/synonym crawling or an account dump. |
| R09.2 | Entrypoints маршрутизируют в references без потери trust/permission правил | PASS | Skill entrypoints route conditionally to references while retaining trust boundaries, explicit action authorization, stable request IDs and unknown/partial handling. |
| R09.3 | Frontmatter/ссылки и safe walkthrough всех перечисленных сценариев проверены | PASS | Frontmatter, links and dependency names validate. Static walkthroughs cover deferred discovery, typed context, multilingual empty search, bounded browse, recovery, duplicates, permission failure and hostile content. Actual SDK scenarios verify the stateful parts without model evals. |
| R09.4 | Исходные/установленные skills совпадают; миграция сохранённого workflow документированного SDK-адаптера проверена через discovery/save/readback/callback с сохранением правила и прав | PASS | All 17 source/installed skill files independently match after final cutover. Exact-fab saved SDK workflow migration and old-catalogue negative control passed under race: one-field migration, preserved rule/permissions, readback, callback execution and restart. This follows the recorded user clarification, without claiming an Ann task was changed. |
| R10.1 | Legacy search/context описаны как group-only compatibility | PASS | Legacy search/context descriptions explicitly identify group-only compatibility and name the preferred typed alternatives. |
| R10.2 | Typed предпочтительный путь виден; metadata tools и legacy consumers сохранены | PASS | Typed reads are the preferred route; live group metadata stays a separate source, and existing legacy consumers/resources remain supported. |
| R11.1 | Генератор/верификатор живёт в repo, не зависит от/tmp или private runtime | PASS | The reference generator/checker is committed in the repository and needs no private state, network, temporary helper or running service. |
| R11.2 | CI проверяет каталог/описания/поля/resources/Events/reactions/CLI/ссылки | PASS | Exact-fab CI run 39117306119 independently reports the reference check and five negative controls successful on macOS and Ubuntu. Local checks confirm 27 tools, 5 resources, 4 templates, 6 Events, 54 reactions, 20 CLI commands and 118 links. |
| R11.3 | Архитектурная проза сверена ревью отдельно от генерации | PASS | Architectural prose was reviewed against ownership, transports, state, permissions and delivery/processing semantics separately from generated catalogue matching. |
| R12.1 | Корневые test/race/vet/lint/architecture и обязательные existing offline проверки PASS | PASS | Exact-fab CI run 39117306119: root test/race/vet, lint, architecture controls and mandatory offline profile all successful on both operating systems. Offline evidence is 50 cases/480 assertions; no new model or live evals were run. |
| R12.2 | Вложенныйzcago и tooling: обязательные CI проверки PASS | PASS | The same exact-revision CI independently confirms nested zcago test/race/vet and architecture tooling test/race/vet successful on both operating systems. |
| R12.3 | CGO0 macOSarm64/Linuxamd64 и native smoke PASS | PASS | The exact-revision CI confirms CGO-disabled darwin/arm64, darwin/amd64 and linux/amd64 builds plus native service startup cleanup. Installed metadata independently confirms the clean native CGO-disabled build. |
| R12.4 | Публичная точная ревизия: подписанный commit/push и зелёныйCI | PASS | Public fab50b commit signature independently verified true/reason valid. CI 39117306119 has the exact head SHA and completed successfully with all required macOS/Ubuntu steps successful. |
| R12.5 | Проверенный приватный backup/rollback; сохранены данные/config/subscriptions/checkpoints | PASS | Independent verification of 17 stopped-backup file hashes and current SQLite integrity; all 12 protected table baseline rows preserved, schema20, 18 saved operations, config/plist hashes unchanged, write grants closed. The authorized inactive baseline is preserved: active business/control subscriptions and pending/sending rows in both queues are all zero. |
| R12.6 | Установленный backend: revision/readiness/safeHTTP+STDIO чтение PASS | PASS | Independent installed HTTP/native STDIO calls verify exact clean fab50b backend/bridge, 27 manifest-identical descriptors, authenticated/connected status, 119 messages and safe conversation/resource reads. Events discovery returns six profiles; persisted is advertised; subscription listing is empty. |
| R12.7 | Независимый reviewer1:100% критериев,0 нерешённых ошибок | PASS | After B froze its own independent findings, B inspected A's completed exact-fab 43-row report: all 41 substantive criteria independently PASS, zero unresolved errors, and no remaining product or deployment action. Both completed audit processes satisfy the mutual final-recording gate. |
| R12.8 | Независимый reviewer2:100% критериев,0 нерешённых ошибок | PASS | B completed an independent audit of every one of the fixed 43 criteria, verified exact public/installed fab plus CI and preserved state, and independently confirmed the completed peer review. Final B acceptance is 43/43, 100%, zero unresolved errors. |

## Установленный сервис и сохранность

Установлен чистый native Go-бинарник: CGO_ENABLED=0, darwin/arm64,
vcs.revision соответствует принятому коммиту, vcs.modified=false. SHA-256:
`7efb9d2d40e458a6c5a9d8e4148968b26abafecf46d11002cbddb345ee3b24f8`.

Существующий LaunchAgent запускает Go напрямую; config, plist и пути runtime
не менялись. Перед заменой сервис остановлен, взят account lock, сохранены
полный state, прежний бинарник, config и plist в приватном каталоге вне repo.
Копия проверена по SHA всех 17 файлов, SQLite integrity и снимкам 43 таблиц.
Оба reviewers независимо перепроверили backup. Все исходные строки 12 защищённых
таблиц, message actions и processing checkpoints сохранены после запуска.
Схема SQLite20, 119 сообщений и 18 результатов message actions. Collector
authenticated/connected. Send/read-receipt/reaction grants закрыты.

Через HTTP2026 и native STDIO2025 проверены точные 27 tool descriptors,
resources, безопасное чтение и независимые build identity backend/bridge.
HTTP публикует 6 Events и explicit readiness_mode=persisted. Сами HTTP200,
наличие схемы и доступность STDIO не объявляются доказательством обработки
события внешним host. Все 17 файлов двух установленных skills совпадают
с исходниками; приватные копии прежних skills проверены до замены.

По отдельному прямому поручению пользователя все рабочие/тестовые подписки
отменены через events/unsubscribe. Read-only SQLite и MCP list подтверждают
0 активных business/control подписок и 0 pending/sending доставок в обеих
очередях. Отмена сохранила сообщения, operation outcomes и checkpoints.
Итоговый rollout сохраняет именно эту согласованную неактивную baseline;
подписки для приёмки не восстанавливались.

## Что проверено

- Root test/race/vet/lint; pinned zcago и tooling test/race/vet; architecture
  compliance/coverage100%, 14 isolated и 6 source negative controls.
- CGO0 сборки darwin/arm64, darwin/amd64 и linux/amd64; native macOS smoke.
- Существующий mandatory offline suite: 50/50 cases, 480/480 assertions.
  Model и live profiles не запускались; новые модельные evals не создавались.
- Исполняемые схемы, manifest и actual SDK fixtures: старый default совместим,
  persisted readiness не зависит от callback receipt, ранний ack не меняет
  delivery state/lease/bytes. Проверены обе гонки, retry, timeout, restart,
  cancellation, retention, policy и ordered checkpoints.
- Poisoned envelope, включая100КБ nested data, становится receipted gap до
  page budget; ordered ack открывает следующую запись. Валидный oversized v3
  остаётся RESPONSE_TOO_LARGE, не превращается в ложный gap/ack.
- Tools-only Unicode content paging, отдельная архивная идентичность,
  продолжение coverage800+gaps, bounded browse и current-policy проверки.
- Reference checker: 27 tools, 5 resources, 4 templates, 6 Events, 54 реакции,
  20 CLI, 118 локальных ссылок; 5 generator negative-control tests.
- Source/installed skill validators и 12 безопасных статических walkthrough
  маршрутов. Это структурная проверка инструкций, не модельный eval.
- Сканирование публикуемого состава gitleaks8.24.3; реальные credentials,
  приватная переписка, native account/subscription IDs в отчёт не включались.

Текущий compact JSON размер input/output schemas: 136877 → 98997 байт,
минус37880 байт в11output schemas. Это измерение JSON, не токенов или качества
модели. Непрореженные контракты остаются исполняемым источником истины.

Воспроизводимые проверки из репозитория:

```sh
make check
python3 scripts/reference/reference.py --check
python3 -m unittest discover -s scripts/reference -p 'test_*.py'
go test -race ./internal/mcpserver -run '^TestSyntheticClient(SavedWorkflow|WorkflowMigration)' -count=1
```

`make check` выполняет существующий offline suite без модели и live-действий.
Локальные приватные backup/runtime-proof файлы — свидетельства установленного
Mac, не обязательные зависимости репозиторных проверок.

## Уточнение клиента и границы доказательства

Пользователь уточнил: Ann — пример клиента, а не обязательный host приёмки.
Первоначальный Ann-only gate был нашей избыточной трактовкой. Все43критерия
сохранены; task§5.3 допускает документированный MCP-адаптер.

Проверен generic SDK-клиент, владеющий временным сохранённым workflow:
actual discovery → изменение только readiness_mode → save/readback →
подписанный TLS callback → чтение и ordered ack до2xx → перезапуск и повторное
чтение без дублирования. Сохраняются точные subscription/generation, scope,
направление, фильтры, правило, канал, язык и запрет действий в Zalo. Без
advertised capability сохранённый файл остаётся побайтно прежним. Runner
использует сохранённые аргументы, а не захардкоженный новый mode.

Это доказательство сервера и документированного адаптера, не обновления
конкретной Ann/ChatGPT automation, наличия payload в её модельном контексте
или улучшения поведения модели. Для внешнего клиента остаются инструкции
поддерживаемого discovery и миграции сохранённого задания в
[контракте readiness](contracts/journal-processing-readiness.md) и
[automation rule](../skills/handling-zalo-message-events/reference/automation-rule.md).

Legacy/default delivery_terminal сохранён. Для устранения гонки клиент явно
выбирает persisted; новая business-подписка не нужна. Опциональный companion
проверен отдельно как страховка потерянного/прерванного запуска, в установленном
сервисе не активирован. Callback acceptance, processing ack и Zalo read receipt
остаются разными фактами. Внешнее уведомление и ack не атомарны; exactly-once
не обещается. Политики, сроки хранения, budget errors и явные gaps сохранены;
утраченное событие не восстанавливается под старым ID из текущего корпуса.

## Исправленный CI-дефект и история

Предыдущий документационный run39116337542 выявил совпадение 5-секундного
ожидания теста с production shutdown budget5с, после которого тест ошибочно
начинал второго владельца account lock. Исправлен только тест: ожидание включает
bounded cleanup allowance10с; timeout прекращает сценарий через t.Fatal.
Production timeout не изменён. Пять локальных race-повторов и независимые
проверки обоих reviewers PASS; итоговый CI принятой версии PASS на обеих ОС.

Замороженные промежуточные отчёты сохраняют свои прежние verdict:
[715faf5 A](reviews/mcp-skills-hardening-release-715faf5-a.md),
[715faf5 B](reviews/mcp-skills-hardening-release-715faf5-b.md),
[c67567c A](reviews/mcp-skills-hardening-release-c67567c-a.md),
[c67567c B](reviews/mcp-skills-hardening-release-c67567c-b.md).
Их ограничения не превращены задним числом в новые доказательства.

Финальные отчёты относятся к версии продукта fab50b01. Последующий отчётный
коммит меняет только Markdown; исполняемые Go-входы, JSON-контракты и skills
не меняются, принятый установленный бинарник остаётся тем же.
