# Независимое ревью кандидата MCP hardening

Основание: [R01–R12](../task-mcp-skills-hardening.md),
[43 обязательных критерия](../mcp-skills-hardening-acceptance.md).
Дата: 2026-10-18. Проверено незакоммиченное дерево поверх
`a3e0baea4396a30fa6902cea6b831bfe39956001`.

Это предварительное ревью разработки. Итоговая опубликованная ревизия,
установленный backend и обновлённое задание Ann ещё не приняты.

## Независимые первые verdict

Оба рецензента проверяли весь объём и вынесли первые verdict до обмена выводами.

| Рецензент | PASS / 43 | Полнота | FAIL | NOT_RUN | Verdict |
| --- | --- | --- | --- | --- | --- |
| A (`hardening_acceptance_a`) | 25 / 43 | 58,14% | 2 | 16 | NOT ACCEPTED |
| B (`hardening_acceptance_b`) | 25 / 43 | 58,14% | 3 | 15 | NOT ACCEPTED |

Классификация критериев различалась: A отнёс неверное отображение ошибок к
R03.3, B отнёс legacy-fallback и устаревшую прозу к R05.1/R09.3/R11.3. Знаменатель
сохранён; эти проценты не пересчитаны задним числом после исправлений.

## Замечания и независимая перепроверка

| ID | Источник | Замечание | Результат повторной проверки |
| --- | --- | --- | --- |
| C01 | A и B | R01: после blocked journal нет устойчивого продолжения | IMPLEMENTED CANDIDATE: отдельный опциональный journal.ready, actual-SDK synthetic host проверен; реальный host и выпуск ещё не приняты |
| C02 | A | Ошибки хранилища и отмена превращались в постоянный NOT_FOUND | CLOSED A: безопасные категории/retryability и отсутствие утечки проверены targeted tests |
| C03 | B | Content-tool отвергал выданный legacy group URI в tools-only workflow | CLOSED B: LegacyGroup, исходная проекция, encoder URI и коллизия group/direct проверены SDK regression |
| C04 | B | В свободной прозе справочника оставалось «25 tools» | CLOSED B: заменено на каталог tools; генератор и ручная сверка подтверждены |
| C05 | B; A тоже не засчитал критерий | R06.2: не хватало содержательных валидных fixtures для сравнения схем | CLOSED B: динамический набор всех 11 изменённых outputs, реальные SDK request/response и invalid-derived fixtures проверены обоими валидаторами |

Тестовые основания исправлений находятся в репозитории:

- `internal/app/contentread/read_test.go` — категории ошибок и приватность;
- `internal/mcpserver/read_content_test.go` — legacy URI, namespaces, ресурсы,
  tools-only и полный wire budget;
- `internal/mcpserver/publication_equivalence_test.go` — 11 изменённых schemas,
  22 валидных request/response fixtures, 33 невалидных производных варианта;
- `scripts/reference` — воспроизводимый каталог и negative controls.

Исправления C02–C05 не дают автоматического нового процента полноты. После
реализации R01 и выпуска оба рецензента должны проверить точную итоговую
ревизию, всю матрицу и выдать новые независимые verdict. Ни один текущий отчёт
не подтверждает 100% и 0 ошибок по полному goal.

## Повторный verdict B после исправлений

ReviewerB независимо подтвердил28/43 (65,12%), FAIL0, NOT_RUN15, verdict
NOT ACCEPTED. Прежние замечания C03–C05 закрыты; C02 отдельно закрыт A.

Незавершённые группы поB: R01.1–4, R07.3, R09.3–4, R12.1–8. Отсутствие
оставшихся обнаруженных дефектов не заменяет эти обязательные критерии.
ReviewerA после исправления C02 подтвердил его закрытие; новый общий процентA
не вычислялся. Итоговые независимые verdict обоих reviewer ещё впереди.

## Второе ревью R01 после интеграции

- C06 (A и B, P2): broad recovery пропускал terminal head после отзыва доступа
  к одному чату и не пробуждал обработку gap/следующего разрешённого элемента.
  CLOSED независимо A и B: `TestRecoveryBroadRevokedGapDoesNotStrandAllowedTail`
  для all/direct; exact-scope revocation по-прежнему подавляет доставку.
- C07 (A, P3): control logs не связывали retries с эпизодом. CLOSED независимо A: keyed domain-separated control reference, retry/restart,
  reminder и связь двух log sinks с journal episode проверены race-тестами.
- C08 (B): rollback gate не учитывал recovery state. CLOSED независимо B: table-aware
  aggregate guard, отсутствие старых tables, terminal/orphan payload и read-only
  поведение проверены TestLegacyRollback*.

Проценты предыдущего ревью сохранены как исторические. Финальные verdict и
операционная приёмка не заменяются результатами отдельных синтетических тестов.

Последний предварительный verdict B после этих исправлений: 32/43 (74,42%),
NOT ACCEPTED, нерешённых конкретных дефектов B 0. R01 operational route,
установленная identity, saved host workflow и release gates остаются незавершёнными.
A отдельно закрыл C07 и побайтно сверил установленные skills с исходниками.
