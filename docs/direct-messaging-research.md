# Исследование расширения личных сообщений

Задача: [task-direct-messaging.md](task-direct-messaging.md).
Статус: исследование и проектирование; готовность расширения не подтверждена.

## 3 октября 2026: локальное discovery

На установленном сервисе выполнены два запроса чтения с существующим локальным
MCP-токеном внутри процесса. Токен не выводился и не сохранялся в отчёте.

- `server/discover`: HTTP 200, capabilities содержат events, tools, resources и logging.
- `events/list`: HTTP 200, определения `zalo.message.created` и `zalo.conversation.message.created` присутствуют.
- SHA-256 установленного бинарника: `af71d874a74e170694900bf58855547a321e949d11cffbf4337fe9bfc2ac1412`.
- Профиль существующего туннеля направляет MCP на проверенный loopback endpoint.

Само отсутствие определения в ChatGPT не воспроизведено на локальном endpoint.
Нельзя считать причиной старый бинарник или отсутствие capability на этом процессе.
Проверка внешнего маршрута и клиентского discovery ещё необходима; кеш клиента
остаётся гипотезой, а не установленной причиной.

Туннель на момент проверки отвечает HTTP 200 на /healthz и /readyz. Это проверка
готовности процесса, а не доказательство прохождения events/list из ChatGPT.
Во встроенном браузере страница плагинов открывается без пользовательской сессии;
проверка каталога конкретного установленного плагина через этот браузер недоступна.
Сессию другого браузера и credentials не переносить для обхода ограничения.

В профиле Authorization задаётся ссылкой `env:ZALO_MCP_AUTHORIZATION`, а не готовым
Bearer. Существующий supervisor перед exec читает локальный MCP-токен и заполняет
эту переменную. Запрос с буквальным значением env-ссылки возвращает 401, что ожидаемо
и не доказывает неисправность туннеля. Runtime API key для этой проверки не читался.

## Исходники отправки

Закреплённый `third_party/zcago/api/send_message.go` предоставляет SendMessage,
текст и необязательный SendMessageQuote. У цитаты обязательны upstream-метаданные;
имеющегося reply ID недостаточно. Текстовый запрос создаёт clientId из текущего
времени при каждом вызове. Автоматический повтор сетевого вызова при неизвестном
результате не имеет доказанной защиты от дублирования.

Следующие проверки: воспроизведение discovery через клиентский маршрут; фиксация
совместимого версионирования payload и фильтров; исследование цитаты, текстового
лимита и безопасной классификации неоднозначной отправки на синтетическом transport.
Рабочие подписки, сессия и процессы при исследовании не менялись.

## Начало реализации журнала отправки

Зафиксированы [правила контракта](contracts/direct-messaging.md) и исполняемые
send_request.input.json / send_operation.output.json. Пока это общие контракты;
новые MCP tools ещё не зарегистрированы. Выбран отдельный профиль Events версии 2,
чтобы не менять строгий payload версии 1 у действующих receiver.

В исходниках добавлен журнал операций без текста, с постоянным отпечатком аргументов,
UUID, ограничением вместимости, атомарным claim и явным восстановлением interrupted
sending в unknown. Восстановление должно вызываться сервисом под account lock, а не
любым открытием SQLite. Подключение этого lifecycle и upstream/MCP ещё предстоит.

Синтетические storage/domain tests прошли: конкурентный claim имеет одного победителя,
повтор terminal/unknown не запускает отправку, изменённые аргументы конфликтуют.
Установленную БД эта проверка не открывает и не мигрирует.

## Подключение отправки к сервису

В текущих исходниках зарегистрированы zalo_send_direct_message и zalo_get_send_status
(всего 14 tools). Менеджер использует текущую сессию collector через доменный Sender;
новую сессию/процесс для отправки не создаёт. Под account lock сервис восстанавливает
прерванные sending в unknown до приёма запросов. allow_send по умолчанию false,
send_recipient_ids задаёт отдельное ограничение адресатов.

Для текстовых direct-записей сохраняются идентификаторы цитаты, без второй копии текста.
FK каскадно очищает их при удалении/retention сообщения. Старые записи без upstream
метаданных цитаты не объявляются пригодными для ответа: возвращается QUOTE_UNAVAILABLE.
Необходимость восстановления этих метаданных для доступного replay ещё исследуется.

Root go test ./... прошёл. Добавлены проверки неоднозначной отправки без повтора,
разрешённого адресата, цитаты/удаления, подтверждённого результата после отмены клиента
и преобразования доменного ответа в upstream quote. Это синтетические проверки;
реальная отправка и внешнее discovery не подтверждены. Установленный сервис не менялся.

## Направление и первое входящее

Добавлена транзакционная миграция 6: immutable direction/first_incoming в журнале
событий, фильтры подписки и постоянные маркеры peer_first_incoming. Старые подписки
получают direction=all без изменения ID, generation/start_seq и очереди. Новизна
прежних диалогов без достаточных данных остаётся unknown. Неизвестное направление
также не позволяет объявить следующее сообщение первым.

