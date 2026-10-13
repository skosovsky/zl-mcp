# Permanent service verification after migration

The architecture migration was accepted at `518f7e9af861e46a840bd71a7a410acaf43e05c6`. Its T01–T10/SDK-01 evidence, dependency-update evidence and rollback handoff are historical and remain unchanged. The user authorized retiring frozen-HEAD comparison from current CI on 2026-10-13.

## Disposition and implementation order

| Step | Retire | Preserve or transfer |
|---|---|---|
| H01 | Frozen baseline archive/overlay build, paired CLI and baseline corrections, migration candidate command and one-off consumer probe | Current standalone offline assertions, exact JSON/response/effect validation; current-only build provenance until H04 |
| H02 | Python CLI acceptance runner | Go CLI failure, timeout and lifecycle tests; existing Go/evaly offline, root/nested checks |
| H03 | Python architecture runner and temporary AST helper | Go tooling module, exact config controls, all-build source imports, SDK bridge restriction, production transitive graph |
| H04 | Python benchmark/watchdog/comparison tests and runner | Current-only Go builds, five runs x three kinds x 1000 calls, all raw/provenance and fixed budgets |
| H05 | Legacy Python Codex runner/exporter | Unique scenario/trigger/follow-up checks in Go graders and synthetic fixtures; model driver stays opt-in |
| H06 | HTTP transport interface in domain | Identical consumer-owned archive port and native lifecycle guarantees |

No product schema, persistence, authority or effect contract is retired. Old-vs-new capture equality and the rounded old-SDK exception are migration assertions; current precision must still be exact. Scenario IDs in `docs/evals/migration-v1.json` remain permanent synthetic service scenarios despite the historical filename. Legacy model scenario disposition is recorded when H05 is implemented; its runners remain until that coverage review passes.

## Reproduction of historical acceptance

Use a separate checkout of the immutable T10 commit above to run its documented migration tools, paired evaluation or frozen/native restart checks. Keep its exact dependencies and Go version. Do not mix current source into its baseline overlay. Historical Python requirements apply only to that historical checkout. Acceptance remains restricted offline; no deployment is implied.

## Gates and constraints

Each H01–H06 has one atomic commit after leaf requirements/candidate fingerprint, relevant checks and two independent reviews (completeness 100%, correctness PASS). Existing dirty cleanup/dependency work is preserved and not included automatically. Current root pins remain toolsy v0.20.6 / SDK fork 4a7b89fe086f.

Permanent checks do not execute model/live paths. Those gates remain NOT_RUN. No service installation, LaunchAgent restart, live account, paid model run, publication, sibling library edit or third_party edit is authorized.
