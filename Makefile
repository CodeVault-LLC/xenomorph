SHELL := /bin/bash

ROOT := $(CURDIR)
BIN_DIR := $(ROOT)/bin
MODULES := platform/client platform/services/gateway platform/shared
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
WEBSITE_DIR := $(ROOT)/platform/website

GO ?= go
BUN ?= bun
FIPS_MODULE ?= v1.0.0
GOPATH ?= $(shell $(GO) env GOPATH)
GOFMT ?= gofmt

GOBIN ?= $(shell $(GO) env GOBIN)
GOBIN := $(if $(strip $(GOBIN)),$(GOBIN),$(if $(strip $(GOPATH)),$(GOPATH)/bin,$(ROOT)/bin))
GOLANGCI_LINT ?= $(or $(shell command -v golangci-lint 2>/dev/null),$(GOBIN)/golangci-lint)
STATICCHECK ?= $(or $(shell command -v staticcheck 2>/dev/null),$(GOBIN)/staticcheck)
GOVULNCHECK ?= $(or $(shell command -v govulncheck 2>/dev/null),$(GOBIN)/govulncheck)
GOSEC ?= $(or $(shell command -v gosec 2>/dev/null),$(GOBIN)/gosec)

GATEWAY_DIR := $(ROOT)/platform/services/gateway
CLIENT_DIR := $(ROOT)/platform/client

.PHONY: help fmt fmt-check test test-race vet staticcheck govulncheck gosec tidy tidy-check wire-generate wire-generate-check build build-all build-gateway build-client run-gateway run-client clean lint install-tools web-install web-format-check web-lint web-typecheck web-build ci-go ci-web ci

help:
	@printf '%s\n' "Available targets:"
	@printf '%s\n' "  make fmt           Format Go source across every Go module"
	@printf '%s\n' "  make fmt-check     Verify Go source formatting"
	@printf '%s\n' "  make test          Run tests for every Go module"
	@printf '%s\n' "  make test-race     Run race-detector tests for every Go module"
	@printf '%s\n' "  make vet           Run go vet for every Go module"
	@printf '%s\n' "  make staticcheck   Run staticcheck for every Go module"
	@printf '%s\n' "  make govulncheck   Run govulncheck for every Go module"
	@printf '%s\n' "  make gosec         Run gosec for every Go module"
	@printf '%s\n' "  make tidy          Normalize module metadata across every Go module"
	@printf '%s\n' "  make wire-generate Generate XBP registries, codecs, vectors, and reference"
	@printf '%s\n' "  make tidy-check    Verify go mod tidy produces no changes"
	@printf '%s\n' "  make build         Build native gateway and client binaries"
	@printf '%s\n' "  make build FIPS_MODULE=v1.0.0  Build with the selected Go FIPS 140-3 module"
	@printf '%s\n' "  make build-all     Cross-compile gateway and client for Linux, macOS, and Windows"
	@printf '%s\n' "  make ci-go         Run the complete Go quality and build gate"
	@printf '%s\n' "  make ci-web        Run the complete website quality and build gate"
	@printf '%s\n' "  make install-tools Install pinned static-analysis tools into GOPATH/bin"
	@printf '%s\n' "  make ci            Run all repository CI checks"

fmt:
	@set -euo pipefail; \
	for module in $(MODULES); do \
		cd $(ROOT)/$$module; \
		while IFS= read -r -d '' file; do $(GOFMT) -w "$$file"; done < <(find . -name '*.go' -not -path '*/gen/go/*' -print0); \
	done

fmt-check:
	@set -euo pipefail; \
	for module in $(MODULES); do \
		cd $(ROOT)/$$module; \
		unformatted=$$(find . -name '*.go' -not -path '*/gen/go/*' -exec $(GOFMT) -l {} +); \
		if [[ -n "$$unformatted" ]]; then printf '%s\n' "$$unformatted"; exit 1; fi; \
	done

test:
	@set -euo pipefail; \
	for module in $(MODULES); do cd $(ROOT)/$$module && GOFIPS140=$(FIPS_MODULE) $(GO) test ./...; done

test-race:
	@set -euo pipefail; \
	for module in $(MODULES); do cd $(ROOT)/$$module && GOFIPS140=$(FIPS_MODULE) $(GO) test -race ./...; done

vet:
	@set -euo pipefail; \
	for module in $(MODULES); do cd $(ROOT)/$$module && GOFIPS140=$(FIPS_MODULE) $(GO) vet ./...; done

staticcheck:
	@set -euo pipefail; \
	if [[ ! -x $(STATICCHECK) ]]; then printf '%s\n' "staticcheck is not installed; run make install-tools"; exit 1; fi; \
	for module in $(MODULES); do cd $(ROOT)/$$module && $(STATICCHECK) ./...; done

