# Development

Use the Go version declared in `go.mod`. The first publication provides source; build from a complete checkout, including `third_party/zcago`. The local dependency replacement means `go install ...@version` is not the supported installation path.

```sh
go build -o bin/zl-mcp ./cmd/zl-mcp
```

The production executable is a Go binary. Python scripts are optional development tooling and are not needed to run the service. Building a checkout does not update an installed binary or restart its LaunchAgent.

## Contracts and tests

Change executable contracts before changing their implementation. `docs/contracts` is a Go package that embeds JSON schemas at build time; preserve it when reorganizing documentation. Tests should use Arrange–Act–Assert and synthetic data.

Run the application checks from the repository root:

```sh
go test ./...
go test -race ./...
go vet ./...
```

The included zcago source has its own Go module. Root checks do not traverse that module; check it separately:

```sh
cd third_party/zcago
go test ./...
go test -race ./...
go vet ./...
```

CI runs these checks on Linux and native macOS, and builds without CGO for macOS arm64, macOS amd64, and Linux amd64. A cross-build checks compilation; it does not establish service installation support on that platform.

The test suite uses temporary directories, synthetic upstreams, and local HTTP/TLS receivers. It covers search and context, contract validation, permissions and locking, service shutdown and restart, authentication-required state, event delivery retries, subscription boundaries, cancellation, and log rotation. These checks do not require Zalo credentials or an active account and do not run model evals.

Conversation tests cover direct/group identity collisions, incoming/self normalization, mixed replay, discovery, policy reduction, migration boundaries, legacy and versioned Events profiles and typed full-text resources. The bounded socket/listener queues apply cancellable backpressure to message data rather than evicting it; synthetic burst tests cover this path.

Direct-messaging tests cover incoming/outgoing filters, first locally known incoming messages, unknown migration history, retained quote metadata, send permissions, concurrent request claims and ambiguous send recovery. The service integration test exercises quoted sending through HTTP MCP using the collector's session and verifies that an exact repeat after restart returns the stored result without another upstream send. These synthetic checks do not establish live Zalo acceptance or discovery in ChatGPT; record those separately. See the [task and acceptance requirements](task-direct-messaging.md) and [executable contract](contracts/direct-messaging.md).

Run the focused checks while changing this extension:

```sh
go test -race ./internal/messaging ./internal/storage ./internal/events ./internal/service ./internal/mcpserver ./docs/contracts
```

Run the isolated corpus load check separately:

```sh
go test ./internal/storage -run '^$' -bench BenchmarkManyConversations -benchtime=3x -count=1
```

It persists 8,000 synthetic messages in 4,000 conversations through the production Put path and checks catalogue pagination, aggregate status, search with coverage and context. Report timings as measurements of the machine and build, not as a fixed SLA. No live Zalo account or installed state is used.

The synthetic fixture command and `docs/evals/fixtures.json` support isolated development. Some unit tests also read the project skill files, so keep these fixtures and references when removing generated evaluation output. Optional model-evaluation scripts require a separate explicit run; they are not CI acceptance checks.

## Dependency maintenance

See [third-party notices](../THIRD_PARTY.md) and [zcago patch notes](../third_party/zcago/PATCHES.md). Keep the upstream commit, local source, patch explanation, and regression tests synchronized. Verify quote IDs above JavaScript's integer precision limit, HTTP 401 versus other statuses, login error envelopes, and cancellation without an HTTP response before replacing patched zcago.

## Public changes

Commit source, contracts, synthetic fixtures, and documentation. Keep sessions, tokens, real configuration, account data, SQLite databases, QR images, logs, generated traces, build outputs, and Python caches outside the public tree. Use example paths and placeholder identifiers in documentation.

Before publication, validate a fresh checkout with the same application and nested-module checks used by CI. Live-account operations, installed-service changes, and model evals are separate from these checks.

## Secret scanning

Before publication, scan the complete Git history with `gitleaks git --config .gitleaks.toml --redact .`. The configuration extends default rules and excludes only the public upstream protocol constant and fixed synthetic regression key in their specific source files. Never add an account credential to these exceptions.

Webhook tests construct synthetic signing keys from test bytes at runtime. Avoid
embedding complete `whsec_` tokens in fixtures: GitHub can classify Standard Webhooks
test values as Stripe signing secrets. Inspect an alert's source and provenance
before resolving it as a false positive; real exposed credentials require revocation.

## Historical import checks

History operations have shared flat executable schemas and three explicit MCP
names. Update the schema alias map with registration; validate both HTTP and
STDIO discovery. The worker uses the collector's guarded session under the
existing account lock. Synthetic service tests prove one restore/listener,
readable imported records, stable request retries and an empty Events queue.

```sh
go test -race ./internal/historyimport ./internal/storage ./internal/service ./internal/mcpserver ./docs/contracts
```

Regressions cover exact decimal cursors, whole-page rollback, checkpoint/restart,
auth pause, cancellation of late responses, account/policy revocation, durable
work limits and retention-safe silent identities. All Message output schemas
accept `source=history`; catalogue metadata provenance remains a separate enum.
These checks do not establish live source availability or direct history.

History failure diagnostics log fixed categories and optional numeric API codes.
Tests deliberately put a private marker in the underlying error and require its
absence from logs and the exported safe error. Original causes remain available
for typed authentication/cancellation handling; never print them in production.

The owner-only `probe-preload` command is for bounded source verification through
an already running service. It emits counts/safe categories and never imports
records. It cannot use arbitrary endpoints or bypass the account guard. This
command is separate from MCP tools and requires both CLI and service versions
supporting its executable private contract. Do not run a second login/listener
for source investigation. The command does not prove read-effect absence,
Strangers catalogue completeness or old-history recovery.

The mobile backup candidate has a trusted local attempt ledger, not an exposed
mobile-history MCP source. See [mobile diagnostics](mobile-backup-diagnostics.md)
for preparation/status/prepared-only cancellation. These commands do not authorize
or execute a phone request and require the already running single service.

## Native startup acceptance

To exercise the built service executable rather than an injected Go service
function, set an absolute path to the verified native binary:

```sh
ZL_MCP_ACCEPTANCE_BINARY="$HOME/.local/bin/zl-mcp" \
  go test -race ./internal/mobilebackup -run TestNativeBinaryTerminalStartupAcceptance -count=1
```

The test creates a short private temporary state directory, a synthetic
account-bound terminal history operation and encrypted snapshot, then starts
that binary with a separate loopback endpoint and temporary HOME. It checks
startup removal before authentication, unchanged terminal state, no imported
messages or Events, retained private key/spent UUID and graceful shutdown.
It neither uses the installed state/session nor requests phone synchronization.
Without the explicit environment variable this subprocess test is skipped;
ordinary in-process regression tests still run. CI explicitly builds and tests a
native binary on both Linux and macOS runners. Run it with a binary built for
the current host. It does not establish real mobile archive eligibility or
successful import of private history.
