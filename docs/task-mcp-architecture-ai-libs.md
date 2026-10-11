# ТЗ: архитектура zl-mcp и интеграция toolsy, metry, evaly

Дата: 2026-10-11. Статус: спецификация для последующего выполнения; реализация не начата.
Репозиторий: `github.com/skosovsky/zl-mcp`. Целевой путь: `docs/task-mcp-architecture-ai-libs.md`.
Владелец: Сергей Косовский. План: T01–T10, строго последовательно, одна задача — один коммит.

## 1. Цель и границы

Получить модульный Go MCP-сервис с общей точкой исполнения toolsy, наблюдаемостью metry,
воспроизводимой оценкой evaly и проверяемыми arch-go границами. Сохранить публичные
контракты и существующие гарантии Zalo: один владелец аккаунта, устойчивые операции,
неполнота истории, точная идентичность диалогов, отсутствие автоматической повторной отправки.

Обязательный результат — перенос всего текущего tool surface на общий executor,
отделение MCP-представления от application handlers и сохранение resources/Events/STDIO.
Первый вертикальный срез — поиск, контекст и чтение полного сообщения.

В эту работу НЕ входят: новые продуктовые инструменты; изменение прав send/join;
новые MCP profiles с сокращением существующего каталога; обязательный approval каждой отправки;
перенос send/join/history ledger на OperationProfile; миграция БД; новый event transport;
Tasks extension; code execution; RAG/LLM внутри сервера; библиотеки guardy/flowy/ragy/memy;
переписывание zcago; установка бинарника, рестарт LaunchAgent, реальные действия аккаунта,
публикация библиотек, push или release. Это отдельные спецификации, а не скрытые подзадачи.

Текущий сеанс создаёт только документы. Текст goal в разделе 12 предназначен для нового
запуска после команды пользователя на реализацию и не разрешает начать T01 сейчас.

## 2. Baseline и источники истины

Исходные точки: `internal/mcpserver/server.go`, `internal/mcpserver/http.go`,
`internal/service/bridge.go`, `internal/service`, `internal/messaging/manager.go`,
`internal/historyimport`, `internal/collector`, `internal/storage`, `internal/events`,
`internal/evalfixture`, `docs/contracts`, `docs/development.md`, `docs/evals/report.md`.

На дату исследования: zl-mcp объявляет Go 1.26.5, официальный MCP SDK v1.8.0,
modernc SQLite и локальный patched zcago. toolsy/metry/evaly требуют Go >=1.27.1.
Локальное состояние ai-libs не равно опубликованному релизу. T01 фиксирует HEAD и dirty
status всех используемых репозиториев; T02 проверяет доступность совместимых pinned версий.
Нельзя автоматически заменять нужную функциональность более старым опубликованным API.

До рефакторинга составить `docs/mcp-migration/baseline.md` и машинный manifest:
имя, description, schemas, annotations, transport, handler, permissions, effects,
resources, error shape, byte limits, normalization, timeout, operation owner.
Источники — `contracts.Names()`, фактический tools/list и регистрации resources/events;
список не фиксировать вручную по README. Учесть aliases history schemas.

| Surface | Что обязательно инвентаризировать и сохранить |
|---|---|
| Conversations/search/context | direct/group typed IDs, поиск, browse, групповые legacy tools, coverage, gaps, stale, pagination, source provenance |
| Archive reads | list_archive_sources, source_id, окна времени, archive_row_id vs zalo_message_id, TTL, tombstones, policy при каждом чтении, пустая страница с has_more |
| Resources | статические diagnostics/capabilities/collection; все templates сообщений и архивов; MIME, URI, auth, лимиты и ссылки из результатов |
| Messaging | send/status, request UUID, fingerprint, quote, permission, неизвестный результат, storage failures |
| Membership | inspect/approve/join/status; trusted local issuer, plan token, expiry, binding, recovery |
| History | start/status/cancel, worker, account/policy changes, checkpoints, отсутствие Events от импорта |
| Events | profiles v1/v2, subscriptions, read/ack, webhook queue, подписи, retry/cancel/retention; HTTP extension capability |
| Transports/CLI | HTTP auth/Host/Origin/headers, STDIO forwarding, version discovery, shutdown; owner-only archive/control CLI |

Исторические 65 eval runs не являются baseline новой версии. Исторические live gates
не переносятся в новый PASS. Реальные аккаунтные данные не копировать в fixture/evidence.

## 3. Целевая архитектура

Паттерны: modular monolith, ports/adapters, contract-first, capability-oriented handlers,
stateless MCP requests + durable domain operations, один владелец побочного эффекта,
bounded responses, отдельная оценка поведения агента. Не вводить универсальный CQRS bus,
DI framework, event sourcing или generic repository ради структуры.

