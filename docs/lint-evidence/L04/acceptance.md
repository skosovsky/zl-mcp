# L04 acceptance

Scope: `refactor(lint): simplify orchestration and remove unused code` from the approved lint plan. Remaining L05 style/constants/formatting findings do not constitute L04 acceptance. Full lint and final plan acceptance remain FAIL/OPEN until L05–L06 finish.

Candidate: code hash `681c72d8ecaa7d750bff1f15e04e620b4eb0fc73914b49a01a8f4280a00c392e`; exact 195 Go files and parent commit are recorded in `candidate.json`. Documentation/review artifacts are outside this code hash. Existing user Go 1.27.2/config/Makefile changes remain preserved and outside the L04 commit scope.

| Leaf | Requirement | Evidence |
|---|---|---|
| C01 | Parsing, validation, execution and grading are separate; single effects and transaction ordering remain unchanged. | Root/evaluation/service/collector/history/storage/native tests and race; SDK invocation and idempotency contracts. |
| C02 | Owned/borrowed buffers retain Clear, cancellation, lock, join and process Wait lifetimes; primary errors survive cleanup. | Root/mobilebackup/history/service/tools race and watchdog tests. |
| C03 | Wire keys/order/null/empty, signed cursors, fingerprints and persisted formats remain unchanged. | Golden cursor bytes, storage identity/hash tests, SDK ID tests, standalone offline suite. |
| C04 | Unused private helpers are removed after build-tag search; Preview is private; shadowing uses meaningful names. | Full diagnostics with successful loader, all production AST imports, root compile/platform checks retained for final L06 acceptance. |
| C05 | All L04 complexity/shadow/unused groups close without config/threshold/exclusion changes; only specific explained local exceptions. | Fresh full root/tools diagnostics including multiple messages per line; nolintlint; unchanged initial config checksum. |
| C06 | Architecture checks active and hidden graphs with exact config and valid negative controls; loader error is not a PASS. | `make architecture`, 14 isolated and 6 source controls. |
| C07 | Existing user edits/evidence/history survive; no siblings, third_party, deployment, live accounts or paid evaluation changes. | Git diff scope and preserved initial diff/checksums; model/live NOT_RUN. |
| C08 | Full root/tools relevant tests/race/vet and standalone offline acceptance pass before commit. | Checks below; post-commit compilation/relevant tests required by plan. |

## Checks

PASS: full root/tools race; root/tools vet exit 0; architecture host/hidden 100%, 14 isolated + 6 source controls; standalone offline 50/50 cases and 480/480 assertions. Raw logs: `/tmp/zl-lint-L04-accept-root-race.log`, `accept-tools-race.log`, `accept-root-vet.log`, `accept-tools-vet.log`, `accept-architecture-offline.log` (all with the same `/tmp/zl-lint-L04-` prefix). After reviewer fixes: `/tmp/zl-lint-L04-review-fixes-race.log` (zalo/service/storage/mcpserver), `/tmp/zl-lint-L04-no-callback-race.log`, `/tmp/zl-lint-L04-reviewed-root-vet.log`; all exit 0.

Final full diagnostics: root 1408, tools 165; loaders successful. Every L04 complexity/shadow/unused group and nolintlint is 0. L05 style findings remain OPEN. Exact diagnostics are retained beside this report. The config checksum remains `dec18ea507979a4c6f58ce2f47a49f784fdbd91615682fe23182057996640335`.

Earlier failed compile/load runs and reviewer candidate failures remain history and do not count as acceptance. Review found and fixed an errors.As autofix regression in bounded listener diagnostics; hostile callbacks and nested schema depth have dedicated passing tests. Meaningless nested error variable names were replaced by operation names.

Performance/provenance Go 1.27.2, mandatory pinned lint/CI and final three platform/native/patched-zcago acceptance belong to L06 and remain OPEN/NOT_RUN. Historical budgets/evidence are unchanged. Model/live NOT_RUN.

## Independent review

Completeness **100% (8/8)** and correctness **PASS** from independent reviewers `l04_completeness` and `l04_correctness` for candidate `681c72d8ecaa7d750bff1f15e04e620b4eb0fc73914b49a01a8f4280a00c392e`, 195/195 file checksums verified. Initial review: completeness 87.5% (ambiguous generated error names), correctness FAIL (errors.As diagnostic regression). Both findings were fixed and reviewers reran against the updated candidate; no blocking findings remain. This accepts L04 only; overall lint acceptance remains OPEN for L05–L06.
