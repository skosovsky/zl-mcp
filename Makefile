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

# T03 adds mandatory offline eval; T09 adds parity/performance acceptance.
check: test race vet nested-test nested-race nested-vet architecture build-platforms
