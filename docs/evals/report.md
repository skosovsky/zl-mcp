# Historical verification summary

These results describe the MVP and skills evaluated on 2026-10-01. They do not certify the current service or the subsequently updated research and event-handling skills. No new model evaluations were performed for the skill update.

## Synthetic model evaluations

The recorded aggregate comprises 65 accepted runs: 45 scenario/follow-up phases and 20 skill-selection cases, using gpt-5.5 with low reasoning through Codex CLI 0.137.0. All 65 recorded checkpoints and answer reviews passed. The fixture used synthetic messages and a fake upstream; it did not access a live Zalo session. Selection cases measured skill selection rather than full task completion.

The runs made 244 tool calls with seven errors: three intentional argument errors and four recovered errors. Usage was 2,520,242 input tokens (1,906,688 cached), 36,408 output tokens and 4,608 reasoning output tokens. Baseline and skill comparisons had the same final pass rate; the skill increased calls and input tokens. These small, model-specific samples do not establish general superiority or reliability.

Aggregate counts and comparison metrics are retained in [results-index.json](results-index.json). Raw prompts, answers and traces are excluded from publication. Consequently the public summary preserves historical measurements, but cannot independently reproduce the original answer reviews. Native skill installation and automatic discovery were not evaluated.

## Implementation and live checks

Historical implementation checks covered contracts, allowlist and resource access, FTS/search/context, pagination, approvals, idempotency, persistence and failure recovery. The [MVP summary](../archive/completion-audit.md) and [unified-service summary](../archive/unified-service-completion-audit.md) distinguish synthetic tests from authorized live observations.

A historical 100,000-message synthetic search benchmark recorded p95 121.549625 ms on the original implementation and 113.459583 ms after the unified-service migration. Both measured warmed first-page queries on a local macOS arm64 environment and are not hardware-independent guarantees.

Current acceptance must rerun the repository's relevant unit, contract, integration, race, static-analysis and build checks. Historical live observations do not prove complete upstream history, indefinite replay, behavior during Mac sleep, production agent connectivity or actual reboot recovery.
