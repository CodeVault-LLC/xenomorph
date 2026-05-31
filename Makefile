SHELL := /bin/bash

ROOT := $(CURDIR)
BIN_DIR := $(ROOT)/bin
MODULES := platform/client platform/services/gateway platform/shared

GO ?= go
GOPATH ?= $(shell $(GO) env GOPATH)
GOFMT ?= gofmt

GOBIN := $(if $(strip $(GOPATH)),$(GOPATH)/bin,$(ROOT)/bin)
GOLANGCI_LINT ?= $(GOBIN)/golangci-lint

GATEWAY_DIR := $(ROOT)/platform/services/gateway
CLIENT_DIR := $(ROOT)/platform/client

.PHONY: help fmt test tidy build build-gateway build-client run-gateway run-client clean lint

help:
	@printf '%s\n' "Available targets:"
	@printf '%s\n' "  make fmt           Format Go source across the repository"
	@printf '%s\n' "  make test          Run tests for every Go module"
	@printf '%s\n' "  make tidy          Run go mod tidy in every Go module"
	@printf '%s\n' "  make build         Build gateway and client binaries"
	@printf '%s\n' "  make run-gateway   Run the gateway directly from source"
	@printf '%s\n' "  make run-client    Run the client directly from source"
	@printf '%s\n' "  make clean         Remove build outputs"
	@printf '%s\n' "  make lint          Run golangci-lint when installed"

fmt:
	@set -euo pipefail; \
	for module in $(MODULES); do \
		cd $(ROOT)/$$module && find . -name '*.go' -not -path '*/gen/go/*' -print0 | xargs -0r $(GOFMT) -w; \
	done

test:
	@set -euo pipefail; \
	for module in $(MODULES); do \
		cd $(ROOT)/$$module && $(GO) test ./...; \
	done

tidy:
	@set -euo pipefail; \
	for module in $(MODULES); do \
		cd $(ROOT)/$$module && $(GO) mod tidy; \
	done

build: build-gateway build-client

build-gateway:
	@mkdir -p $(BIN_DIR)
	@cd $(GATEWAY_DIR) && $(GO) build -o $(BIN_DIR)/xenomorph-gateway ./cmd

build-client:
	@mkdir -p $(BIN_DIR)
	@cd $(CLIENT_DIR) && $(GO) build -o $(BIN_DIR)/xenomorph-client ./cmd

run-gateway:
	@cd $(GATEWAY_DIR) && $(GO) run ./cmd

run-client:
	@cd $(CLIENT_DIR) && $(GO) run ./cmd

clean:
	@rm -rf $(BIN_DIR)

lint:
	@set -euo pipefail; \
	if [[ -x $(GOLANGCI_LINT) ]]; then \
		for module in $(MODULES); do \
			cd $(ROOT)/$$module && $(GOLANGCI_LINT) run; \
		done; \
	else \
		printf '%s\n' "golangci-lint is not installed at $(GOLANGCI_LINT)"; \
		exit 1; \
	fi