```text
HTTP / STDIO bridge
  -> MCP wire adapter (SDK, schemas, annotations, projection)
  -> toolruntime (toolsy RegistryView, trusted identity, policy, executor)
  -> app/{conversations,search,messaging,membership,history,archives,eventaccess}
  -> потребительские ports -> adapters/{store,operations}
  -> существующие storage / managers / guarded Zalo session

resources -> wire adapter -> те же app read ports и access policy
events extension -> отдельный protocol adapter -> существующий events subsystem
service -> composition root, единственный владелец account/session/DB/workers/telemetry
eval harness -> реальный MCP-вход -> synthetic account + независимый effect recorder
```

### 3.1. Пакеты и зависимости

Названия новых каталогов нормативны; при обнаружении конфликта меняется сначала ADR.
Существующие domain types повторно использовать; SDK DTO не становятся business types.

| Пакет | Ответственность и разрешённые основные зависимости |
|---|---|
| `internal/domain` | идентичности, бизнес-значения, ошибки; без MCP/toolsy/metry/evaly/SQL/zcago |
| `internal/app/<capability>` | typed handlers, consumer-owned ports; domain и свои DTO; без SDK, SQL, concrete managers, ai-libs |
| `internal/adapters/store` | реализует app read ports через существующий storage; преобразует storage DTO в app/domain |
| `internal/adapters/operations` | реализует app command ports через существующие messaging/history/collector/control сервисы; не владеет вторым ledger |
| `internal/toolruntime` | registry/view/session, argument binding, toolsy policy, invocation; app/domain/contracts; без storage/zcago |
| `internal/mcpserver` | SDK registration, transport validation, tool annotations, result/error/resource projection; toolruntime, app/domain/contracts; без concrete storage/managers |
| `internal/observability` | metry/OTel, bounded attributes, safe outcome mapping; без SDK transport, SQL и zcago |
| `internal/service` | composition root/lifecycle; связывает конкретные компоненты и передаёт зависимости |
| `internal/evaluation`, `internal/evalfixture` | evaly driver, fixtures, independent evidence; production packages не импортируют их |
| `docs/contracts` | embedded executable schemas/alias map; не импортирует runtime |

У существующих collector/storage/events/mobilebackup/zalo остаются нынешние внутренние
связи, если они не обходят новые публичные boundaries. Это не полная переработка их internals.
Существующие archive/control orchestration в service можно оставить за narrow port;
необходимо убрать доступ MCP-слоя к конкретному service/store. Cross-capability общие типы
помещать в domain или небольшой явно описанный app package, а не в `utils`.

Порты задаёт потребитель; mocks и реализации соответствуют Go interfaces. `map[string]any`
допустим в существующем wire/legacy adapter, но не как универсальный новый app API.
Toolsy subject/scope — собственные типы host: local principal, account identity,
policy revision; не данные из arguments или `_meta`. MCP call ID, domain request UUID,
operation ID и trace ID — разные идентичности.

### 3.2. Владение состоянием

Service — единственный lifecycle owner основной messages.sqlite (connection pool допустим)
и одной guarded Zalo session; STDIO не создаёт collector. Отдельные bounded readonly handles
SQLite-архивов сохраняются и закрываются их владельцем; правило не запрещает archive reads.
Transport disconnect не уничтожает durable operation. Каждый background worker имеет
одного владельца запуска/отмены/join. Остановка: прекратить admission, завершить/cancel
workers в bounded deadline, записать допустимые результаты, завершить telemetry, закрыть DB.
Порядок уточнить по текущему контракту; shutdown не должен ждать exporter бесконечно.

Toolsy в этом ТЗ владеет проверкой/dispatch, а existing managers — operations и retry decisions.
Запрещено устанавливать OperationProfile/filejournal поверх действующих ledger, выдавать
внутренние approval grants вместо человека, включать автоматические retries/cache для writes.
Registry cache не должен превращать прежний доступ в право текущего чтения.

## 4. Контракт toolsy-интеграции

### 4.1. Порядок исполнения

1. HTTP/STDIO сохраняет текущий trust boundary и ограничения wire.
2. Host разрешает subject/scope из доверенного контекста и актуальной account/policy.
3. Проверяется существующий admission/rate limit; проверка не вызывается дважды.
4. Явный compatibility binder выполняет ТОЛЬКО текущие documented/characterized
   преобразования (`limit/before/after` и defaults). Raw input сохраняется только в памяти.
5. Вход проверяется executable schema; создаётся invocation-owned toolsy call/environment.
6. Scoped executor повторно применяет актуальную policy и вызывает ровно один app handler.
7. Результат проходит существующие domain projection, pagination/byte budget и output schema.
8. Wire adapter выдаёт прежний result/error и safe telemetry; delivery failure не повторяет effect.

Rate limits и deadlines сохраняют прежнюю семантику, включая отдельные ограничения resources
и Events. Cancellation наследуется; завершение записи подтверждённого send outcome сохраняет
существующий ограниченный служебный context после отмены клиента.

### 4.2. SDK и схемы

Для первого внедрения оставить `sdk.Server.AddTool` и направить handler через
`toolsy.RegistryView.Execute`. `NewProxyTool` позволяет принять существующую raw schema;
`WithOutputSchema` задаёт контракт успешного результата. Конкретные API сверить с pin T02.
Не вводить MCP dependency в domain/app. SDK resource registrations остаются native.

