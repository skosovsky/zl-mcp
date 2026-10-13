# Проверки поведения и модельные checkpoints

Текущие постоянные проверки работают на Go/evaly: `make eval-offline` запускает
синтетические сценарии сервиса без модели, аккаунта и установленного состояния.
`make check` включает этот gate, tests/race/vet и проверки архитектуры.

Успешный offline run доказывает проверенные контракты сервиса, но не фактический
выбор tools моделью, полноту её ответа или качество human review.

## Offline checkpoint replay

Уникальные проверки прежнего Codex harness сохранены в Go graders и версионном
synthetic dataset. См. [контракт checkpoints](../model-checkpoints.md): проверяются
источники, уточнения, skill selection, stop rules, recovery и отсутствие повторной
мутации. Проверка наблюдений не запускает Codex, модель или внешний provider.
Checkpoint PASS имеет ограниченный scope; model/live остаются NOT_RUN, а human
answer review не присваивается автоматически.

## Отдельно разрешаемый model profile

`make eval-model MODEL_DRIVER=/absolute/authorized-driver` остаётся opt-in и не
входит в CI. Отсутствующий driver и неполный model setup дают NOT_RUN/INCOMPLETE
с ненулевым exit code. Подготовка к публикации и checkpoint replay не разрешают
запуск модели. Полный model gate сейчас не принят: имеющийся protocol smoke
не заменяет настройку и выполнение всей матрицы.

## Synthetic fixture

Для изолированных transport tests используется `cmd/zl-mcp-eval-fixture` с
`docs/evals/fixtures.json`. Он создаёт новое временное состояние, synthetic
upstream и те же schemas/tools, не читая production config/account. Клиенту
нельзя одновременно предоставлять рабочий Zalo MCP. Права AI-клиента на shell,
файлы и connectors ограничиваются отдельно от STDIO-сервера.

Approved-join использует только приглашение `https://zalo.me/g/evalapproved`,
synthetic preview/разрешение и исходный request UUID. Неопределённый результат
не разрешает новую заявку. Никакое разрешение fixture не разрешает действие
в реальном аккаунте. Счётчики effects и lifecycle evidence обязательны для
поведенческой приёмки, а не выводятся из текстового ответа.

## Исторические model runs

[cases.json](cases.json), [results-index.json](results-index.json) и прежние
human-review результаты остаются историческими; их не переоценивает эта уборка.
Старые Python runner/exporter доступны через immutable T10 checkout из
[harness maintenance](../harness-maintenance.md), но не используются в текущем CI.

Исторический export выбирал последние полные reruns, сохранял исходные checks
и требовал отдельного просмотра answer/traces. Пересчёт checkpoints никогда не
считался новым model run. Trigger-only проверки не доказывали semantic recall,
а exposure skill через fixed MCP resources не доказывала native skill discovery.
Историческая проблема Codex CLI 0.137.0 с отдельным MCP approval prompt также не
превращается в успешный join-eval. Все старые traces, хеши и отметки human review
сохраняются; новых model/live прогонов здесь нет.
