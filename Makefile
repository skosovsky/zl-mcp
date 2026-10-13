ARCH_GO_VERSION := v2.1.2
TOOLS_BIN ?= $(CURDIR)/.tools
TOOLS_ABS := $(if $(filter /%,$(TOOLS_BIN)),$(TOOLS_BIN),$(CURDIR)/$(TOOLS_BIN))
ARCH_GO := $(TOOLS_ABS)/arch-go
HARNESS := $(TOOLS_ABS)/zl-mcp-harness
ARCHITECTURE_EVIDENCE ?=

.PHONY: tools architecture test race vet nested-test nested-race nested-vet build-platforms check

tools:
	mkdir -p "$(TOOLS_BIN)"
	GOWORK=off GOBIN="$(TOOLS_ABS)" go install github.com/arch-go/arch-go/v2@$(ARCH_GO_VERSION)

harness:
	mkdir -p "$(TOOLS_BIN)"
	cd tools/harness && GOWORK=off go build -trimpath -o "$(HARNESS)" ./cmd/harness

architecture: tools harness
	GOWORK=off "$(HARNESS)" architecture --root "$(CURDIR)" --binary "$(ARCH_GO)" $(if $(ARCHITECTURE_EVIDENCE),--evidence "$(ARCHITECTURE_EVIDENCE)",)

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

# Standalone synthetic service contracts are mandatory; migration parity is complete.
check: tools-test tools-race tools-vet test race vet nested-test nested-race nested-vet architecture build-platforms eval-offline

EVAL_OUTPUT ?= /tmp/zl-mcp-offline-$$(date +%s)-$$$$
.PHONY: eval-offline
eval-offline:
	GOWORK=off go run ./cmd/zl-mcp-eval --root "$(CURDIR)" --profile offline --output "$(EVAL_OUTPUT)" --run-budget 30m --repeats 2

.PHONY: bench-mcp
BENCH_CPU_MODEL ?=
bench-mcp: harness
	set -eu; task_bench_root=$$(mktemp -d /tmp/zl-mcp-bench.XXXXXX); \
	GOWORK=off "$(HARNESS)" current-build --root "$(CURDIR)" --output "$$task_bench_root/build" && \
	"$(HARNESS)" bench-mcp --root "$(CURDIR)" --binary "$$task_bench_root/build/zl-mcp-eval-candidate" $(if $(BENCH_CPU_MODEL),--cpu-model "$(BENCH_CPU_MODEL)",) --build-manifest "$$task_bench_root/build/build-manifest.json" --output "$$task_bench_root/results"

.PHONY: performance-acceptance
PERFORMANCE_BASELINE ?= docs/mcp-migration/evidence/T03/performance-baseline/manifest.json
PERFORMANCE_BASELINE_BUILD ?= docs/mcp-migration/evidence/T03/baseline-build-manifest-v2.json
PERFORMANCE_CANDIDATE ?=
PERFORMANCE_REPORT ?= /tmp/zl-mcp-performance-comparison.json
performance-acceptance: harness
	"$(HARNESS)" compare-performance --baseline "$(PERFORMANCE_BASELINE)" --candidate "$(PERFORMANCE_CANDIDATE)" --baseline-build "$(PERFORMANCE_BASELINE_BUILD)" --output "$(PERFORMANCE_REPORT)"

.PHONY: eval-model

# A missing separately authorized driver produces NOT_RUN and a nonzero exit.
# This opt-in target is excluded from check and CI.
MODEL_DRIVER ?=
eval-model:
	GOWORK=off go run ./cmd/zl-mcp-eval --root "$(CURDIR)" --profile model --model-driver "$(MODEL_DRIVER)" --output "$(EVAL_OUTPUT)" --run-budget 30m

.PHONY: harness tools-test tools-race tools-vet
tools-test: tools
	cd tools/harness && ARCH_GO_BINARY="$(ARCH_GO)" GOWORK=off go test ./...
tools-race: tools
	cd tools/harness && ARCH_GO_BINARY="$(ARCH_GO)" GOWORK=off go test -race ./...
tools-vet: tools
	cd tools/harness && GOWORK=off go vet ./...