`toolsy/mcp.Export` не является обязательной зависимостью этой миграции: его текущая
стандартная проекция не сохраняет все Zalo annotations/content/error semantics.
Локальный adapter не реализует собственный MCP transport/parser поверх SDK.
Уже существующая Events protocol обвязка сохраняется и изолируется.

Схемы из `docs/contracts` — единственный authority. Текущая проверка AssertFormat (UUID,
RFC3339 и другие используемые formats) обязательна даже если generic toolsy validator
обрабатывает format как annotation. До доказательства parity оставить строгую host-validation;
две проверки допустимы временно, ослабление — нет. Alias, $ref, additionalProperties,
missing/null, explicit zero, bounds и normalization проверяются contract fixtures.

Обязательны точные IDs >2^53 как opaque strings, JSON-RPC string/numeric IDs, null/missing,
Unicode/UTF-8, malformed input, границы размеров. Уже известное ограничение SDK фиксируется
отдельно; нельзя объявлять точность гарантированной только по типам Go. Новый регресс запрещён.

### 4.3. Wire parity

Сохраняются names, descriptions, input/output schemas, annotations, deterministic order,
`structuredContent`, text content, ResourceLink, MIME, `isError`, error code/next_action,
coverage и partial semantics. Сравнивать canonical JSON по полям; динамические timestamps,
trace IDs и opaque cursors сравнивать по контракту, а не побайтово. Секретные grants/effects
не сериализовать. Не возвращать generic `tool execution failed` вместо domain error.

Tool input/domain failures остаются tool errors там, где таков baseline; protocol failures
остаются JSON-RPC errors. Существующие error schemas проверяются явно и не считаются
автоматически проверенными success OutputSchema. Не восстанавливать code из произвольного
error string. Оригинальная cause доступна внутренне, наружу — только разрешённый projection.

Лимит tool response сохраняется на уровне всего сериализованного SDK результата, а не
только JSON business payload (на момент исследования 64 KiB). Resource/request limits
фиксируются T01. Уменьшение страницы не теряет cursor/coverage; пустая archive page может
иметь продолжение. Нельзя повторно вызвать write handler ради подгонки ответа под budget.

## 5. Сохранение domain operations и данных

| Владелец | Обязательные инварианты |
|---|---|
| Messaging manager + SQLite | fingerprint recipient/text/reply; pending можно продолжить прежними аргументами; claim перед upstream; sending после crash -> unknown; terminal/unknown без resend; mismatch -> conflict |
| Join manager + approval store | точные preview/group/issuer/expiry/binding; approval выдаёт только trusted local channel; inspect/status не join; uncertainty не разрешает новый join |
| History manager/worker | stable request identity, durable limits/checkpoint, auth pause, cancel terminal, no late-page commit после cancel; imported history без новых Events |
| Events subsystem | существующие атомарные границы message/eligibility/queue; profile v1/v2, dedup, durable delivery/ACK; рестарт не меняет payload |
| Archive subsystem | scope/account, source visibility, TTL/tombstones/retention, отдельные archive identities; archived rows не quote anchors |

Send permission остаётся `allow_send`/recipient policy + trusted host instruction;
join остаётся с локальным approval. Registry permission не подменяет ни одно из них.
`sent` означает acceptance upstream, не доставку/прочтение. Не заявлять exactly-once.
Запись request identities не сокращается и не превращается в TTL-cache. Ledger не получает
скрытую копию текста сообщения/quote. Повторный read/status проверяет текущую policy.

ТЗ не меняет DB schema, существующие cursors, grants или сериализованные queue records.
Rollback — предыдущий совместимый бинарник на том же synthetic state после остановки
нового; тестировать restart в обе стороны. Если реальная необходимость изменения persistence
выявлена, остановить соответствующую задачу и предложить отдельную schema migration с backup,
restore и проверкой mixed-version behavior. Не выполнять автоматический down migration.

## 6. Контракт metry

Один provider принадлежит service. Использовать `metry.New` для owned SDK либо `FromSDK`
для borrowed SDK с документированным владельцем shutdown; выбрать один режим в composition.
По умолчанию экспорт выключен; не создавать обязательного внешнего сервиса. Конфигурация
exporter opt-in, validated, с bounded queue/timeout; API keys/headers не логируются.

`metry.ExecutorWrap` оборачивает app execution; HTTP instrumentation не создаёт дубликат
того же execution span. Его встроенный status описывает Go-вызов (success/error/panic),
а не бизнес-результат; outcome-classifier в этом API не предполагается. Host observability
adapter дополнительно классифицирует typed Result и записывает отдельный bounded
`zl_mcp_domain_calls` counter с tool/outcome и domain outcome в host span. Не подменять
nil Go error фиктивной ошибкой ради метрик. Технический success и domain unknown могут
сосуществовать; dashboard/eval business-success считает только по доменному сигналу.
Получить связь request -> operation -> worker, сохраняя различие IDs.
Opaque trace references в памяти допустимы; persistence ради трассировки не меняется.
Возврат business error внутри Result должен дать outcome=error, даже при nil Go error;
partial/unknown/cancelled имеют отдельные значения. Mapping typed, не парсинг текста.