Профиль zalo.conversation.message.created.v2 и исполняемые схемы отдельно от версии 1.
Fanout применяет direction и first_incoming_only к сохранённым фактам; canonical ID
включает фильтры с нормализованными defaults. Каталог Events содержит три определения.
Подписка не создаётся при включении collection all или запуске сервиса.

Синтетические проверки подтвердили один первый входящий после собственного исходящего,
дедупликацию/replay, deletion/restart, unknown, migration baseline и сохранение
подписки/queued bytes. Receiver независимо проверяет HMAC и payload версии 2;
проверены canonical defaults, отличающиеся фильтры и отмена. Root go test ./... прошёл.

Matching replay теперь может дополнить отсутствующие метаданные цитаты прежней
сохранённой записи, если совпадают peer/message ID, автор, текст и sent_at. Это не
создаёт новое событие и не заменяет текст. Live-восстановление метаданных не проверено.

## Проверка отправки через единый сервис

Добавлен синтетический интеграционный сценарий production service → HTTP MCP →
текущая сессия collector. Цитированный ответ, повтор, остановка/запуск и чтение
статуса сохраняют подтверждённый результат без второго upstream-вызова. Отдельный
обычный текст по известному ID отсутствующего в корпусе собеседника использует ту
же сессию. Это проверка маршрутизации, а не разрешения Zalo писать любому аккаунту.

Storage-тест подтверждает восстановление quote metadata только при совпадении
сохранённого сообщения; конфликтующий replay не дополняет цитату. Количество
сообщений и событий остаётся прежним. Ошибки SQLite при чтении статуса и цитаты
теперь возвращают STORAGE_ERROR, а не NOT_FOUND/QUOTE_UNAVAILABLE, без upstream-send.

Актуализированы development guide и формат результатов event skill. Проверка
структуры выявила двоеточие в неэкранированном YAML description; поле переведено
в многострочный scalar. Установленный сервис и пользовательские копии skills
не менялись. Внешнее discovery и согласованная live-проверка остаются отдельными
незавершёнными требованиями.

## Классификация результата upstream

При чтении api.resolveResponse и httpx.handleZaloResponse установлено: один
ZaloAPIError включает protocol error_code, HTTP status и code=0 при нарушении
декодирования. Нельзя считать сам тип доказательством отказа. Adapter теперь
сохраняет ambiguous для nil/0 и кодов в диапазоне HTTP, кроме известного protocol
invalid-params 114. Остальные отличимые ненулевые protocol codes классифицируются
как отказ. Тесты проверяют value/pointer формы, HTTP 408/500/302, decode=0,
отсутствующий code, protocol 114 и синтетический protocol -10. Никакие исходные
тексты upstream-ошибок не передаются клиенту. Реальный отказ с перекрывающимся
HTTP-кодом может оставаться unknown: происхождение библиотека не сохраняет.

## Сервис, транспорт и остановка во время отправки

Wire-тесты закреплённого zcago прошли с race detector: локальный receiver
расшифровывает реальный POST form, проверяет обычный /sms и /quote, Unicode,
decimal-string IDs, автора/type/TS/TTL цитаты и зашифрованный ответ с message ID.
Это не live-отправка. Реализация зависимости не изменялась; добавлены только тесты.

Интеграционный TLS-сценарий полного сервиса проходит для v1 и v2. V2 incoming
отсекает собственные исходящие, сохраняет first_incoming после restart и передаёт
текст/resource URI с независимо проверяемой HMAC-подписью. Отмена не останавливает
сбор. Отдельная MCP-отправка с потерянным результатом остаётся unknown после
повтора и перезапуска; дополнительного upstream-вызова нет.

Приёмка остановки во время заблокированной отправки сначала воспроизвела
удержание account lock после shutdown. Одного BaseContext HTTP-сервера оказалось
недостаточно: запрос инструмента жил независимо. Domain port теперь объединяет
контекст вызова с lifecycle сервиса, отменяя send до закрытия session/SQLite.
После исправления сценарий проходит с race detector: unknown записан, account
lock освобождён, restart/status/repeat не отправляют сообщение второй раз.

Бюджеты согласованы: upstream 30 секунд, запись результата 3 секунды, tool 35,
HTTP-клиент STDIO-моста 40, write timeout сервиса 45. Установленные процессы,
пользовательские подписки и runtime-state в этих сценариях не использовались.

## Завершение автономных сценариев

Миграции 5/6 проверены с trigger, прерывающим запись schema_migrations после DDL:
ошибка не оставляет новые таблицы/колонки/version marker и не меняет сохранённый
текст. После удаления искусственной ошибки миграция и повтор проходят нормально.

40 конкурентных вставок 10 incoming identities имеют ровно один first_incoming;
маркер ссылается на первую вставку seq. Истечение retention удаляет все старые
тексты, но следующая запись того же peer имеет first_incoming=false. Проверка
работает с race detector. В запросе нового теста исправлена неверная ссылка на
message_id в event journal: связь проходит через message seq.

Два настоящих STDIO subprocess проходят отправку/чтение статуса с одним UUID
через один production service. Collector один, upstream-send один, синтетический
приватный текст не попадает в stderr мостов и файл логов сервиса.

