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

The synthetic fixture command and `docs/evals/fixtures.json` support isolated development. Some unit tests also read the project skill files, so keep these fixtures and references when removing generated evaluation output. Optional model-evaluation scripts require a separate explicit run; they are not CI acceptance checks.

## Dependency maintenance

See [third-party notices](../THIRD_PARTY.md) and [zcago patch notes](../third_party/zcago/PATCHES.md). Keep the upstream commit, local source, patch explanation, and regression tests synchronized. Verify quote IDs above JavaScript's integer precision limit, HTTP 401 versus other statuses, login error envelopes, and cancellation without an HTTP response before replacing patched zcago.

## Public changes

Commit source, contracts, synthetic fixtures, and documentation. Keep sessions, tokens, real configuration, account data, SQLite databases, QR images, logs, generated traces, build outputs, and Python caches outside the public tree. Use example paths and placeholder identifiers in documentation.

Before publication, validate a fresh checkout with the same application and nested-module checks used by CI. Live-account operations, installed-service changes, and model evals are separate from these checks.

## Secret scanning

Before publication, scan the complete Git history with `gitleaks git --config .gitleaks.toml --redact .`. The configuration extends default rules and excludes only the public upstream protocol constant and fixed synthetic regression key in their specific source files. Never add an account credential to these exceptions.