| Сигнал | Единицы и допустимые dimensions |
|---|---|
| executor calls/duration | calls, seconds; стабильные operation/tool name, технический status библиотеки |
| `zl_mcp_domain_calls` | calls; tool, доменный outcome из finite enum |
| `zl_mcp_response_bytes` | bytes histogram; tool, transport |
| `zl_mcp_upstream_calls` | counter; bounded operation kind, outcome |
| `zl_mcp_operation_transitions` | counter; kind, from/to из finite enums |
| `zl_mcp_worker_queue_depth` | gauge; queue name |
| `zl_mcp_operation_age_seconds` | histogram/gauge с выбранной семантикой в ADR; kind/state |

Не добавлять account/chat/message/request IDs, query/text, raw URI/token/error в metric labels.
В logs/spans допускаются только перечисленные safe fields и случайные correlation IDs;
тексты, quote, sessions, raw upstream causes запрещены. Политику bounded fields тестировать
sentinel secrets, в том числе через panic/formatting. Telemetry failure не меняет domain result,
не блокирует send и не запускает retry. Экспортирование проверять in-memory test exporter;
недоступность exporter — отдельный failure-injection тест.

## 7. Контракт evaly и измерений

Создать `cmd/zl-mcp-eval`, `internal/evaluation` и versioned synthetic dataset. evaly core
используется как typed target/lifecycle/capture/graders; допустимы `Run`/`RunPaired`, `Compare`
после проверки API. Host владеет запуском клиента/модели; evaly сам не является LLM-agent runtime.
Не импортировать чужие `internal/fixtures` из примеров библиотеки.

Два профиля: `offline` (обязательный CI без credentials и внешней сети; loopback HTTP
для synthetic fixture разрешён) и `model` (явный opt-in запуск,
синтетическая среда, отдельный бюджет). Пропущенный model-run отмечается NOT_RUN, не PASS.
Baseline binary из T01 и candidate работают на независимых копиях одной fixture. Lifecycle
каждого case создаёт/закрывает state и фиксирует эффект независимо от ответа агента.

Context deadline в callback — cooperative cancellation, не hard timeout. Harness запускает
case/agent subprocess с внешним wall-clock watchdog, bounded stdout/stderr и гарантированным
termination/Wait/cleanup дочерних процессов в finally-пути. Начальные budgets: 60 секунд
на offline case; 180 секунд и максимум 30 tool calls на model case; глобальный run budget
явный параметр manifest. Превышение любого budget — failed/incomplete evidence, не PASS.
Пересмотр budgets до baseline оформляется dataset revision, после измерений — отдельным решением.

CLI exit 0 разрешён только при полном успешном обязательном gate, а не при успешном сохранении
артефакта evaly. Failed assertions, missing evidence и inconclusive дают nonzero; например,
1 для assertion/infrastructure failure и 2 для incomplete/inconclusive. Report сохраняется
при обоих исходах. CI проверяет exit status И completeness всех ожидаемых cases/graders.
Опциональный незапущенный model profile не входит в offline denominator и остаётся NOT_RUN.

Минимум 20 именованных cases, каждый с expected evidence, timeout, allowed tools/effects:
1) обычный поиск; 2) одноимённые диалоги; 3) direct/group с одинаковым ID;
4) пустой корпус; 5) gap/incomplete history; 6) большой текст + resource;
7) пагинация и unchanged filters; 8) archive source и обязательные даты;
9) пустая archive page с has_more; 10) TTL/tombstone/policy revoke;
11) injection в сообщении; 12) malformed args/normalization; 13) numeric precision;
14) разрешённый send; 15) запрещённый send; 16) quote из другого диалога;
17) повтор request UUID и conflict; 18) timeout/unknown/restart без resend;
19) join без approval/с approval; 20) history cancel/restart без Events.
Дополнительно обязательны Events read/ACK redelivery и malformed resource URI сценарии.

Deterministic graders проверяют данные, citations/source IDs, gaps, effects, schemas,
permissions и число upstream dispatch. LLM judge допускается для качества текста как
дополнительная оценка; он не может зачесть безопасность по словам агента.

Evidence manifest: baseline/candidate SHA + tree hash, toolchain/library/SDK pins,
fixture hash, dataset revision, model ID/settings/prompt/skill revision (если model),
seed и число повторов, elapsed time, tool calls, bytes, token usage если доступна,
timeouts, skipped/failed cases, raw synthetic trace locations и grader revisions.
Недоступные token counts — null с причиной, не 0. Failures сохраняют уже случившиеся effects.

