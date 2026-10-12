# T09 latency FAIL — handoff

Решение пользователя: «Оставляем приложения работать; сохрани latency FAIL и handoff.»
Goal остаётся blocked. T09 не принят и не закоммичен. SDK-01/T10 не начинались.
Приложения не закрыты; повторные замеры не запускаются. Deployment, LaunchAgent,
живой аккаунт, paid model evaluation и публикация не выполнялись.

## Зафиксированный candidate

- Parent: `61d8b6c31ccdcbc107ae92114000ba677e53110d` (принятый T08).
- Source digest: `b6b51db253ddab35174a8a09d934f6145627c04c27cf88d60076f703c0b4ef84`.
- Официальный toolsy v0.20.2; локальные диагностические patches не приняты.
- `requirements-v2.json`: 62 leaf requirements; final reviews на этой редакции ещё не получены.
- `performance-gate-contract-v2.json`: p95 ≤ 1.15× baseline; allocations/op ≤ 2×; B/op ≤ 1.20×.
- Baseline T03 и исторические FAIL сохранены без переклассификации.

## Авторитетный performance результат

`performance-comparison-v4.json`: FAIL. 15 измерительных прогонов, 5×1000 вызовов
на каждый read/search/context, без ошибок. Все objects/B/op gates PASS;
все p95 gates FAIL.

| Сценарий | p95, мс | Предел, мс | Objects/op факт / предел | B/op факт / предел |
| --- | ---: | ---: | ---: | ---: |
| read | 7.153 | 4.098 | 1585.700 / 1735.280 | 495971.416 / 516337.795 |
| search | 6.430 | 3.882 | 2124.235 / 2288.540 | 547051.704 / 558227.242 |
| context | 6.361 | 4.077 | 2053.564 / 2132.106 | 539645.728 / 546305.126 |

`performance-v3/` хранит raw measurements и manifest.
`latency-anomaly-proof-v1.json` и `baseline-parallel-control-v1/` подтверждают:
сопоставимый frozen baseline также замедлился примерно вдвое при текущей нагрузке.
Это диагностическое evidence, а не замена приёмочного baseline и не PASS p95.
`latency-blocked-audit-v3.json` фиксирует три последовательные проверки блокировки.
Ошибочные первоначальные PASS drafts сохранены как invalid; использовать их для приёмки нельзя.

## Проверки, которые прошли

- `toolsy-v0.20.2-checks.json`: root/nested tests, race, vet; 18 Python checks;
  consumer probe и restart в обе стороны.
- `architecture-v6.json`: архитектурные ограничения и negative/source controls PASS.
- `build-manifest-v6.json`, `platform-build-manifest-v3.json`: native/eval и CGO-free builds PASS.
- `offline-v6/report.json`: 50/50 cases, 480/480 assertions PASS.
- `paired-v6/report.json`: обе стороны 50/50, 480/480; 50 canonical pairs PASS.

Ранее независимые reviewers v4 проверяли прежний performance contract (61 leaf),
сохранили FAIL и не заменяют будущие reviewers по 62 leaves.
Model/live gates: **NOT_RUN**. JSON-RPC ID gates: deferred to **SDK-01**, не PASS.

## Принятые коммиты

- `a3151c6bbee589cfaa3aeb21b147afe59e2ad321` — test(mcp): freeze migration compatibility baseline
- `23b3e200e18ad0d0433f38fe4832207c01bb74d2` — build: pin migration toolchain and architecture checks
- `da3d7b6f0369f85e58d17b546c933b362c342224` — test(eval): add evaly harness and reproducible MCP baseline
- `775d3a271110b2771a9cc0ac63d3f51151481e53` — refactor(search): extract application handlers and read ports
- `45230af90f90cf27358f41e5ff52949fd686525d` — feat(runtime): execute search tools through scoped toolsy registry
- `a0ed93568dd3dae388e4bc65142becceb5085890` — refactor(mcp): migrate reads archives and resources to application ports
- `bcab7133d00a34db8bfc148fee80480d05c1126f` — refactor(operations): route commands through toolsy preserving durable owners
- `61d8b6c31ccdcbc107ae92114000ba677e53110d` — feat(observability): instrument MCP execution and workers with metry

## Продолжение

1. Не пересоздавать T01–T08 и не сбрасывать рабочее дерево/evidence. Проверить git status,
   source digest и целостность artifacts; пользовательские изменения сохранять.
2. После отдельного продолжения и снижения нагрузки выполнить полный candidate run
   (5×1000/kind), сохранив новые raw artifacts в новом каталоге. Не заменять baseline.
   Если `/tmp/zl-T09-build-v6` исчез, восстановить binaries через build_eval.py и
   заново проверить provenance/source hashes; не доверять старым временным путям.
3. Запустить fail-closed compare_performance.py против исходных T03 manifest/build.
   Только действительный PASS всех трёх бюджетов открывает performance gate.
4. Два независимых reviewers: completeness 62/62=100%, correctness PASS;
   исправить findings, заново зафиксировать candidate и релевантные checks.
5. Только после gates создать единственный T09 commit:
   `refactor(mcp): remove migration glue and verify behavior parity`.
6. Затем SDK-01, затем T10; всего требуется 11 атомарных task commits.

Изменения T09 остаются в рабочем дереве. Сервис не развёрнут, поэтому runtime rollback
не требуется. Не выполнять reset/clean и не перезапускать LaunchAgent для rollback.
Последний принятый код — T08; handoff не выдаётся за финальную приёмку T10.
