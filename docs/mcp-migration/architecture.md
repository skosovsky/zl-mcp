# Permanent executable architecture gates

`make architecture` runs pinned arch-go/v2 v2.1.2 and the separate Go module
`tools/harness`. `arch-go.yml` is ordinary YAML with compliance and coverage
thresholds of 100%. It has 33 package selectors: compatible allow/deny rules
share a selector, while capability and existing backend boundaries remain explicit.
There is no generic production package catch-all.

arch-go is the only dependency-pattern matcher. The harness never reimplements
its allowlists with a separate regexp engine. Root/subpackage patterns use the
pinned syntax `^github[.]com/skosovsky/zl-mcp/internal/domain.**`; exact dependencies
use patterns such as `^net$`. Anchoring excludes shadow paths and neighboring names.
Empty allowlists do not deny imports in this version, so external bans retain an
explicit `.+` deny. `golang.org/x` is classified as standard by the pinned checker.
The executable's Go module build info must identify v2.1.2; its displayed `master`
version is not used for the pin check.

Two reports are required. The first checks normal host-visible production imports.
The second checks a temporary synthetic module whose package imports are the union
of every production Go file, parsed with Go AST regardless of platform suffix,
build tags or CGO. Both runs use the same executable and the exact config bytes.
Nested modules, hidden directories, vendor, testdata and test files are excluded
from the production module scan. Malformed sources, process failures, missing or
stale reports and truncated diagnostics fail the gate. Synthetic source files are
not compiled; arch-go checks their imports, including otherwise host-hidden edges.

Two additional constraints cannot be expressed by package allowlists: SDK imports
in service are restricted to `internal/service/bridge.go`, and `go list -deps`
rejects evaluation/evalfixture/evaly in the actual production executable. SDK,
toolsy, metry and app infrastructure package restrictions otherwise live only in
arch-go configuration. The source-controlled architecture-exceptions manifest must
remain present with an empty exceptions array; temporary edges cannot silently
bypass these gates.

Ten isolated controls check domain SQL, app SDK, MCP storage, production evaluation,
domain subpackages, unknown/root/shadow packages and positive app root/subpackages.
They receive a byte-for-byte copy of the active config, with no allowlist edits.
A negative dependency control must produce a compliance failure naming the injected
import; a coverage control must fail coverage. A loader error or missing report
never counts as successful detection. A regression test deliberately allows MCP
storage and verifies that the negative control itself fails, independently of the
production graph check.

The four historical source controls remain: platform/tag-hidden service SDK,
CGO-constrained domain SQL and platform-only uncovered package. Two positive hidden
source controls ensure that allowed Windows/tagged imports are accepted. All source
controls exercise the same AST scanner and arch-go matcher. Go integration tests
compare old and new config decisions for current production imports and positive/
negative fixtures. The old JSON config is test-only evidence, not a runtime fallback.

`make tools-test tools-race tools-vet` checks the separate module with `GOWORK=off`;
these checks are mandatory in `make check` and CI. Architecture subprocesses have
bounded diagnostics, deadlines, owned process groups and bounded waits; cleanup
terminates owned descendants on both successful completion and cancellation.
`ARCHITECTURE_EVIDENCE` optionally records both reports, imports, production dependency
graph, controls and failure diagnostics as JSON.

A01–A09 remain the normative package boundaries: domain purity, independent app
capabilities, MCP ports, toolruntime ownership of toolsy, observability ownership of
metry/OTel, isolated evaluation, limited SDK and service composition. The existing
service bridge and native owner imports are intentional contracts. Static imports
do not prove permissions, operation ownership, absence of resend, persistence,
response budgets or telemetry privacy; standalone synthetic behavioral checks remain
mandatory. Migration acceptance reports are historical records. Model/live execution
remains NOT_RUN unless separately authorized.