Acceptance: offline cases и обязательные assertions — 100%; unauthorized effects и
duplicate dispatch после unknown — 0. Model profile: минимум 3 парных прогона каждого
case с одинаковыми настройками; safety failures=0, aggregate completion candidate >= baseline,
ни один safety case не исключён. Если baseline flaky/недоступен, не объявлять superiority:
сохранить NOT_RUN/INCONCLUSIVE и отдельный незакрытый model gate. Paid/model execution не
является условием завершения offline migration, если пользователь отдельно его не поручил.

Performance baseline — frozen T01 sources, пересобранные выбранным в T02 toolchain;
исходный T01 binary отдельно сохраняется для behavior/rollback checks. Зафиксировать оба
build manifests и не выдавать смену компилятора за результат архитектурного улучшения.
Performance: одинаковая машина, Go, fixture, concurrency и exporters disabled; 5 независимых
прогонов после warm-up, каждый >=1000 вызовов на read/search/context. Median-of-run p95
candidate <=1.15*baseline; allocations/op <=1.20*baseline. Для нулевого baseline allocation
порог — 0. Фиксировать также CPU/OS, counts, errors, response bytes. Порог нельзя менять
после получения результата ради PASS; повтор оправдан только зафиксированной аномалией.
Model tokens/calls — измерение, не обязательное уменьшение при сохранении semantics.

## 8. Arch-go: исполняемые архитектурные границы

Использовать `github.com/arch-go/arch-go/v2`, исходный pin v2.1.2 (уже используется в tg-mcp),
проверить совместимость в T02. Вводятся `arch-go.yml`, pinned install target и
`make architecture`; CI обязан падать при нарушении. Не добавлять arch-go в production imports.
Конкретный syntax/globs проверить на pinned binary; это ТЗ задаёт правила, не обещает,
что строка с полным module path совпадает с package matcher без проверки.

| ID | Проверяемое правило |
|---|---|
| A01 | domain не импортирует runtime/adapters/app/SDK/ai-libs/SQL/zcago |
| A02 | app импортирует только собственные capability/shared packages и domain; external infra запрещена |
| A03 | mcpserver не импортирует storage/collector/messaging/historyimport/zalo/service или database/sql |
| A04 | toolruntime не импортирует concrete adapters/storage/zalo; toolsy ограничен toolruntime |
| A05 | adapters могут импортировать app/domain и явно перечисленные существующие backend packages; не mcpserver/service |
| A06 | metry/OTel импортируются только observability и явно требуемым composition кодом |
| A07 | evaly/evaluation/evalfixture доступны только evaluation commands/tests, production import graph не содержит их |
| A08 | docs/contracts не импортирует runtime; SDK ограничен mcpserver/существующим bridge и isolated evaluation support |
| A09 | production entrypoint вызывает service/config; wiring concrete implementations — composition root |

Каждому production package соответствует содержательное правило; generic allow-all для
100% coverage запрещён. Для существующих backend packages фиксируется разрешённый граф
без расширения новых входящих зависимостей из app/MCP. Third-party nested module анализируется
отдельно тестами, не переписывается под нашу package архитектуру.

С T02: compliance=100% для активных правил; coverage=100% для явно зафиксированного набора
production packages. Временные исключения имеют exact package/dependency, причину и задачу
удаления T04/T05/T06/T07. Запрещены широкие exclusions каталога. К T10 нет временных исключений
для новых boundaries; сознательно сохранённые backend edges перечислены в final graph.

Отрицательный контроль в изолированной fixture: нарушения domain->SQL, app->SDK,
mcpserver->storage, production->evaluation должны реально давать nonzero exit.
Отдельно проверять, что root package и subpackages совпали с globs и новый package не выпал
из coverage. Не исправлять провал ослаблением threshold. Production import graph дополнительно
проверяется `go list -deps`, поскольку test-only dependencies и nested modules имеют иной scope.

Arch-go проверяет статические imports/structure. Он не доказывает single session ownership,
atomicity, no resend, privacy, cancellation или корректность авторизации — это test gates.

## 9. Последовательные задачи: одна задача — один коммит

Общий DoD каждой задачи: её acceptance + актуальные regression tests + обновление evidence,
два независимых ревью (раздел 11), исправление всех findings до коммита. Не создавать
отдельные fixup/review commits внутри задачи. Не начинать следующую до принятия предыдущей.
Документация этого ТЗ не считается выполненной T01. Commit сообщений ниже — точные темы.

### T01 — Baseline и executable compatibility matrix

Зависимости: нет. Scope: contract fixtures и тестовый harness без изменения production behavior.
Снять manifest раздела 2, baseline SHA/tree/toolchain, существующие тесты/известные failures,
зафиксировать synthetic old binary или воспроизводимую сборку и dataset; определить точные
границы CLI/events/resources. Составить parity assertions для всех tools и negative inputs.
Artifacts: `docs/mcp-migration/baseline.md`, manifest, testdata, baseline measurement command.
Acceptance: manifest покрывает runtime catalog 1:1; tests различают допустимые динамические
поля и реальные изменения; baseline можно запустить без аккаунта; исходные failures явно отделены.
Checks: root test/vet, HTTP/STDIO/resources/contracts/operations suites, baseline smoke.
Commit: `test(mcp): freeze migration compatibility baseline`.

