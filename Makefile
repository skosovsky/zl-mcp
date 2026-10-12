ARCH_GO_VERSION := v2.1.2
TOOLS_BIN ?= $(CURDIR)/.tools
ARCH_GO := $(TOOLS_BIN)/arch-go
ARCHITECTURE_EVIDENCE ?=

.PHONY: tools architecture dependency-probe test race vet nested-test nested-race nested-vet build-platforms check

tools:
	mkdir -p "$(TOOLS_BIN)"
	GOWORK=off GOBIN="$(TOOLS_BIN)" go install github.com/arch-go/arch-go/v2@$(ARCH_GO_VERSION)

architecture: tools
	GOWORK=off python3 scripts/mcp-migration/architecture.py --binary "$(ARCH_GO)" $(if $(ARCHITECTURE_EVIDENCE),--evidence "$(ARCHITECTURE_EVIDENCE)",)

dependency-probe:
	GOWORK=off python3 scripts/mcp-migration/dependency_probe.py /tmp/zl-mcp-consumer-probe.json

test:
	go test ./...
race:
	go test -race ./...
vet:
	go vet ./...
nested-test:
	cd third_party/zcago && go test ./...
nested-race:
	cd third_party/zcago && go test -race ./...
nested-vet:
	cd third_party/zcago && go vet ./...
build-platforms:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o bin/zl-mcp-darwin-arm64 ./cmd/zl-mcp
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -o bin/zl-mcp-darwin-amd64 ./cmd/zl-mcp
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/zl-mcp-linux-amd64 ./cmd/zl-mcp

# Full synthetic offline parity is mandatory; performance uses the recorded host baseline.
check: test race vet nested-test nested-race nested-vet architecture build-platforms eval-offline eval-paired

EVAL_OUTPUT ?= /tmp/zl-mcp-offline-$$(date +%s)-$$$$
.PHONY: eval-offline
eval-offline:
	GOWORK=off go run ./cmd/zl-mcp-eval --root "$(CURDIR)" --profile offline --output "$(EVAL_OUTPUT)" --run-budget 30m

.PHONY: bench-mcp
BENCH_ROLE ?= candidate
BENCH_CPU_MODEL ?=
bench-mcp:
	set -eu; task_bench_root=$$(mktemp -d /tmp/zl-mcp-bench.XXXXXX); \
	GOWORK=off python3 scripts/mcp-migration/build_eval.py --output "$$task_bench_root/build" && \
	python3 scripts/mcp-migration/bench_mcp.py --binary "$$task_bench_root/build/zl-mcp-eval-$(BENCH_ROLE)" $(if $(BENCH_CPU_MODEL),--cpu-model "$(BENCH_CPU_MODEL)",) --build-manifest "$$task_bench_root/build/build-manifest.json" --output "$$task_bench_root/results"

.PHONY: eval-paired performance-acceptance
eval-paired:
	set -eu; task_pair_root=$$(mktemp -d /tmp/zl-mcp-paired.XXXXXX); \
	GOWORK=off python3 scripts/mcp-migration/build_eval.py --output "$$task_pair_root/build"; \
	"$$task_pair_root/build/zl-mcp-eval-candidate" --root "$(CURDIR)" --baseline-binary "$$task_pair_root/build/zl-mcp-eval-baseline" --output "$$task_pair_root/paired" --repeats 2

PERFORMANCE_BASELINE ?= docs/mcp-migration/evidence/T03/performance-baseline/manifest.json
PERFORMANCE_BASELINE_BUILD ?= docs/mcp-migration/evidence/T03/baseline-build-manifest-v2.json
PERFORMANCE_CANDIDATE ?=
PERFORMANCE_REPORT ?= /tmp/zl-mcp-performance-comparison.json
performance-acceptance:
	python3 scripts/mcp-migration/compare_performance.py --baseline "$(PERFORMANCE_BASELINE)" --candidate "$(PERFORMANCE_CANDIDATE)" --baseline-build "$(PERFORMANCE_BASELINE_BUILD)" --output "$(PERFORMANCE_REPORT)"
