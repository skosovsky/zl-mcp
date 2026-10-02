# Проверка локальных патчей zcago

Дата: 2 октября 2026 года. Исследование выполнено без live-запросов к Zalo и без изменения установленного сервиса или Go-зависимости.

## Проверенная upstream-версия

Публичный [репозиторий amrakk/zcago](https://github.com/amrakk/zcago) клонирован отдельно в `/tmp/zl-mcp-zcago-upstream-review-20261002`. Default branch main и её HEAD — `d4ff65b460577b2557e70220b68d08ce1f7431b4`, commit от 8 сентября 2026 года. Это та же версия, которая закреплена в go.mod проекта. Проверены remote heads/tags: main — единственная опубликованная ветка; последний version-tag v0.3.0 предшествует HEAD. Нового upstream commit, заменяющего наши правки, не найдено.

Runtime-копия отличается от upstream в трёх существующих файлах: model/message.go, session/auth/login.go, internal/websocketx/client.go. Дополнительно добавлены errs/authentication.go (типизированный sentinel), два regression test-файла и PATCHES.md. Остальной общий runtime source совпадает; CLI/examples в локальную копию не включены.

## Вывод по каждой правке

| Правка | Что уже есть в upstream | Наблюдение и вывод |
| --- | --- | --- |
| Quote ID/timestamp: число или строка | TQuote.UnmarshalJSON обрабатывает ownerId, но cliMsgId/globalMsgId/ts декодируются как int64 через alias | Строковый cliMsgId даёт json unmarshal error. Listener декодирует сообщение до передачи в приложение; смена типов только в нашем адаптере не исправляет вход. Патч нужен для наблюдавшегося Reply/replay. |
| Login/server-info error codes и отмена | Общий HandleZaloResponse умеет обрабатывать error_code для API, но auth.Login использует ParseBaseResponse/decryptLoginResponse; server-info напрямую ParseZaloResponse | Ошибки outer/decrypted/server-info не превращаются в typed code; отмена с nil response вызывает panic в makeServerInfoRequest. Патч нужен. Общий helper не является уже работающей альтернативой для этих путей. |
| WebSocket HTTP 401 | Dial отбрасывает response, но поддерживает внедряемый HTTPClient через options и session client | Без правки нет auth sentinel. Однако типизированную причину можно вернуть из адаптерного RoundTripper: оригинальный Dial сохраняет её через errors.Is. Этот кусок патча можно вынести из зависимости в наш адаптер. |

Исходники для проверки: [TQuote](https://github.com/amrakk/zcago/blob/d4ff65b460577b2557e70220b68d08ce1f7431b4/model/message.go), [login/server-info](https://github.com/amrakk/zcago/blob/d4ff65b460577b2557e70220b68d08ce1f7431b4/session/auth/login.go), [общий HTTP helper](https://github.com/amrakk/zcago/blob/d4ff65b460577b2557e70220b68d08ce1f7431b4/internal/httpx/client.go), [WebSocket Dial](https://github.com/amrakk/zcago/blob/d4ff65b460577b2557e70220b68d08ce1f7431b4/internal/websocketx/client.go), [публичные options](https://github.com/amrakk/zcago/blob/d4ff65b460577b2557e70220b68d08ce1f7431b4/options.go).

## Автономные доказательства

Наши regression tests перенесены в временную upstream-копию; current app/internal и schemas скопированы в `/tmp/zl-mcp-zcago-unpatched-app-20261002`, где replace указывает на upstream. Сначала чистый upstream не смог собрать приложение: errs.ErrAuthenticationRequired отсутствует. Для выполнения поведенческих тестов добавлена только декларация sentinel в errs/authentication.go. Она не меняет ни один runtime path; три исследуемых исходных файла оставлены неизменными, что проверено git diff. Это совместимая декларация для теста, а не утверждение, что чистый upstream собирает нынешнее приложение без адаптации.

Результаты на upstream с этой декларацией:

- TestQuoteWireIntegerAndStringIDs: FAIL, `cannot unmarshal string ... cliMsgId ... int64`.
- TestDialClassifiesOnlyExplicitUnauthorized: FAIL на 401, обычная ошибка handshake вместо typed auth cause.
- TestLoginResponsePreservesCodeAndExplicit401: FAIL, потерян код outer login response.
- TestDecryptedLoginAndServerInfoPreserveCode: FAIL, decrypted rejection даёт nil error.
- Отдельный TestReviewServerInfoCode: FAIL, error_code=102 даёт nil error.
- TestReviewAuthHTTP401: FAIL для login и server-info, HTTP 401 с синтетическим JSON body даёт nil error.
- TestServerInfoCancellationWithoutResponseDoesNotPanic: FAIL, nil pointer dereference в makeServerInfoRequest.

Те же существующие регрессии на нашей patched зависимости и quote test приложения прошли с `-count=1`. Дополнительный TestReviewInjectedTransportClassifies401 прошёл на неизменённом upstream Dial: локальный HTTP server возвращает 401/403/429/500; RoundTripper возвращает собственный sentinel только для 401, Dial сохраняет его через errors.Is. Проверка не касается реальных credentials и не доказывает live-сценарий целиком.

Использованы Go 1.26.5 и текущие зависимости. Это unit/regression проверки, не модельные evals. Runtime-код zl-mcp, local replace и установленный бинарник не менялись.

## Рекомендация

Сейчас сохранять local replace: удалить всю third_party/zcago и перейти на upstream без изменений нельзя без потери требуемого поведения. Quote decoding и исправления login действительно нужны; их стоит сопровождать как минимальный fork и предложить upstream отдельными исправлениями. По поручению пользователя опубликованы отдельные issue (см. ниже).

Отдельной доработкой можно перенести распознавание HTTP 401 в HTTP transport нашего адаптера, распознавать собственную типизированную auth cause и удалить websocketx patch вместе с зависимостью приложения от добавленного upstream sentinel. Нужно сохранить cookie jar/timeouts, закрытие response body, различие 401 и 403/429/500, редактирование безопасных ошибок и регрессии. Проведённый probe подтверждает транспортный механизм, но этот рефакторинг здесь не выполнен.

Ручное исправление расшифрованного login или WebSocket frames в RoundTripper не рекомендуется как замена оставшихся патчей: оно переносит приватную логику библиотеки в приложение и усложняет сопровождение. Избавиться от папки в репозитории можно, подключив свой опубликованный fork по версии/commit; это изменит размещение зависимости, но не отменит необходимость исправлений.

## Опубликованные upstream issue

Проверены существующие открытые и закрытые issue; дубликатов не найдено. Каждый issue содержит проверенный commit, синтетическое воспроизведение, фактическое и ожидаемое поведение. Данные аккаунта не публиковались.

- [TQuote.UnmarshalJSON rejects decimal-string message IDs and timestamps](https://github.com/amrakk/zcago/issues/2)
- [WebSocket Dial discards handshake HTTP status needed to classify authentication failures](https://github.com/amrakk/zcago/issues/3)
- [Login ignores nonzero error_code in outer and decrypted response envelopes](https://github.com/amrakk/zcago/issues/4)
- [Server-info parses the normalized response type and silently ignores wire error_code](https://github.com/amrakk/zcago/issues/5)
- [Login and server-info ignore HTTP 401 and can return success for an unauthorized response](https://github.com/amrakk/zcago/issues/6)
- [makeServerInfoRequest panics when transport fails without an HTTP response](https://github.com/amrakk/zcago/issues/7)