### T02 — Toolchain и arch-go gates

Зависимости: T01. Поднять Go до 1.27.1 или минимальной большей patch-версии, реально требуемой
выбранными pin; синхронизировать CI/build docs. Установить pinned arch-go вне runtime deps,
ввести правила A01–A09 с exact migration exceptions и negative controls.
Проверить доступные версии ai-libs в изолированном consumer probe с GOWORK=off.
Не коммитить абсолютные replace на соседние checkout; отсутствие подходящего релиза — BLOCKED
dependency gate, а не разрешение публиковать библиотеку. Результат probe записать в dependency matrix.
Acceptance: root/nested zcago тесты и CGO-free builds проходят; active architecture rules=100%,
negative controls fail ожидаемым правилом; нет незадокументированного dependency drift.
Commit: `build: pin migration toolchain and architecture checks`.

### T03 — Evaly harness и новая baseline evaluation

Зависимости: T02. Подключить pinned evaly; создать offline target/lifecycle/evidence и CLI.
Расширить существующую synthetic fixture cases раздела 7, не создавая альтернативную business
реализацию в harness. Снять исходную функциональную/производительную baseline до T04.
Acceptance: 22+ cases запускаются без credentials; effects фиксируются независимо от result;
повторный запуск изолирован; report учитывает failed/skipped; model profile opt-in и NOT_RUN
честно отражён. Не запускать платные модели без отдельного поручения/бюджета.
Checks: harness unit/integration, offline eval, architecture production dependency check.
Commit: `test(eval): add evaly harness and reproducible MCP baseline`.

### T04 — Application boundary для search/context/resources

Зависимости: T03. Вынести первый срез в app/search и app/conversations, создать store adapter,
сохранить typed IDs, gaps, response budgeting и ресурс полного сообщения. Остальные маршруты
временно остаются на baseline пути с точными exceptions. Source_id branches пока остаются
через существующий port до T06, но общий read contract уже описан.
Acceptance: первый срез проходит через app без MCP/SQL imports; parity HTTP/STDIO/resource,
FTS result order/cursors/coverage и limits не изменились; не появилась повторная выборка write.
Checks: relevant contract/storage/MCP tests, offline eval subset, arch-go.
Commit: `refactor(search): extract application handlers and read ports`.

### T05 — Toolsy execution для первого среза

Зависимости: T04. Подключить pinned toolsy core, создать toolruntime registry/view и trusted
subject/scope, binder, strict validation, domain error/result mapper; SDK остаётся native.
Сохранить annotations и wire projection; не подключать OperationProfile/result cache.
Acceptance: поиск/context выполняется один раз через toolsy; forbidden direct call не доходит
до handler; malformed/format-invalid input не dispatch; schemas и legacy normalization совместимы;
resource access также использует актуальную policy, не наследует разрешение из search.
Checks: integration invocation counters, schema parity, unauthorized calls, race, arch-go, eval subset.
Commit: `feat(runtime): execute search tools through scoped toolsy registry`.

### T06 — Все read tools, archives и diagnostic resources

Зависимости: T05. Перенести остальные reads на app/toolruntime, включая legacy group tools,
archive catalog/browse source_id, operation status и Events diagnostics/read paths.
Resources используют app ports напрямую с теми же access policies и самостоятельным admission.
SDK subscription/webhook transport продолжает обслуживать отдельный adapter.
Acceptance: полный read manifest покрыт; archive identities/TTL/tombstones/policy revoke/empty
page continuation сохранены; status не вызывает command; STDIO не получает ложную Events capability.
Checks: все read/resource/archive/transport suites, read evals, negative access, arch-go.
Commit: `refactor(mcp): migrate reads archives and resources to application ports`.

### T07 — Write/control dispatch без переноса ledger

Зависимости: T06. Направить send/join/history start/cancel/event ACK через toolruntime и app
command ports; operations adapter делегирует существующим владельцам. CLI approve/control
сохраняет authority и проходит ту же domain проверку; CLI не обязан становиться MCP tool.
Удалить перенесённые switch branches/direct concrete dependencies из mcpserver.
Acceptance: текущий полный tool catalog исполняется через registry; один owner каждой операции;
crash/restart, cancellation, concurrency, revoked auth, duplicate/conflicting request UUID,
join approval binding и history no-events проходят; no hidden grants/retries/second ledger.
Checks: messaging/collector/history/events/service/storage race suites, subprocess restart fixtures,
complete offline eval and parity, arch-go A03 без временных исключений.
Commit: `refactor(operations): route commands through toolsy preserving durable owners`.

### T08 — Metry и безопасный lifecycle telemetry

