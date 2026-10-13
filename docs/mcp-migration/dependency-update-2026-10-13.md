# Dependency update 2026-10-13

User-authorized scoped update after accepted migration HEAD `518f7e9`:

- toolsy `v0.20.6`, origin `e540be6b5abf6e6344f3ba038afe211bc2916163`.
- `github.com/modelcontextprotocol/go-sdk` stays required at `v1.8.0`; remote replacement is `github.com/skosovsky/go-sdk v1.8.1-0.20261013081052-4a7b89fe086f`, origin `4a7b89fe086f3afdf9ea15b095bbcd83b3061122`.
- `go mod tidy` changed only these two version/checksum pairs; `go mod verify` PASS.
- Existing cleanup edits remain in the working tree. No sibling library or third_party edits.

Historical checks completed before harness retirement:

- Root `go test ./...`: PASS, including SDK01 identity/lifecycle tests.
- Race: `internal/mcpserver`, `internal/toolruntime`, `internal/service`, `internal/adapters/store`: PASS.
- Root `go vet ./...`: PASS.
- Architecture: 100% compliance/coverage, production graph, ten isolated and all-build source controls: PASS. The reviewed negative-control issue was subsequently fixed by H03 of harness retirement.
- Unmodified SDK module `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`: PASS.
- CGO-free builds darwin/arm64, darwin/amd64, linux/amd64: PASS.
- Standalone offline and paired baseline/candidate evaluation, two repeats each: PASS, 50/50 cases and 480/480 assertions per report.
- toolsy isolated module tests/race/vet: PASS with `GOWORK=off`. Initial workspace-mode attempt failed because the published go.work references omitted nested modules; no library files were changed.
- Fresh five-run performance (15,000 measured calls, zero errors): PASS, all nine gates. Median-of-run p95: Read 6.517458ms, Search 6.497125ms, Context 6.212375ms. Existing fixed gates remain objects <=2x frozen baseline, bytes <=1.20x, p95 Read 7.8683ms, Search 7.0726337ms, Context 6.9967799ms.

Preserved build manifest, offline/paired reports, canonical parity and pre-initialize wire evidence: [evidence/dependency-update-2026-10-13](evidence/dependency-update-2026-10-13/). Full temporary driver captures: `/tmp/zl-dependency-update-4a7b89/`. All 15 raw performance records and the comparison are preserved in the same evidence directory. Individual-run latency excursions are retained; the contracted gate uses median-of-five run p95, not a per-run ceiling. First run overlapped the end of isolated toolsy checks; later runs were not overlapped with our builds/tests. Existing user applications were left running. PASS under revised p95 budgets does not establish original-baseline latency parity.
Historical SDK-01/T09/T10 evidence and acceptance remain unchanged and describe their original pins.
Model/live gates: **NOT_RUN**. No deployment, LaunchAgent restart, live account, paid model evaluation or publication.

## Accepted protocol correction

The new SDK changes the STDIO response to a legacy session request before initialization (for example `tools/list`): JSON-RPC error code `0` becomes `-32600` (Invalid Request), with the same rejection and no application dispatch. Old frozen fixture wire behavior was reproduced. The initialized offline suite does not cover this delta; the user explicitly accepted this correction on 2026-10-13, preserving all other parity requirements. HTTP stateless sessions synthesize initialization, so this specific branch does not apply there. No SDK compatibility opt-out has been enabled.

Independent final reviews: completeness 10/10 (100%), correctness PASS, no unresolved update findings. Receipts and artifact hashes are preserved in the evidence directory. The architecture negative-control issue is closed by H03; H06 final acceptance confirms the permanent gate.

## Permanent verification after harness retirement

The same production code and dependency pins passed the final H06 `make check` without Python: root/tools/zcago tests, race and vet, host/all-build architecture, three CGO-free platform builds and standalone offline evaluation (50/50 cases, 480/480 assertions). Current native startup/cleanup passed under race with two sequential starts. The fresh current-only performance run completed 15,000 calls without errors and passed all nine unchanged gates; median p95 Read/Search/Context was 6.926125/6.593125/6.659875 ms. See [H06 acceptance](evidence/harness-retirement/H06/acceptance.json) and its retained raw/provenance.

Frozen-binary comparison, paired CLI and Python runners are historical acceptance tools, retired from current Make/CI. Their results above remain unchanged. Current verification uses `make check` and the separate Go performance gate; model/live remain NOT_RUN.
