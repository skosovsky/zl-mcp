# Executable architecture gates (T02)

`arch-go.yml` is JSON syntax accepted as YAML by arch-go/v2 v2.1.2. Thresholds
are compliance100% and coverage100%. Every current production/support package has
its own meaningful dependency allowlist; no generic package catch-all is used.
Existing backend imports are frozen individually. The normative new boundaries
are domain; each app capability/shared; store/operations adapters; toolruntime;
observability; evaluation and command composition. Future capability packages have
separate own/domain/shared imports, no third-party imports and no SQL/network/OS ports.

Rules correspond to A01–A09 in the task. SDK remains in MCP wire adapters, the existing
service/bridge.go and isolated evaluation support. A supplemental Go AST import scan
restricts the service SDK exception to that exact bridge file. All production source
imports are parsed regardless of platform, custom build tags or CGO; nested modules,
testdata and test files are outside the production module scan. The same allowlists
also check these otherwise host-invisible files and reject uncovered source packages. `go list -deps` independently
rejects evaluation/evalfixture/evaly in the real service executable. Supplemental graph
checks ensure toolsy is only in toolruntime, metry/OTel only in observability/explicit
composition, pure app imports and exact MCP/CLI migration exceptions. Root and subpackage
patterns are exercised by positive controls; unknown/root packages fail coverage.

T07 closes all exact temporary edges: MCP imports only typed app/runtime contracts
and SDK/wire dependencies; it constructs no storage/control/native owner. SQL
no-row errors become the same safe domain error in backend adapters. The CLI
entrypoint calls service composition, which owns login, logger, control client and
private-state wiring with unchanged local authority/validation. The exception
manifest is empty; native subsystem internal edges remain deliberately retained.
The checker reads the source-controlled manifest, not an acceptance report.

Ten isolated controls run the same pinned executable: domain->SQL, app->SDK,
mcpserver->storage, production->evaluation, domain subpackage->SQL, uncovered package,
uncovered root and shadow-path packages, and positive app root/subpackages.
Four additional source controls reject platform/tag-hidden service SDK imports,
CGO-constrained domain SQL and a platform-only uncovered package. The bare normative fixture has
no legacy implementation, so its copy of A03 omits documented temporary edges; negative
controls must fail on the specified dependency/coverage rule, never just a loader error.
Active repository rules enforce A03/A09 without temporary exceptions. Negative fixture
results and reasons are retained in evidence/T02/architecture.json.

Pinned syntax details are characterized: empty shouldOnlyDependsOn lists do not forbid
imports; no-external packages have an explicit deny regex `.+`. Standalone `**` is not a
valid pinned matcher. All package and dependency selectors use anchored full import
paths with escaped dots; this prevents substring matches from granting false coverage. The pinned checker classifies golang.org/x as standard, so existing unix and Unicode normalization imports appear explicitly in their standard allowlists as well as the
production graph. This does not authorize another dependency. The supplemental graph
uses actual Go import paths and catches app infrastructure regardless of that heuristic.
The executable prints "master" via --version; its Go build module info must identify
arch-go/v2 v2.1.2. Neither a generic allow-all nor a reduced threshold fixes a failure.

Static imports do not prove runtime operation ownership, no resend, permissions,
persistence compatibility, response budgets or telemetry privacy. Their behavioral
checks remain mandatory in later tasks. Model/live execution stays NOT_RUN.

T08 adds one explicit observability -> app/shared edge for the consumer-owned
TelemetryPort and Invocation types. The adapter imports no other app capability,
transport, SQL or native owner. metry/OTel remain inside observability; service
constructs its one owned provider through the host factory.

The native messaging/collector/historyimport/Events packages and service gain
only the explicit app/shared observation-port edge. Their existing native
DB/session owners remain intact. Config gains exact standard errors/net-url/strings
imports to validate private opt-in settings and return neutral errors.

T08 review correction: MCP final-call audit imports only standard crypto/rand to
create unrelated random correlation IDs. This exact edge retains no raw caller
or native identity and introduces no business/SDK dependency.