Зависимости: T07. Подключить pinned metry, observability adapter, provider lifecycle,
safe bounded metrics/spans, opt-in exporter config и schema/docs. Это additive configuration;
defaults не меняют сетевое поведение установленного сервиса. Убрать дублирующее execution logging,
сохранив существующую operational диагностику и redaction.
Acceptance: сигналы раздела 6 наблюдаемы; privacy sentinel отсутствует в sinks; nil/disabled
provider обработан корректно; exporter failure/timeout не меняет business outcome; shutdown bounded;
domain partial/unknown корректны при nil Go error; нет unbounded label cardinality.
Checks: in-memory exporter, privacy/failure/lifecycle tests, race, arch-go, full offline eval.
Commit: `feat(observability): instrument MCP execution and workers with metry`.

### T09 — Удаление переходного кода и сравнительная приёмка

Зависимости: T08. Удалить migration shims/unused branches, закрыть exceptions, обновить
architecture/development docs и команды CI; выполнить paired baseline/candidate eval и benchmarks.
Не удалять поддерживаемые legacy tools или сохранённые backend edges под видом cleanup.
Acceptance: требования разделов 4–8 выполнены; offline 100%, safety=0 violations, performance
thresholds соблюдены; regression findings исправлены в этой задаче; no unused temporary glue.
Model gate отдельно PASS/NOT_RUN/INCONCLUSIVE, без ложного заявления полной model acceptance.
Checks: root+nested tests/race/vet, arch-go, CGO-free platform builds, parity, full offline eval,
performance, old/new synthetic-state restart compatibility.
Commit: `refactor(mcp): remove migration glue and verify behavior parity`.

### T10 — Финальная независимая приёмка и handoff

Зависимости: T09. Два новых независимых субагента проверяют весь candidate, contracts,
git history T01–T09, evidence и scope. Подготовить итоговый report, limitations, rollback/runbook,
dependency lock matrix и status всех требований. Исправления findings допускаются внутри T10
с повторными релевантными checks и двумя ревью; итоговый коммит включает их и отчёты.
Acceptance: completeness 100%, correctness PASS, zero unresolved findings; все обязательные
offline gates выполнены; deferred/model/live чётко отделены от принятого scope; 10 task commits.
Commit: `chore(mcp): finalize architecture migration acceptance`.

## 10. Проверки, evidence и rollback

Существующие обязательные проверки не сокращать: root `go test ./...`, `go test -race ./...`,
`go vet ./...`; отдельно те же команды в `third_party/zcago`.
Сохранить Linux/native macOS CI и CGO_ENABLED=0 builds darwin/arm64, darwin/amd64, linux/amd64.
Добавить `make architecture`, `make eval-offline`, `make bench-mcp`, `make check-mcp-parity`
и общий `make check` с документированным составом. Названия — требования к будущим targets,
а не утверждение об их наличии. Model-run вынести в `make eval-model`, по умолчанию не CI.

Все тесты Arrange–Act–Assert, synthetic state в tempdir. Не запускать второй listener на
реальном аккаунте, не читать production SQLite для фикстур. Failure injection покрывает:
before claim, after claim/before dispatch, after external acceptance/before persist,
after persist/before response, race duplicate calls, cancellation, auth/policy revoke.

Evidence: `docs/mcp-migration/evidence/Txx/` — summary и safe machine-readable results,
commands/exit status/tool versions/candidate hash; большие raw logs могут храниться отдельно
по hash/reference. Доказательство нельзя заменить текстом «тесты зелёные» без команды и результата.
Не переиспользовать evidence после изменения затронутого поведения. Неуспех gate не скрывать
уменьшением dataset, исключением package или изменением expected под реализацию.

Реальное включение новой версии — отдельное разрешённое действие после разработки.
Handoff содержит сборку из чистого checkout, совместимость state, остановку/возврат старого
бинарника и перечень live checks; сам rollout в T01–T10 не выполняется.

## 11. Независимая приёмка

### 11.1. Правила для каждой задачи и финала

Два разных субагента не реализуют проверяемую задачу и не читают вердикт друг друга до
первого собственного отчёта. Главный агент выдаёт обоим одинаковые ТЗ, task scope, candidate
tree hash и evidence. Проверки могут идти параллельно, implementation tasks — последовательно.

Ревью A — полнота: разложить все нормативные пункты задачи и применимые общие инварианты
на leaf requirements, заранее назначить каждому ID. Все пункты равновесные:
`completeness = 100 * verified_pass / required_total`.
PARTIAL/FAIL/NOT_RUN/BLOCKED дают 0, не дробный балл. N/A допустим только если это заранее
объявленный non-goal; после начала задачи denominator нельзя уменьшать ради PASS.
Отчёт: ID, requirement, evidence, status, missing work, numerator/denominator, процент.

Ревью B — ошибки: contracts/API compatibility, authority, state transitions, concurrency,
resource/lifecycle/privacy, data migration, metrics/eval validity, architecture loopholes.
Каждый finding: severity, файл/строка или requirement ID, counterexample/repro, consequence,
fix expectation. Correctness PASS означает отсутствие обнаруженных нерешённых ошибок после
исправлений и повторной проверки, а не математическое доказательство отсутствия любых багов.

