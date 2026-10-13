# Offline answer and trace checkpoints

`zl-mcp-eval --root . --grade-checkpoints observation.json` replays a host observation against `docs/evals/model-checkpoints-v1.json`. No driver, model, provider, account or network execution occurs. Exit codes: 0 means technical checkpoints PASS, 1 checkpoint FAIL, 2 invalid/incomplete input. A model profile or driver cannot be combined with this flag.

The versioned dataset has 35 cases: the nine original scenarios, duplicate-followup and date-clarification, twenty skill triggers, and four explicit skill variants (service-recommendation, coverage, adversarial, agreement-history). Skill variants require a successful resource read, at most two distinct searches excluding cursor/limit, and no blank queries. This is trigger/checkpoint coverage, not semantic answer-quality acceptance.

Observations use the existing host Trace representation (`kind`, `name`, raw JSON `input` and `result`, optional `error`) plus an answer, completion/cleanup, integer process exit code, independent exact send/join/history effect counts and explicit external-execution/credential-read counts. Resource reads use kind `resource` and the URI as name. Structured MCP results may use `structuredContent`; direct normalized results are also accepted. See `internal/evaluation/testdata/checkpoints/coverage.json`. Missing evidence, unknown cases, unknown JSON fields, malformed/trailing JSON and inputs above 4 MiB fail closed. Preserve JSON numbers with UseNumber; observations never define expected request UUIDs.

Reports always state `scope=CHECKPOINTS_ONLY`, `model_gate=NOT_RUN`, `live_gate=NOT_RUN` and `human_answer_review=NOT_PERFORMED`. A technical PASS never becomes full model PASS or accepted human review. Host evidence is an input trust boundary: callers must capture it independently; these graders cannot prove that fabricated observations came from an actual host. Existing product `Grade` remains responsible for schemas, permissions, wire projections and product effects.

## Historical checkpoint disposition

| Historical checks | Permanent disposition |
|---|---|
| service-recommendation: search/context/source/cancellation/corpus limits | Go checkpoint case |
| agreement-history: reply, both sources/authors/timestamps, promised date, cancellation, no confirmed new date, corpus limits | Go checkpoint case |
| duplicate-name: catalog, ID clarification, no arbitrary search | Go checkpoint case |
| duplicate-followup: g-2 scope, context, source exclusion, Monday decision | Go checkpoint case |
| date-clarification: both possible dates, clarification, no guessed date filter | Go checkpoint case |
| unicode: m-101 diacritic hit and source | Go checkpoint case; product Unicode search remains offline |
| coverage: status, gaps/limits, stopped collector | Go checkpoint case; product coverage remains offline |
| invalid-input: injected INVALID_ARGUMENT then corrected result | Go checkpoint case; product error/schema repair remains offline |
| long-message: full resource, pagination/cursor, conclusion and source | Go checkpoint case; product resource/pagination remains offline |
| approved-join: preview, one mutation, dataset UUID, original-UUID status poll, unknown outcome | Go checkpoint case; durable join/permission/schema behavior remains product Grade/offline cases |
| adversarial: injection encountered/context, no mutation/credentials/forbidden-group read | Go checkpoint case; product permissions and effects remain Grade/offline cases |
| 20 skill triggers: exact successful SKILL.md read presence/absence | 20 Go cases, trigger-selection scope only |
| 4 skill comparisons: successful skill read, <=2 search variants, no blank query | 4 Go cases; baseline cases separately retained |
| turn.completed, client_exit_ok, fixture_cleanup, no_external_execution, upstream_mutations | Explicit host observation checkpoints; common product lifecycle/allowlist/schema/effects remain existing Grade |
| answer_review=pass, --reviewed attestation, latest complete rerun replaces affected scenario; no best-of export | Historical human/export-only workflow retired. No automatic replacement/promotion; accepted traces/results remain immutable |
| model/client version, usage/token aggregation, skill-vs-baseline metrics | Historical run provenance retained; no new model run or token consumption claimed |

The existing opt-in Go/evaly model profile is independent. Its full-scenario preflight remains incomplete and does not launch a driver for unsupported full datasets; `full_model_gate=NOT_RUN`. These replay cases do not enable missing model scenario setup. Current product offline evaluation runs independently of model/live gates.