Повторная диагностика установленного endpoint: server/discover и events/list
вернули HTTP 200 без RPC error; capabilities events/logging/resources/tools,
профили zalo.message.created и zalo.conversation.message.created. SHA-256 бинарника
по-прежнему af71d874a74e170694900bf58855547a321e949d11cffbf4337fe9bfc2ac1412.
Изменение каталога ChatGPT через этот маршрут не доказано. Профиль v2 существует
в новых исходниках, но в установленном бинарнике его ещё нет.

Проверка 34 Markdown-файлов не обнаружила отсутствующих локальных link targets;
gitleaks dir с проектной конфигурацией не нашёл секретов в рабочем дереве.

## Воспроизводимость кандидата и граница клиентской диагностики

Отдельный source snapshot вне iCloud содержит 426 tracked/unignored Git-файлов
с SHA-256 manifest. Root/nested go test и vet, а также обе no-CGO сборки прошли.
Snapshot включает незакоммиченные изменения; это не checkout опубликованной
ревизии. Manifest и бинарники находятся в временном локальном каталоге, runtime
сервиса не менялся. Полные root/nested test/race/vet перед этим прошли в worktree.

Read-only история клиентского чата подтверждает вызовы
automations.discover_webhook_schema по идентификатору подключения, без указания
event name в фактическом запросе. Ответ этого вызова текущий инструмент чтения
истории не возвращает, поэтому каталог клиента и способ выбора профиля остаются
непроверенными. Исторический локальный отчёт клиента тоже завершился до
authenticated discovery из-за ограничений его диагностики; он не доказывает,
что сервер отвечал неполным каталогом.

Следующая проверка — получить actual client discovery result и выяснить, выбирает
ли интеграция один профиль либо показывает весь events/list. Нельзя исправлять
это сменой порядка профилей или объявлять кеш причиной без доказательства.
Сообщений в другие чаты, обновления подключения и новых подписок не выполнялось.

## Повторная проверка поступившего direct-сообщения и клиентского доступа

После сообщения пользователя о новом входящем подтверждена его live-запись в
личном диалоге: прежние записи replay также доступны. Это устраняет необходимость
ждать новое сообщение для проверки корпуса. Установленная БД по-прежнему имеет
версии миграций 1–4 и не содержит quote metadata; возможность цитирования новой
реализацией требует trial migration и доступного upstream replay, а не ожидания
произвольного нового сообщения. Чтение корпуса не разрешает исходящую отправку.

В этой сессии появились восемь Zalo connector tools прежнего группового интерфейса.
Реальный удалённый zalo_get_status успешно вернул connected/authenticated и
обновлённые last_event_at/last_persisted_at. Маршрут обычных tools работает.
Локальный endpoint объявляет tools нового conversation-интерфейса и два Events
профиля; каталог доступных клиенту tools их не отражает. Это подтверждённое
расхождение каталогов, но не доказательство конкретной причины или ответа
удалённого events/list. Ограничение/обновление discovery клиента остаётся
следующим шагом; новые subscriptions и исходящие sends не выполнялись.

## Discovery and listener follow-up, 2026-10-03

The running endpoint was queried directly with its existing local token (never
printed). It returned twelve tools, both legacy and conversation Events profiles,
and Events capability in `server/discover`. The client initially displayed eight
tools and only the legacy group event. After a tunnel-client restart and client
refresh, the user confirmed discovery was working. This sequence establishes
recovery, but does not isolate whether transport reconnect or metadata refresh
caused it. No collector or subscription changes were made for that check.

Collection still reported `reconnecting` / `UPSTREAM_UNAVAILABLE`. A private
log exposed only the decoding error class, field `data.rMsg.gMsgID` and expected
`int` type. Source review and a failing regression establish a cleanup defect:
`Stop` could leave the socket assigned after cancellation, causing all later
`Start` calls to reject an already-started listener. Source cleanup and exact
integer/string reaction ID parsing are patched; live recovery and callback
delivery remain separate acceptance checks.

## Quoted-send acknowledgement investigation

The authorized quoted reply was independently observed in the corpus with its
correct reference, while the send ledger remained `unknown`. The raw HTTP
response/error was not retained, so a retrospective exact cause is unavailable.

Source review found a concrete mismatch: Go `SendMessageResult.msgId` required a
JSON string, whereas the maintained JavaScript reference declares a number.
Encrypted `/sms` and `/quote` local receiver tests now supply an exact integer
above 2^53. Both failed before the patch with `ZaloAPIError[0]`; the adapter maps
that code to ambiguity. Both pass after an exact integer/string decoder patch.
This verifies the failure mechanism without another live message. The previous
wire fixture used a string for both routes and did not cover numeric replies.

This supports the numeric reply as a plausible cause, not proof of the lost live
response. No blanket success inference, text-based ledger reconciliation or new
UUID retry was added. Root and nested race/test/vet passed. A future independently
authorized quote can check the patched acknowledgement; the original operation
and its evidence remain intact.