Принятие: A=100% И B=PASS И обязательные tests/gates PASS. Любая обоснованная ошибка
в нормативном scope блокирует acceptance независимо от severity. Спорный finding разобрать
с evidence; не считать закрытым по несогласию автора. Новые изменения после ревью требуют
повторной проверки затронутых требований обоими reviewer.

### 11.2. Коммит и связь с evidence

После двух PASS главный агент создаёт ровно один коммит задачи. Reviews внутри коммита
ссылаются на parent SHA и digest проверяемых исходников (исключая сами reports), чтобы не
возникало невозможной самоссылки на SHA содержащего их коммита. После commit сверить tree;
следующая задача фиксирует предыдущий SHA. Финальный report включает T01–T09 SHA и T10
parent/source digest; финальный T10 SHA сообщается в handoff после commit.
Не amend чужие/опубликованные commits. Findings до принятия исправляются в текущем working
tree; поздний finding предыдущей задачи явно учитывается в текущем task report и revalidation.

### 11.3. Приёмка самого ТЗ

До передачи на реализацию провести два независимых review этого документа по тем же ролям.
Матрица полноты спецификации может использовать заранее зафиксированные веса по разделам;
формулу/веса приложить к report. Ревью документа не засчитывает implementation acceptance.
Результат сохраняется в `docs/mcp-migration/spec-review.md` с hash проверенного ТЗ.

## 12. Goal для последовательного выполнения (не запускать в сеансе спецификации)

```text
Цель: выполнить ТЗ docs/task-mcp-architecture-ai-libs.md в репозитории zl-mcp.
Работай строго T01 -> T02 -> ... -> T10. На каждую задачу ровно один атомарный коммит
с указанным сообщением. Сначала контракты и evidence, затем реализация. Сохраняй пользовательские
изменения; не меняй соседние библиотеки и third_party вне оговорённого scope.

В начале прочитай ТЗ/AGENTS.md, проверь git status и уже принятые task reports/commits.
Создай активный goal на эту реализацию, если его ещё нет. После продолжения чата возобновляй
первую незавершённую задачу; не пересоздавай выполненные задачи и не сбрасывай evidence.

Для каждой задачи: зафиксируй leaf acceptance requirements и candidate hash, выполни работу
и релевантные проверки, запусти двух независимых субагентов (полнота в %, отсутствие ошибок),
исправляй findings до 100% и PASS обоих. Только затем коммит и следующая задача.
При невозможности получить compatible dependency, нарушении публичного/persistence контракта
или необходимости расширить scope зафиксируй blocker и запроси решение; не обходи gate.

Не разворачивай сервис, не перезапускай LaunchAgent, не используй живой аккаунт, не запускай
платные model evals и не публикуй изменения без отдельного поручения. Offline evaluation
обязательна; model/live gates без запуска остаются NOT_RUN и не препятствуют только явно
ограниченной offline-приёмке данного ТЗ. Не скрывай их в финальном результате.

Goal завершён, когда T01–T10 приняты, создано 10 task commits, обязательные offline checks
пройдены, финальные независимые reviewers дали 100% completeness и correctness PASS,
сохранены архитектура, dependency matrix, parity/eval/performance evidence и rollback handoff.
Финальный ответ: commits, evidence, acceptance, незапущенные model/live gates.
```

## 13. Исследовательская база и отложенные решения

Проверено 2026-10-11; при T01 перепроверять версии, не менять scope по одному roadmap.

- [MCP 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28): самодостаточные запросы; durable business state принадлежит приложению.
- [Tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools): schemas, annotations, predictable discovery и разные error surfaces.
- [Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http): transport boundary остаётся SDK/HTTP adapter.
- [Go SDK releases](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0): исходная версия для совместимости, не автоматическое обещание отсутствия SDK limitations.
- [Effective tools](https://www.anthropic.com/engineering/writing-tools-for-agents): оценивать workflow success и context cost; не выбирать granularity по эстетике API.
- [MCP roadmap](https://modelcontextprotocol.io/development/roadmap): progressive discovery/transport/event evolution не считать готовой обязательной возможностью.
- [Tasks extension](https://tasks.extensions.modelcontextprotocol.io/specification/2026-07-28/tasks): возможный следующий отдельный эксперимент поверх history operations после проверки host support.
- [Arch-go](https://github.com/arch-go/arch-go): import rules и coverage, не доказательство runtime safety.

Локальные library contracts: toolsy `docs/execution-contract.md`, `mcp/README.md`,
`mcp/export.go`; metry `INTEGRATION.md`, `PRIVACY.md`; evaly `docs/execution-contract.md`,
`docs/integration-contract.md` и compiling public examples. При T02 фиксируются ревизии.
Следующие отдельные ADR: scoped read/messaging profiles; toolsy/mcp export parity improvements;
SQLite OperationStore и перенос одного ledger; Tasks/history; улучшение поиска с ragy.
Ни один из этих пунктов не является неявным условием завершения T01–T10.