govulncheck:
	@set -euo pipefail; \
	if [[ ! -x $(GOVULNCHECK) ]]; then printf '%s\n' "govulncheck is not installed; run make install-tools"; exit 1; fi; \
	for module in $(MODULES); do cd $(ROOT)/$$module && $(GOVULNCHECK) ./...; done

gosec:
	@set -euo pipefail; \
	if [[ ! -x $(GOSEC) ]]; then printf '%s\n' "gosec is not installed; run make install-tools"; exit 1; fi; \
	for module in $(MODULES); do cd $(ROOT)/$$module && $(GOSEC) -exclude-generated ./...; done

tidy:
	@set -euo pipefail; \
	for module in $(MODULES); do cd $(ROOT)/$$module && GOFIPS140=$(FIPS_MODULE) $(GO) mod tidy; done

tidy-check:
	@set -euo pipefail; \
	$(MAKE) tidy; \
	if ! git diff --exit-code -- go.mod go.sum $(addsuffix /go.mod,$(MODULES)) $(addsuffix /go.sum,$(MODULES)); then \
		printf '%s\n' "go mod tidy changed module metadata"; \
		exit 1; \
	fi

wire-generate:
	@cd $(ROOT)/platform/shared && $(GO) run ./cmd/wiregen

wire-generate-check:
	@cd $(ROOT)/platform/shared && $(GO) run ./cmd/wiregen -check

build: build-gateway build-client

build-gateway:
	@mkdir -p $(BIN_DIR)
	@cd $(GATEWAY_DIR) && CGO_ENABLED=0 GOFIPS140=$(FIPS_MODULE) $(GO) build -trimpath -o $(BIN_DIR)/xenomorph-gateway ./cmd

build-client:
	@mkdir -p $(BIN_DIR)
	@cd $(CLIENT_DIR) && CGO_ENABLED=0 GOFIPS140=$(FIPS_MODULE) $(GO) build -trimpath -o $(BIN_DIR)/xenomorph-client ./cmd

build-all:
	@set -euo pipefail; \
	for target in $(TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; extension=""; \
		if [[ "$$os" == "windows" ]]; then extension=".exe"; fi; \
		output=$(BIN_DIR)/$$os/$$arch; mkdir -p "$$output"; \
		(cd $(GATEWAY_DIR) && CGO_ENABLED=0 GOFIPS140=$(FIPS_MODULE) GOOS="$$os" GOARCH="$$arch" $(GO) build -trimpath -o "$$output/xenomorph-gateway$$extension" ./cmd); \
		(cd $(CLIENT_DIR) && CGO_ENABLED=0 GOFIPS140=$(FIPS_MODULE) GOOS="$$os" GOARCH="$$arch" $(GO) build -trimpath -o "$$output/xenomorph-client$$extension" ./cmd); \
	done

run-gateway:
	@cd $(GATEWAY_DIR) && GOFIPS140=$(FIPS_MODULE) $(GO) run ./cmd

run-client:
	@cd $(CLIENT_DIR) && GOFIPS140=$(FIPS_MODULE) $(GO) run ./cmd

clean:
	@rm -rf $(BIN_DIR)

lint:
	@set -euo pipefail; \
	if [[ ! -x $(GOLANGCI_LINT) ]]; then printf '%s\n' "golangci-lint is not installed at $(GOLANGCI_LINT)"; exit 1; fi; \
	for module in $(MODULES); do cd $(ROOT)/$$module && $(GOLANGCI_LINT) run --config $(ROOT)/.golangci.yml; done

lint-fix:
	@set -euo pipefail; \
	if [[ ! -x $(GOLANGCI_LINT) ]]; then printf '%s\n' "golangci-lint is not installed at $(GOLANGCI_LINT)"; exit 1; fi; \
	for module in $(MODULES); do cd $(ROOT)/$$module && $(GOLANGCI_LINT) run --config $(ROOT)/.golangci.yml --fix; done

install-tools:
	@$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
	@$(GO) install honnef.co/go/tools/cmd/staticcheck@v0.7.0
	@$(GO) install golang.org/x/vuln/cmd/govulncheck@v1.1.4
	@$(GO) install github.com/securego/gosec/v2/cmd/gosec@v2.22.10

web-install:
	@cd $(WEBSITE_DIR) && $(BUN) install --frozen-lockfile

web-format-check:
	@cd $(WEBSITE_DIR) && $(BUN) run format:check

web-lint:
	@cd $(WEBSITE_DIR) && $(BUN) run lint

web-typecheck:
	@cd $(WEBSITE_DIR) && $(BUN) run typecheck

web-build:
	@cd $(WEBSITE_DIR) && $(BUN) run build

ci-go: wire-generate-check fmt-check tidy-check test-race govulncheck lint build-all

ci-web: web-install web-format-check web-lint web-typecheck web-build

ci: ci-go ci-web
